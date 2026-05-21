// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
	"github.com/Snuffy2/shellport/application/network"
	"github.com/Snuffy2/shellport/application/rw"
)

const (
	MoshServerRemoteStdOut               = 0x00
	MoshServerHookOutputBeforeConnecting = 0x01
	MoshServerConnectFailed              = 0x02
	MoshServerConnectSucceed             = 0x03
)

const (
	MoshClientStdIn  = 0x00
	MoshClientResize = 0x01
)

const (
	MoshRequestErrorBadUserName      = command.StreamError(0x01)
	MoshRequestErrorBadRemoteAddress = command.StreamError(0x02)
	MoshRequestErrorBadAuthMethod    = command.StreamError(0x03)
	MoshRequestErrorUnsupportedProxy = command.StreamError(0x04)
	MoshRequestErrorBadMetadata      = command.StreamError(0x05)
)

const (
	moshMaxUsernameLen = sshMaxUsernameLen
	moshMaxHostnameLen = sshMaxHostnameLen
)

var (
	ErrMoshUnknownClientSignal = errors.New("unknown client signal")
	ErrMoshSocks5Unsupported   = errors.New(
		"Mosh does not support SOCKS5 proxying in this version because its session uses UDP",
	)
	ErrMoshRemoteSessionUnavailable = errors.New("remote Mosh session is unavailable")
)

var moshServerDetachedPIDPattern = regexp.MustCompile(`(?m)\[mosh-server detached, pid = ([0-9]+)\]`)

type moshSessionBuilder func(host string, port int, key string) (moshSession, error)
type moshHostResolver func(ctx context.Context, host string) ([]net.IP, error)

type moshClient struct {
	w          command.StreamResponder
	l          log.Logger
	hooks      command.Hooks
	cfg        command.Configuration
	meta       map[string]string
	bufferPool *command.BufferPool

	baseCtx       context.Context
	baseCtxCancel func()

	remoteCloseWait                 sync.WaitGroup
	remoteReadTimeoutRetry          bool
	remoteReadForceRetryNextTimeout bool
	remoteReadTimeoutRetryLock      sync.Mutex

	credentialReceive                       chan []byte
	credentialProcessed                     bool
	credentialReceiveCloseOnce              sync.Once
	fingerprintVerifyResultReceive          chan bool
	fingerprintProcessed                    bool
	fingerprintVerifyResultReceiveCloseOnce sync.Once
	processedLock                           sync.Mutex

	sessionReceive chan moshSession
	session        moshSession
	sessionClosed  bool
	sessionLock    sync.Mutex

	sessionBuilder moshSessionBuilder
	hostResolver   moshHostResolver
	remoteStarter  func(user string, address string, authMethodBuilder sshAuthMethodBuilder)
}

func newMosh(
	l log.Logger,
	hooks command.Hooks,
	w command.StreamResponder,
	cfg command.Configuration,
	bufferPool *command.BufferPool,
) command.FSMMachine {
	ctx, ctxCancel := context.WithCancel(context.Background())
	return &moshClient{
		w:                              w,
		l:                              l,
		hooks:                          hooks,
		cfg:                            cfg,
		bufferPool:                     bufferPool,
		baseCtx:                        ctx,
		baseCtxCancel:                  sync.OnceFunc(ctxCancel),
		credentialReceive:              make(chan []byte, 1),
		fingerprintVerifyResultReceive: make(chan bool, 1),
		sessionReceive:                 make(chan moshSession, 1),
		sessionBuilder:                 newMoshGoSession,
		hostResolver:                   defaultMoshHostResolver,
		sessionClosed:                  false,
		remoteReadTimeoutRetryLock:     sync.Mutex{},
		remoteCloseWait:                sync.WaitGroup{},
	}
}

func parseMoshConfig(p configuration.Preset) (configuration.Preset, error) {
	p = configuration.NormalizePresetMeta(p, map[string]string{
		"Encoding":    "utf-8",
		"Mosh Server": moshDefaultServerCommand,
	})
	return normalizePresetHost(p, sshDefaultPortString), nil
}

func moshRemoteHost(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}

	return host
}

func defaultMoshHostResolver(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

func (d *moshClient) Bootup(
	r *rw.LimitedReader,
	b []byte,
) (command.FSMState, command.FSMError) {
	if err := d.validateProxySupport(); err != nil {
		return nil, command.ToFSMError(err, MoshRequestErrorUnsupportedProxy)
	}

	sBuf := d.bufferPool.Get()
	defer d.bufferPool.Put(sBuf)

	userName, userNameErr := ParseString(r.Read, (*sBuf)[:moshMaxUsernameLen])
	if userNameErr != nil {
		return nil, command.ToFSMError(userNameErr, MoshRequestErrorBadUserName)
	}
	userNameStr := string(userName.Data())

	addr, addrErr := ParseAddress(r.Read, (*sBuf)[:moshMaxHostnameLen])
	if addrErr != nil {
		return nil, command.ToFSMError(addrErr, MoshRequestErrorBadRemoteAddress)
	}

	addrStr := addr.String()
	if addrStr == "" {
		return nil, command.ToFSMError(ErrSSHInvalidAddress, MoshRequestErrorBadRemoteAddress)
	}

	rData, rErr := rw.FetchOneByte(r.Fetch)
	if rErr != nil {
		return nil, command.ToFSMError(rErr, MoshRequestErrorBadAuthMethod)
	}

	requestMeta, requestMetaErr := parseMoshRequestMeta(r, (*sBuf)[:])
	if requestMetaErr != nil {
		return nil, command.ToFSMError(requestMetaErr, MoshRequestErrorBadMetadata)
	}
	d.meta = requestMeta
	presetID, presetIDErr := parseOptionalPresetID(
		r,
		(*sBuf)[:configuration.MaxPresetIDLength],
	)
	if presetIDErr != nil {
		return nil, command.ToFSMError(presetIDErr, MoshRequestErrorBadMetadata)
	}

	authMethodBuilder, authMethodBuilderErr := d.buildAuthMethod(
		rData[0],
		presetID,
		userNameStr,
		addrStr,
	)
	if authMethodBuilderErr != nil {
		return nil, command.ToFSMError(authMethodBuilderErr, MoshRequestErrorBadAuthMethod)
	}

	details := connectionDebugDetails{
		Protocol:   "Mosh",
		User:       userNameStr,
		Address:    addrStr,
		Network:    "tcp4/udp4",
		AuthMethod: sshAuthMethodDebugName(rData[0]),
		PresetID:   presetID,
	}
	debugConnectionAttempt(d.l, details)

	d.remoteCloseWait.Add(1)
	if d.remoteStarter != nil {
		go d.remoteStarter(userNameStr, addrStr, authMethodBuilder)
	} else {
		go d.remote(userNameStr, addrStr, authMethodBuilder, details)
	}

	return d.local, command.NoFSMError()
}

func (d *moshClient) validateProxySupport() error {
	if d.cfg.Socks5Configured {
		return ErrMoshSocks5Unsupported
	}

	return nil
}

func parseMoshRequestMeta(r *rw.LimitedReader, b []byte) (map[string]string, error) {
	meta := map[string]string{"Mosh Server": ""}
	if r.Completed() {
		return meta, nil
	}

	serverPath, err := ParseString(r.Read, b)
	if err != nil {
		return nil, err
	}

	moshServer := strings.TrimSpace(string(serverPath.Data()))
	if err := validateMoshServerPath(moshServer); err != nil {
		return nil, err
	}

	meta["Mosh Server"] = moshServer
	return meta, nil
}

func validateMoshServerPath(serverPath string) error {
	serverPath = strings.TrimSpace(serverPath)
	if serverPath == "" {
		return ErrStringParseBufferTooSmall
	}

	if strings.ContainsAny(serverPath, " \t\r\n") {
		return errors.New("Mosh Server must be an executable path without arguments")
	}

	return nil
}

func (d *moshClient) buildAuthMethod(
	methodType byte,
	presetID string,
	user string,
	host string,
) (sshAuthMethodBuilder, error) {
	switch methodType {
	case SSHAuthMethodNone:
		return func(b []byte) []ssh.AuthMethod { return nil }, nil
	case SSHAuthMethodPassphrase:
		return func(b []byte) []ssh.AuthMethod {
			return []ssh.AuthMethod{
				ssh.PasswordCallback(func() (string, error) {
					if credential, ok := presetPasswordCredential(
						d.cfg,
						"Mosh",
						presetID,
						user,
						host,
					); ok {
						return credential, nil
					}

					d.enableRemoteReadTimeoutRetry()
					defer d.disableRemoteReadTimeoutRetry()

					if wErr := d.w.SendManual(SSHServerConnectRequestCredential, b[d.w.HeaderSize():]); wErr != nil {
						return "", wErr
					}

					passphraseBytes, passphraseReceived := <-d.credentialReceive
					if !passphraseReceived {
						return "", ErrSSHAuthCancelled
					}

					return string(passphraseBytes), nil
				}),
			}
		}, nil
	case SSHAuthMethodPrivateKey:
		return func(b []byte) []ssh.AuthMethod {
			return []ssh.AuthMethod{
				ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
					if credential, ok := presetPrivateKeyCredential(
						d.cfg,
						"Mosh",
						presetID,
						user,
						host,
					); ok {
						signer, signerErr := ssh.ParsePrivateKey([]byte(credential))
						if signerErr != nil {
							return nil, signerErr
						}
						return []ssh.Signer{signer}, nil
					}

					d.enableRemoteReadTimeoutRetry()
					defer d.disableRemoteReadTimeoutRetry()

					if wErr := d.w.SendManual(SSHServerConnectRequestCredential, b[d.w.HeaderSize():]); wErr != nil {
						return nil, wErr
					}

					privateKeyBytes, privateKeyReceived := <-d.credentialReceive
					if !privateKeyReceived {
						return nil, ErrSSHAuthCancelled
					}

					signer, signerErr := ssh.ParsePrivateKey(privateKeyBytes)
					if signerErr != nil {
						return nil, signerErr
					}

					return []ssh.Signer{signer}, nil
				}),
			}
		}, nil
	default:
		return nil, ErrSSHInvalidAuthMethod
	}
}

func (d *moshClient) confirmRemoteFingerprint(
	hostname string,
	remote net.Addr,
	key ssh.PublicKey,
	buf []byte,
) error {
	d.enableRemoteReadTimeoutRetry()
	defer d.disableRemoteReadTimeoutRetry()

	fgp := ssh.FingerprintSHA256(key)
	fgpLen := copy(buf[d.w.HeaderSize():], fgp)

	if wErr := d.w.SendManual(SSHServerConnectVerifyFingerprint, buf[:d.w.HeaderSize()+fgpLen]); wErr != nil {
		return wErr
	}

	confirmed, confirmOK := <-d.fingerprintVerifyResultReceive
	if !confirmOK {
		return ErrSSHRemoteFingerprintVerificationCancelled
	}
	if !confirmed {
		return ErrSSHRemoteFingerprintRefused
	}

	return nil
}

func (d *moshClient) enableRemoteReadTimeoutRetry() {
	d.remoteReadTimeoutRetryLock.Lock()
	defer d.remoteReadTimeoutRetryLock.Unlock()
	d.remoteReadTimeoutRetry = true
}

func (d *moshClient) disableRemoteReadTimeoutRetry() {
	d.remoteReadTimeoutRetryLock.Lock()
	defer d.remoteReadTimeoutRetryLock.Unlock()
	d.remoteReadTimeoutRetry = false
	d.remoteReadForceRetryNextTimeout = true
}

func (d *moshClient) dialRemote(
	networkName string,
	addr string,
	config *ssh.ClientConfig,
) (*ssh.Client, net.Addr, func(), error) {
	dialCtx, dialCtxCancel := context.WithTimeout(d.baseCtx, config.Timeout)
	defer dialCtxCancel()

	conn, err := d.cfg.Dial(dialCtx, networkName, addr)
	if err != nil {
		return nil, nil, nil, err
	}
	peerAddr := conn.RemoteAddr()

	sshConn := &sshRemoteConnWrapper{
		Conn:       conn,
		writerConn: network.NewWriteTimeoutConn(conn, d.cfg.DialTimeout),
		requestTimeoutRetry: func(s *sshRemoteConnWrapper) bool {
			d.remoteReadTimeoutRetryLock.Lock()
			defer d.remoteReadTimeoutRetryLock.Unlock()

			if !d.remoteReadTimeoutRetry {
				if !d.remoteReadForceRetryNextTimeout {
					return false
				}
				d.remoteReadForceRetryNextTimeout = false
			}

			s.SetReadDeadline(time.Now().Add(config.Timeout))
			return true
		},
	}

	sshConn.SetWriteDeadline(time.Now().Add(d.cfg.DialTimeout))
	sshConn.SetReadDeadline(time.Now().Add(config.Timeout))

	c, chans, reqs, err := ssh.NewClientConn(sshConn, addr, config)
	if err != nil {
		sshConn.Close()
		return nil, nil, nil, err
	}

	return ssh.NewClient(c, chans, reqs), peerAddr, func() {
		d.remoteReadTimeoutRetryLock.Lock()
		defer d.remoteReadTimeoutRetryLock.Unlock()
		d.remoteReadTimeoutRetry = false
		d.remoteReadForceRetryNextTimeout = true
		sshConn.SetReadDeadline(sshEmptyTime)
	}, nil
}

func (d *moshClient) remote(
	user string,
	address string,
	authMethodBuilder sshAuthMethodBuilder,
	details connectionDebugDetails,
) {
	u := d.bufferPool.Get()
	defer d.bufferPool.Put(u)

	var session moshSession
	defer func() {
		if session != nil {
			session.Close()
		}
		debugConnectionDisconnected(d.l, details, "remote goroutine exited", nil)
		d.w.Signal(command.HeaderClose)
		close(d.sessionReceive)
		d.baseCtxCancel()
		d.remoteCloseWait.Done()
	}()

	err := d.hooks.Run(
		d.baseCtx,
		configuration.HOOK_BEFORE_CONNECTING,
		command.NewHookParameters(2).
			Insert("Remote Type", "Mosh").
			Insert("Remote Address", address),
		command.NewDefaultHookOutput(d.l, func(b []byte) (int, error) {
			dLen := copy((*u)[d.w.HeaderSize():], b) + d.w.HeaderSize()
			return len(b), d.w.SendManual(MoshServerHookOutputBeforeConnecting, (*u)[:dLen])
		}),
	)
	if err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}

	if err := d.validateMoshRemoteAllowed(address); err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}

	bootstrapNetwork, err := moshSSHBootstrapNetwork(address)
	if err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}

	bootstrapDetails := details
	bootstrapDetails.Network = bootstrapNetwork
	d.l.Debug("Dialing Mosh SSH bootstrap remote: %s", bootstrapDetails.fields())
	conn, peerAddr, clearConnInitialDeadline, err := d.dialRemote(bootstrapNetwork, address, &ssh.ClientConfig{
		User: user,
		Auth: authMethodBuilder((*u)[:]),
		HostKeyCallback: func(h string, r net.Addr, k ssh.PublicKey) error {
			return d.confirmRemoteFingerprint(h, r, k, (*u)[:])
		},
		Timeout: d.cfg.DialTimeout,
	})
	if err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, bootstrapDetails, err)
		return
	}
	defer func() {
		d.baseCtxCancel()
		conn.Close()
	}()
	d.l.Debug("Mosh SSH bootstrap connected: %s", bootstrapDetails.fields())

	output, err := d.bootstrapRemoteMoshServer(conn)
	clearConnInitialDeadline()
	if err != nil {
		d.sendConnectFailed((*u)[:], fmt.Errorf("failed to bootstrap mosh-server: %w", err))
		debugConnectionFailed(d.l, details, err)
		return
	}
	d.l.Debug("Mosh server bootstrap output parsed from %s", address)

	connectInfo, err := parseMoshConnectLine(output)
	if err != nil {
		d.sendConnectFailed((*u)[:], fmt.Errorf("failed to parse mosh-server bootstrap output: %w", err))
		debugConnectionFailed(d.l, details, err)
		return
	}

	d.l.Debug("Dialing Mosh UDP session: address=%s port=%d", address, connectInfo.Port)
	session, err = d.buildRemoteSession(address, peerAddr, connectInfo.Port, connectInfo.Key)
	if err != nil {
		d.sendConnectFailed((*u)[:], fmt.Errorf("failed to connect to remote mosh session: %w", err))
		debugConnectionFailed(d.l, details, err)
		return
	}

	d.cacheSession(session)
	if monitorDone, monitorErr := d.monitorRemoteMoshServer(conn, output); monitorErr != nil {
		logRemoteMoshServerLifecycleUnavailable(d.l, monitorErr)
	} else if monitorDone != nil {
		go d.closeSessionWhenRemoteMoshServerExits(monitorDone)
	}

	initialOutput, err := d.awaitRemoteSessionReady(session)
	if err != nil {
		d.sendConnectFailed((*u)[:], fmt.Errorf("failed to verify remote mosh session readiness: %w", err))
		debugConnectionFailed(d.l, details, err)
		return
	}

	d.sessionReceive <- session
	if err = d.w.SendManual(MoshServerConnectSucceed, (*u)[:d.w.HeaderSize()]); err != nil {
		debugConnectionDisconnected(d.l, details, "connect-success response failed", err)
		return
	}
	debugConnectionEstablished(d.l, details)

	if err = d.sendRemoteOutput((*u)[:], initialOutput); err != nil {
		debugConnectionDisconnected(d.l, details, "initial output client send failed", err)
		return
	}

	for {
		output, recvErr := session.Recv(d.recvTimeout())
		if recvErr != nil {
			debugConnectionDisconnected(d.l, details, "remote receive failed", recvErr)
			return
		}

		select {
		case <-d.baseCtx.Done():
			return
		default:
		}

		if err = d.sendRemoteOutput((*u)[:], output); err != nil {
			debugConnectionDisconnected(d.l, details, "client send failed", err)
			return
		}
	}
}

func (d *moshClient) buildRemoteSession(address string, peerAddr net.Addr, port int, key string) (moshSession, error) {
	host, err := d.resolveMoshSessionHost(address, peerAddr)
	if err != nil {
		return nil, err
	}

	return d.sessionBuilder(host, port, key)
}

func parseMoshServerDetachedPID(output string) (int, bool) {
	matches := moshServerDetachedPIDPattern.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0, false
	}

	pidText := matches[len(matches)-1][1]
	pid, err := strconv.Atoi(pidText)
	if err != nil || pid <= 0 {
		return 0, false
	}

	return pid, true
}

func (d *moshClient) monitorRemoteMoshServer(conn *ssh.Client, output string) (<-chan bool, error) {
	pid, ok := parseMoshServerDetachedPID(output)
	if !ok {
		return nil, nil
	}

	session, err := conn.NewSession()
	if err != nil {
		return nil, err
	}

	done := make(chan bool, 1)
	go func() {
		defer close(done)
		defer session.Close()
		commandText := renderMoshServerMonitorCommand(pid)
		output, err := session.CombinedOutput(commandText)
		if err != nil {
			logRemoteMoshServerMonitorError(d.baseCtx, d.l, err)
			done <- false
			return
		}
		done <- strings.Contains(string(output), "shellport-mosh-server-exited")
	}()

	return done, nil
}

func renderMoshServerMonitorCommand(pid int) string {
	script := fmt.Sprintf(
		`while kill -0 %d 2>/dev/null; do sleep 1; done; printf "%%s\n" shellport-mosh-server-exited`,
		pid,
	)
	return fmt.Sprintf("sh -c '%s'", script)
}

func (d *moshClient) closeSessionWhenRemoteMoshServerExits(monitorDone <-chan bool) {
	select {
	case serverExited, ok := <-monitorDone:
		if !ok || !serverExited {
			return
		}
		d.baseCtxCancel()
		if err := d.closeSession(); err != nil {
			logRemoteMoshServerCloseError(d.l, err)
		}
	case <-d.baseCtx.Done():
	}
}

func logRemoteMoshServerLifecycleUnavailable(l log.Logger, err error) {
	l.Warning("Unable to monitor remote mosh-server lifecycle: %s", err)
}

func logRemoteMoshServerMonitorError(ctx context.Context, l log.Logger, err error) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	l.Warning("Remote mosh-server monitor exited with an error: %s", err)
}

func logRemoteMoshServerCloseError(l log.Logger, err error) {
	l.Warning("Failed to close ended remote mosh session: %s", err)
}

func moshSSHBootstrapNetwork(address string) (string, error) {
	host := moshRemoteHost(address)
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return "", fmt.Errorf(
			"Mosh v1 requires an IPv4 target because the current mosh-go UDP client is IPv4-only: %q is IPv6",
			host,
		)
	}

	return "tcp4", nil
}

func (d *moshClient) awaitRemoteSessionReady(session moshSession) ([]byte, error) {
	return session.AwaitReady(d.baseCtx, d.recvTimeout())
}

func (d *moshClient) sendRemoteOutput(buf []byte, output []byte) error {
	if len(output) == 0 {
		return nil
	}

	return d.w.Send(MoshServerRemoteStdOut, output, buf)
}

func (d *moshClient) validateMoshRemoteAllowed(address string) error {
	if !d.cfg.OnlyAllowPresetRemotes {
		return nil
	}

	presets := d.cfg.Presets
	if d.cfg.PresetRepository != nil {
		presets = d.cfg.PresetRepository.List()
	}
	for _, preset := range presets {
		if preset.Host == address {
			return nil
		}
	}

	return network.ErrAccessControlDialTargetHostNotAllowed
}

func (d *moshClient) resolveMoshSessionHost(address string, peerAddr net.Addr) (string, error) {
	if peerAddr != nil {
		return normalizeMoshPeerHost(peerAddr)
	}

	host := moshRemoteHost(address)
	if ip := net.ParseIP(host); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			return ipv4.String(), nil
		}

		return "", fmt.Errorf(
			"Mosh v1 requires an IPv4 target because the current mosh-go UDP client is IPv4-only: %q is IPv6",
			host,
		)
	}

	resolveCtx := d.baseCtx
	if resolveCtx == nil {
		resolveCtx = context.Background()
	}

	cancel := func() {}
	if d.cfg.DialTimeout > 0 {
		resolveCtx, cancel = context.WithTimeout(resolveCtx, d.cfg.DialTimeout)
	}
	defer cancel()

	resolver := d.hostResolver
	if resolver == nil {
		resolver = defaultMoshHostResolver
	}

	ips, err := resolver(resolveCtx, host)
	if err != nil {
		return "", fmt.Errorf("resolve mosh host %q: %w", host, err)
	}

	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			return ipv4.String(), nil
		}
	}

	return "", fmt.Errorf(
		"Mosh v1 requires an IPv4 target because the current mosh-go UDP client is IPv4-only: %q did not resolve to IPv4",
		host,
	)
}

func normalizeMoshPeerHost(peerAddr net.Addr) (string, error) {
	if tcpAddr, ok := peerAddr.(*net.TCPAddr); ok {
		if ipv4 := tcpAddr.IP.To4(); ipv4 != nil {
			return ipv4.String(), nil
		}

		return "", fmt.Errorf(
			"Mosh v1 requires an IPv4 target because the current mosh-go UDP client is IPv4-only: %q is not IPv4",
			tcpAddr.IP.String(),
		)
	}

	host := peerAddr.String()
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("remote SSH peer address %q is not an IP literal", peerAddr.String())
	}

	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.String(), nil
	}

	return "", fmt.Errorf(
		"Mosh v1 requires an IPv4 target because the current mosh-go UDP client is IPv4-only: %q is not IPv4",
		host,
	)
}

func (d *moshClient) bootstrapRemoteMoshServer(conn *ssh.Client) (string, error) {
	session, err := conn.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(renderMoshServerCommand(d.meta))
	outputText := strings.TrimSpace(string(output))
	if err != nil {
		sanitizedOutputText := sanitizeMoshBootstrapOutput(outputText)
		if sanitizedOutputText != "" {
			return sanitizedOutputText, fmt.Errorf("%w: %s", err, sanitizedOutputText)
		}

		return sanitizedOutputText, err
	}

	return outputText, nil
}

func sanitizeMoshBootstrapOutput(output string) string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && fields[0] == "MOSH" && fields[1] == "CONNECT" {
			if len(fields) >= 3 {
				lines[i] = fmt.Sprintf("MOSH CONNECT %s <REDACTED>", fields[2])
				continue
			}
			lines[i] = "MOSH CONNECT <REDACTED>"
		}
	}

	return strings.Join(lines, "\n")
}

func (d *moshClient) recvTimeout() time.Duration {
	if d.cfg.DialTimeout > 0 {
		return d.cfg.DialTimeout
	}

	return time.Second
}

func (d *moshClient) sendConnectFailed(buf []byte, err error) {
	errLen := copy(buf[d.w.HeaderSize():], err.Error()) + d.w.HeaderSize()
	d.w.SendManual(MoshServerConnectFailed, buf[:errLen])
}

func (d *moshClient) cacheSession(session moshSession) moshSession {
	d.sessionLock.Lock()
	defer d.sessionLock.Unlock()

	if d.session == nil {
		d.session = session
		d.sessionClosed = false
	}

	return d.session
}

func (d *moshClient) getSession() (moshSession, error) {
	d.sessionLock.Lock()
	if d.session != nil {
		session := d.session
		d.sessionLock.Unlock()
		return session, nil
	}
	d.sessionLock.Unlock()

	session, ok := <-d.sessionReceive
	if !ok {
		return nil, ErrMoshRemoteSessionUnavailable
	}

	return d.cacheSession(session), nil
}

func (d *moshClient) getSessionIfReady() (moshSession, bool) {
	d.sessionLock.Lock()
	if d.session != nil {
		session := d.session
		d.sessionLock.Unlock()
		return session, true
	}
	d.sessionLock.Unlock()

	select {
	case session, ok := <-d.sessionReceive:
		if !ok {
			return nil, false
		}

		return d.cacheSession(session), true
	default:
		return nil, false
	}
}

func (d *moshClient) closeSession() error {
	d.sessionLock.Lock()
	defer d.sessionLock.Unlock()

	if d.session == nil || d.sessionClosed {
		return nil
	}

	d.sessionClosed = true
	return d.session.Close()
}

func (d *moshClient) local(
	f *command.FSM,
	r *rw.LimitedReader,
	h command.StreamHeader,
	b []byte,
) error {
	switch h.Marker() {
	case MoshClientStdIn:
		session, sessionReady := d.getSessionIfReady()

		for !r.Completed() {
			rData, rErr := r.Buffered()
			if rErr != nil {
				return rErr
			}

			if !sessionReady {
				continue
			}

			if wErr := session.Send(rData); wErr != nil {
				if closeErr := d.closeSession(); closeErr != nil {
					return closeErr
				}
				d.l.Debug("Failed to write data to remote mosh session: %s", wErr)
				return wErr
			}
		}

		return nil

	case MoshClientResize:
		if _, rErr := io.ReadFull(r, b[:4]); rErr != nil {
			return rErr
		}

		session, sessionReady := d.getSessionIfReady()
		if !sessionReady {
			return nil
		}

		rows := uint16(b[0])<<8 | uint16(b[1])
		cols := uint16(b[2])<<8 | uint16(b[3])
		if resizeErr := session.Resize(cols, rows); resizeErr != nil {
			d.l.Debug("Failed to resize mosh session to cols=%d rows=%d: %s", cols, rows, resizeErr)
		}
		return nil

	case SSHClientRespondFingerprint:
		d.processedLock.Lock()
		if d.fingerprintProcessed {
			d.processedLock.Unlock()
			return ErrSSHUnexpectedFingerprintVerificationRespond
		}
		d.fingerprintProcessed = true

		rData, rErr := rw.FetchOneByte(r.Fetch)
		if rErr != nil {
			d.processedLock.Unlock()
			return rErr
		}

		d.fingerprintVerifyResultReceive <- (rData[0] == 0)
		d.processedLock.Unlock()
		return nil

	case SSHClientRespondCredential:
		d.processedLock.Lock()
		if d.credentialProcessed {
			d.processedLock.Unlock()
			return ErrSSHUnexpectedCredentialDataRespond
		}
		d.credentialProcessed = true

		credentialDataBufSize := min(r.Remains(), sshCredentialMaxSize)
		credentialDataBuf := make([]byte, 0, credentialDataBufSize)
		totalCredentialRead := 0

		for !r.Completed() {
			rData, rErr := r.Buffered()
			if rErr != nil {
				d.processedLock.Unlock()
				return rErr
			}

			totalCredentialRead += len(rData)
			if totalCredentialRead > credentialDataBufSize {
				d.processedLock.Unlock()
				return ErrSSHCredentialDataTooLarge
			}

			credentialDataBuf = append(credentialDataBuf, rData...)
		}

		d.credentialReceive <- credentialDataBuf
		d.processedLock.Unlock()
		return nil
	default:
		return ErrMoshUnknownClientSignal
	}
}

func (d *moshClient) Close() error {
	d.processedLock.Lock()
	d.credentialProcessed = true
	d.fingerprintProcessed = true
	d.credentialReceiveCloseOnce.Do(func() {
		close(d.credentialReceive)
	})
	d.fingerprintVerifyResultReceiveCloseOnce.Do(func() {
		close(d.fingerprintVerifyResultReceive)
	})
	d.processedLock.Unlock()

	d.baseCtxCancel()
	if closeErr := d.closeSession(); closeErr != nil {
		d.remoteCloseWait.Wait()
		return closeErr
	}
	d.remoteCloseWait.Wait()
	return nil
}

func (d *moshClient) Release() error {
	d.baseCtxCancel()
	return nil
}
