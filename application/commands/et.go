// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
	"github.com/Snuffy2/shellport/application/network"
	"github.com/Snuffy2/shellport/application/rw"
)

// Server-to-client stream marker constants for ET.
const (
	ETServerRemoteStdOut               = 0x00
	ETServerHookOutputBeforeConnecting = 0x01
	ETServerConnectFailed              = 0x02
	ETServerConnectSucceed             = 0x03
	ETServerConnectVerifyFingerprint   = 0x05
	ETServerConnectRequestCredential   = 0x06
)

// Client-to-server stream marker constants for ET.
const (
	ETClientStdIn              = 0x00
	ETClientResize             = 0x01
	ETClientRespondFingerprint = 0x02
	ETClientRespondCredential  = 0x03
)

// ET bootup stream error codes.
const (
	ETRequestErrorBadUserName      = command.StreamError(0x01)
	ETRequestErrorBadRemoteAddress = command.StreamError(0x02)
	ETRequestErrorBadAuthMethod    = command.StreamError(0x03)
	ETRequestErrorUnsupportedProxy = command.StreamError(0x04)
	ETRequestErrorBadMetadata      = command.StreamError(0x05)
)

const (
	etMaxUsernameLen = 127
	etMaxHostnameLen = 255
)

const etProcessStartupGrace = 250 * time.Millisecond

var (
	ErrETSocks5Unsupported = errors.New(
		"ET does not support SOCKS5 proxying in this version")

	ErrETUnsupportedAuthMethod = errors.New(
		"invalid ET auth method")

	ErrETRemoteUnavailable = errors.New(
		"remote ET process is unavailable")
)

type etProcessReadResult struct {
	Data []byte
	Err  error
}

type etClient struct {
	w     command.StreamResponder
	l     log.Logger
	hooks command.Hooks
	cfg   command.Configuration

	meta       etMetadata
	bufferPool *command.BufferPool

	process     etProcess
	processLock sync.Mutex

	baseCtx         context.Context
	baseCtxCancel   func()
	remoteCloseWait sync.WaitGroup

	credentialReceive                       chan []byte
	credentialReceiveCloseOnce              sync.Once
	fingerprintVerifyResultReceive          chan bool
	fingerprintVerifyResultReceiveCloseOnce sync.Once
	remoteReadTimeoutRetry                  bool
	remoteReadForceRetryNextTimeout         bool
	remoteReadTimeoutRetryLock              sync.Mutex
	credentialProcessed                     bool
	fingerprintProcessed                    bool
	credentialReceiveClosed                 bool
	fingerprintVerifyResultReceiveClosed    bool
	processedLock                           sync.Mutex

	remoteStarter  func(user string, address string, authMethodBuilder sshAuthMethodBuilder, metadata etMetadata, presetID string)
	processStarter etProcessStarter
	remoteDialer   etRemoteDialer
	sendToClient   bool
	sendFrameHook  func(byte, []byte)

	privateKey            []byte
	privateKeyLock        sync.Mutex
	sendCredentialRequest func([]byte) error
}

type etRemoteDialer func(
	networkName string,
	addr string,
	config *ssh.ClientConfig,
) (io.Closer, net.Addr, func(), error)

type etProcessStarter func(
	ctx context.Context,
	metadata etMetadata,
	user string,
	sshAddress string,
	sshConfigPath string,
) (etProcess, error)

func newET(
	l log.Logger,
	hooks command.Hooks,
	w command.StreamResponder,
	cfg command.Configuration,
	bufferPool *command.BufferPool,
) command.FSMMachine {
	ctx, ctxCancel := context.WithCancel(context.Background())
	return &etClient{
		w:             w,
		l:             l,
		hooks:         hooks,
		cfg:           cfg,
		meta:          defaultETMetadata(),
		bufferPool:    bufferPool,
		baseCtx:       ctx,
		baseCtxCancel: sync.OnceFunc(ctxCancel),

		credentialReceive:                    make(chan []byte, 1),
		credentialProcessed:                  false,
		credentialReceiveClosed:              false,
		fingerprintVerifyResultReceive:       make(chan bool, 1),
		fingerprintProcessed:                 false,
		fingerprintVerifyResultReceiveClosed: false,
		processedLock:                        sync.Mutex{},
		remoteReadTimeoutRetry:               false,
		remoteReadForceRetryNextTimeout:      false,
		remoteReadTimeoutRetryLock:           sync.Mutex{},
		processStarter:                       startETPTY,
		sendToClient:                         true,
		remoteDialer:                         nil,
		privateKey:                           nil,
		privateKeyLock:                       sync.Mutex{},
	}
}

func (d *etClient) emitClientFrame(marker byte, data []byte) error {
	if d.sendFrameHook != nil {
		d.sendFrameHook(marker, append([]byte(nil), data...))
	}
	if !d.sendToClient {
		return nil
	}
	return d.w.SendManual(marker, data)
}

func parseETConfig(p configuration.Preset) (configuration.Preset, error) {
	p = configuration.NormalizePresetMeta(p, map[string]string{
		"Encoding":       "utf-8",
		"ET Server Port": strconv.Itoa(etDefaultServerPort),
		"ET Command":     etDefaultCommand,
	})
	return normalizePresetHost(p, sshDefaultPortString), nil
}

func (d *etClient) Bootup(
	r *rw.LimitedReader,
	b []byte,
) (command.FSMState, command.FSMError) {
	if err := d.validateProxySupport(); err != nil {
		return nil, command.ToFSMError(err, ETRequestErrorUnsupportedProxy)
	}

	sBuf := d.bufferPool.Get()
	defer d.bufferPool.Put(sBuf)

	userName, userNameErr := ParseString(r.Read, (*sBuf)[:etMaxUsernameLen])
	if userNameErr != nil {
		return nil, command.ToFSMError(userNameErr, ETRequestErrorBadUserName)
	}
	userNameStr := string(userName.Data())

	addr, addrErr := ParseAddress(r.Read, (*sBuf)[:etMaxHostnameLen])
	if addrErr != nil {
		return nil, command.ToFSMError(addrErr, ETRequestErrorBadRemoteAddress)
	}
	addrStr := addr.String()
	if addrStr == "" {
		return nil, command.ToFSMError(ErrSSHInvalidAddress, ETRequestErrorBadRemoteAddress)
	}

	authData, authErr := rw.FetchOneByte(r.Fetch)
	if authErr != nil {
		return nil, command.ToFSMError(authErr, ETRequestErrorBadAuthMethod)
	}

	metadata, metadataErr := parseETMetadata(r, (*sBuf)[:])
	if metadataErr != nil {
		return nil, command.ToFSMError(metadataErr, ETRequestErrorBadMetadata)
	}
	d.meta = metadata

	presetID, presetIDErr := parseOptionalPresetID(r, (*sBuf)[:configuration.MaxPresetIDLength])
	if presetIDErr != nil {
		return nil, command.ToFSMError(presetIDErr, ETRequestErrorBadMetadata)
	}

	authMethodBuilder, authMethodErr := d.buildAuthMethod(authData[0], presetID, userNameStr, addrStr)
	if authMethodErr != nil {
		return nil, command.ToFSMError(authMethodErr, ETRequestErrorBadAuthMethod)
	}

	d.remoteCloseWait.Add(1)
	if d.remoteStarter != nil {
		go d.remoteStarter(userNameStr, addrStr, authMethodBuilder, metadata, presetID)
	} else {
		go d.remote(userNameStr, addrStr, authMethodBuilder, metadata, presetID)
	}

	return d.local, command.NoFSMError()
}

func (d *etClient) remote(
	user string,
	address string,
	authMethodBuilder sshAuthMethodBuilder,
	metadata etMetadata,
	presetID string,
) {
	u := d.bufferPool.Get()
	defer d.bufferPool.Put(u)

	defer d.remoteCloseWait.Done()

	details := connectionDebugDetails{
		Protocol:   "ET",
		User:       user,
		Address:    address,
		Network:    "tcp",
		AuthMethod: sshAuthMethodDebugName(SSHAuthMethodPrivateKey),
		PresetID:   presetID,
	}
	debugConnectionAttempt(d.l, details)

	var process etProcess
	tempDir := ""
	var remotePublicKey ssh.PublicKey
	defer func() {
		if err := d.closeProcess(); err != nil {
			debugConnectionDisconnected(d.l, details, "process close failed", err)
		}
		if cleanupErr := cleanupETTempDir(tempDir); cleanupErr != nil {
			d.l.Warning("Failed to clean ET temp directory: %s", cleanupErr)
		}
		if d.sendToClient {
			_ = d.w.Signal(command.HeaderClose)
		}
		d.baseCtxCancel()
		debugConnectionDisconnected(d.l, details, "remote goroutine exited", nil)
	}()

	if err := d.validateETRemoteAllowed(address, metadata); err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}

	if err := d.hooks.Run(
		d.baseCtx,
		configuration.HOOK_BEFORE_CONNECTING,
		command.NewHookParameters(2).
			Insert("Remote Type", "ET").
			Insert("Remote Address", address),
		command.NewDefaultHookOutput(d.l, func(
			b []byte,
		) (wLen int, wErr error) {
			wLen = len(b)
			dLen := copy((*u)[d.w.HeaderSize():], b) + d.w.HeaderSize()
			wErr = d.emitClientFrame(
				ETServerHookOutputBeforeConnecting,
				(*u)[:dLen],
			)
			return
		}),
	); err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}

	dial := d.dialRemote
	if d.remoteDialer != nil {
		dial = d.remoteDialer
	}
	conn, _, clearConnInitialDeadline, err := dial("tcp", address, &ssh.ClientConfig{
		User: user,
		Auth: authMethodBuilder((*u)[:]),
		HostKeyCallback: func(h string, r net.Addr, k ssh.PublicKey) error {
			remotePublicKey = k
			return d.confirmRemoteFingerprint(h, r, k, (*u)[:])
		},
		Timeout: d.cfg.DialTimeout,
	})
	if err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}
	clearConnInitialDeadline()
	_ = conn.Close()

	if remotePublicKey == nil {
		materialErr := errors.New("failed to read remote host key")
		d.sendConnectFailed((*u)[:], materialErr)
		debugConnectionFailed(d.l, details, materialErr)
		return
	}

	privateKey, privateKeyOK := d.privateKeyForET()
	if !privateKeyOK {
		materialErr := errors.New("ET private key is not available")
		d.sendConnectFailed((*u)[:], materialErr)
		debugConnectionFailed(d.l, details, materialErr)
		return
	}

	knownHostsLine, knownHostsErr := buildETKnownHostsLine(address, remotePublicKey)
	if knownHostsErr != nil {
		d.sendConnectFailed((*u)[:], knownHostsErr)
		debugConnectionFailed(d.l, details, knownHostsErr)
		return
	}

	tempDir, err = os.MkdirTemp("", "shellport-et-*")
	if err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}

	material, err := writeETSSHMaterial(
		tempDir,
		privateKey,
		knownHostsLine,
		address,
	)
	if err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}

	process, err = d.processStarter(d.baseCtx, metadata, user, address, material.ConfigPath)
	if err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionDisconnected(d.l, details, "process start failed", err)
		return
	}
	d.cacheProcess(process)

	connected := false
	readResults := make(chan etProcessReadResult, 1)
	go readETProcessOutput(d.baseCtx, process, len((*u))-d.w.HeaderSize(), readResults)
	startupTimer := time.NewTimer(etProcessStartupGrace)
	defer startupTimer.Stop()
	pendingOutput := make([][]byte, 0)
	for {
		if !connected {
			select {
			case result := <-readResults:
				if result.Err != nil {
					startupErr := buildETStartupError(pendingOutput, result.Err)
					d.sendConnectFailed((*u)[:], startupErr)
					debugConnectionFailed(d.l, details, startupErr)
					return
				}
				if len(result.Data) > 0 {
					pendingOutput = append(pendingOutput, result.Data)
				}
				continue
			case <-startupTimer.C:
				if err = d.emitClientFrame(ETServerConnectSucceed, (*u)[:d.w.HeaderSize()]); err != nil {
					debugConnectionDisconnected(d.l, details, "connect-success response failed", err)
					return
				}
				connected = true
				debugConnectionEstablished(d.l, details)
				for _, data := range pendingOutput {
					if err = d.emitETProcessOutput((*u)[:], data); err != nil {
						debugConnectionDisconnected(d.l, details, "client send failed", err)
						return
					}
				}
				pendingOutput = nil
				continue
			}
		}

		result := <-readResults
		if result.Err != nil {
			debugConnectionDisconnected(d.l, details, "process output ended", result.Err)
			return
		}
		if len(result.Data) == 0 {
			continue
		}

		if err = d.emitETProcessOutput((*u)[:], result.Data); err != nil {
			debugConnectionDisconnected(d.l, details, "client send failed", err)
			return
		}
	}
}

func readETProcessOutput(
	ctx context.Context,
	process etProcess,
	maxReadSize int,
	results chan<- etProcessReadResult,
) {
	for {
		buf := make([]byte, maxReadSize)
		readLen, readErr := process.Read(buf)
		if readLen > 0 {
			if !sendETProcessReadResult(ctx, results, etProcessReadResult{Data: buf[:readLen]}) {
				return
			}
		}
		if readErr != nil {
			_ = sendETProcessReadResult(ctx, results, etProcessReadResult{Err: readErr})
			return
		}
	}
}

func sendETProcessReadResult(
	ctx context.Context,
	results chan<- etProcessReadResult,
	result etProcessReadResult,
) bool {
	select {
	case results <- result:
		return true
	case <-ctx.Done():
		return false
	}
}

func buildETStartupError(output [][]byte, err error) error {
	if len(output) == 0 {
		return err
	}

	var message strings.Builder
	message.WriteString(err.Error())
	message.WriteString(": ")
	for _, chunk := range output {
		message.Write(chunk)
	}
	return errors.New(strings.TrimSpace(message.String()))
}

func (d *etClient) emitETProcessOutput(buf []byte, data []byte) error {
	dataLen := copy(buf[d.w.HeaderSize():], data)
	return d.emitClientFrame(
		ETServerRemoteStdOut,
		buf[:d.w.HeaderSize()+dataLen],
	)
}

func (d *etClient) confirmRemoteFingerprint(
	hostname string,
	remote net.Addr,
	key ssh.PublicKey,
	buf []byte,
) error {
	d.enableRemoteReadTimeoutRetry()
	defer d.disableRemoteReadTimeoutRetry()

	fingerprint := ssh.FingerprintSHA256(key)
	fgpLen := copy(buf[d.w.HeaderSize():], fingerprint)

	if err := d.emitClientFrame(ETServerConnectVerifyFingerprint, buf[:d.w.HeaderSize()+fgpLen]); err != nil {
		return err
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

func (d *etClient) sendConnectFailed(buf []byte, err error) {
	errLen := copy(buf[d.w.HeaderSize():], err.Error()) + d.w.HeaderSize()
	d.emitClientFrame(ETServerConnectFailed, buf[:errLen])
}

func (d *etClient) enableRemoteReadTimeoutRetry() {
	d.remoteReadTimeoutRetryLock.Lock()
	defer d.remoteReadTimeoutRetryLock.Unlock()
	d.remoteReadTimeoutRetry = true
}

func (d *etClient) validateETRemoteAllowed(address string, metadata etMetadata) error {
	if !d.cfg.OnlyAllowPresetRemotes {
		return nil
	}

	presets := d.cfg.Presets
	if d.cfg.PresetRepository != nil {
		presets = d.cfg.PresetRepository.List()
	}
	for _, preset := range presets {
		if preset.Host != address {
			continue
		}
		port, err := etPresetServerPort(preset)
		if err != nil {
			continue
		}
		if port == metadata.ServerPort {
			return nil
		}
	}

	return network.ErrAccessControlDialTargetHostNotAllowed
}

func etPresetServerPort(preset configuration.Preset) (int, error) {
	portText := strings.TrimSpace(preset.Meta["ET Server Port"])
	if portText == "" {
		return etDefaultServerPort, nil
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return 0, ErrETInvalidServerPort
	}
	if err := validateETServerPort(port); err != nil {
		return 0, err
	}
	return port, nil
}

func (d *etClient) disableRemoteReadTimeoutRetry() {
	d.remoteReadTimeoutRetryLock.Lock()
	defer d.remoteReadTimeoutRetryLock.Unlock()
	d.remoteReadTimeoutRetry = false
	d.remoteReadForceRetryNextTimeout = true
}

func (d *etClient) dialRemote(
	networkName string,
	addr string,
	config *ssh.ClientConfig,
) (io.Closer, net.Addr, func(), error) {
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

func buildETKnownHostsLine(address string, key ssh.PublicKey) (string, error) {
	if key == nil {
		return "", errors.New("ET remote host key is required")
	}

	host := address
	if splitHost, splitPort, err := net.SplitHostPort(address); err == nil {
		if splitPort == "22" {
			host = splitHost
		} else {
			host = "[" + splitHost + "]:" + splitPort
		}
	}

	line := knownhosts.Line([]string{host}, key)
	if line == "" {
		return "", errors.New("failed to build known_hosts line")
	}

	return line, nil
}

func (d *etClient) validateProxySupport() error {
	if d.cfg.Socks5Configured {
		return ErrETSocks5Unsupported
	}

	return nil
}

func (d *etClient) buildAuthMethod(
	methodType byte,
	presetID string,
	user string,
	host string,
) (sshAuthMethodBuilder, error) {
	switch methodType {
	case SSHAuthMethodPrivateKey:
		return func(b []byte) []ssh.AuthMethod {
			return []ssh.AuthMethod{
				ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
					if credential, ok := presetPrivateKeyCredential(
						d.cfg,
						"ET",
						presetID,
						user,
						host,
					); ok {
						signer, signerErr := ssh.ParsePrivateKey([]byte(credential))
						if signerErr != nil {
							return nil, signerErr
						}
						d.cachePrivateKey([]byte(credential))
						return []ssh.Signer{signer}, nil
					}

					privateKeyBytes, privateKeyErr := d.requestPrivateKey(b)
					if privateKeyErr != nil {
						return nil, privateKeyErr
					}

					signer, signerErr := ssh.ParsePrivateKey(privateKeyBytes)
					if signerErr != nil {
						return nil, signerErr
					}

					d.cachePrivateKey(privateKeyBytes)

					return []ssh.Signer{signer}, nil
				}),
			}
		}, nil
	default:
		return nil, ErrETUnsupportedAuthMethod
	}
}

func (d *etClient) requestPrivateKey(b []byte) ([]byte, error) {
	if privateKey, ok := d.privateKeyForET(); ok {
		privateKeyClone := make([]byte, len(privateKey))
		copy(privateKeyClone, privateKey)
		return privateKeyClone, nil
	}

	d.enableRemoteReadTimeoutRetry()
	defer d.disableRemoteReadTimeoutRetry()

	var wErr error
	if d.sendCredentialRequest != nil {
		wErr = d.sendCredentialRequest(b[d.w.HeaderSize():])
	} else {
		wErr = d.w.SendManual(
			ETServerConnectRequestCredential,
			b[d.w.HeaderSize():],
		)
	}
	if wErr != nil {
		return nil, wErr
	}

	credentialBytes, credentialOK := <-d.credentialReceive
	if !credentialOK {
		return nil, ErrSSHAuthCancelled
	}

	privateKeyClone := make([]byte, len(credentialBytes))
	copy(privateKeyClone, credentialBytes)
	return privateKeyClone, nil
}

func (d *etClient) cachedPrivateKey() ([]byte, bool) {
	d.privateKeyLock.Lock()
	defer d.privateKeyLock.Unlock()

	if d.privateKey == nil {
		return nil, false
	}

	keyClone := make([]byte, len(d.privateKey))
	copy(keyClone, d.privateKey)
	return keyClone, true
}

func (d *etClient) cachePrivateKey(privateKey []byte) {
	d.privateKeyLock.Lock()
	defer d.privateKeyLock.Unlock()

	privateKeyClone := make([]byte, len(privateKey))
	copy(privateKeyClone, privateKey)
	d.privateKey = privateKeyClone
}

func (d *etClient) privateKeyForET() ([]byte, bool) {
	return d.cachedPrivateKey()
}

func (d *etClient) clearPrivateKey() {
	d.privateKeyLock.Lock()
	defer d.privateKeyLock.Unlock()

	for idx := range d.privateKey {
		d.privateKey[idx] = 0
	}
	d.privateKey = nil
}

func (d *etClient) local(
	f *command.FSM,
	r *rw.LimitedReader,
	h command.StreamHeader,
	b []byte,
) error {
	_ = f

	switch h.Marker() {
	case ETClientStdIn:
		process, ok := d.getProcessIfReady()
		for !r.Completed() {
			data, err := r.Buffered()
			if err != nil {
				return err
			}
			if ok {
				if _, err := process.Write(data); err != nil {
					closeErr := d.closeProcess()
					return errors.Join(err, closeErr)
				}
			}
		}
		return nil

	case ETClientResize:
		process, ok := d.getProcessIfReady()

		if _, rErr := io.ReadFull(r, b[:4]); rErr != nil {
			return rErr
		}
		if !ok {
			return nil
		}

		rows := uint16(b[0])<<8 | uint16(b[1])
		cols := uint16(b[2])<<8 | uint16(b[3])
		return process.Resize(rows, cols)

	case ETClientRespondFingerprint:
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

		verified := rData[0] == 0
		if !d.fingerprintVerifyResultReceiveClosed {
			d.fingerprintVerifyResultReceive <- verified
		}
		d.processedLock.Unlock()
		return nil

	case ETClientRespondCredential:
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

		if !d.credentialReceiveClosed {
			d.credentialReceive <- credentialDataBuf
		}

		d.processedLock.Unlock()
		return nil
	}

	return ErrSSHUnknownClientSignal
}

func (d *etClient) getProcessIfReady() (etProcess, bool) {
	d.processLock.Lock()
	defer d.processLock.Unlock()

	if d.process == nil {
		return nil, false
	}
	return d.process, true
}

func (d *etClient) cacheProcess(process etProcess) {
	d.processLock.Lock()
	defer d.processLock.Unlock()

	d.process = process
}

func (d *etClient) closeProcess() error {
	d.processLock.Lock()
	process := d.process
	d.process = nil
	d.processLock.Unlock()

	if process == nil {
		return nil
	}

	return process.Close()
}

func (d *etClient) Close() error {
	d.processedLock.Lock()
	d.credentialProcessed = true
	d.fingerprintProcessed = true
	d.processedLock.Unlock()

	d.credentialReceiveCloseOnce.Do(func() {
		d.processedLock.Lock()
		d.credentialReceiveClosed = true
		d.processedLock.Unlock()
		close(d.credentialReceive)
	})
	d.fingerprintVerifyResultReceiveCloseOnce.Do(func() {
		d.processedLock.Lock()
		d.fingerprintVerifyResultReceiveClosed = true
		d.processedLock.Unlock()
		close(d.fingerprintVerifyResultReceive)
	})

	d.baseCtxCancel()
	closeErr := d.closeProcess()
	d.clearPrivateKey()
	d.remoteCloseWait.Wait()
	return closeErr
}

func (d *etClient) Release() error {
	closeErr := d.closeProcess()
	d.clearPrivateKey()
	d.baseCtxCancel()
	return closeErr
}
