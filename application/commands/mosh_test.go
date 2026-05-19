// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"bytes"
	"context"
	"errors"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
	"github.com/Snuffy2/shellport/application/network"
	"github.com/Snuffy2/shellport/application/rw"
)

func TestCommandsIncludesMosh(t *testing.T) {
	commands := New()
	expectedNames := map[byte]string{
		0x00: "Telnet",
		0x01: "SSH",
		0x02: "Mosh",
	}

	for id, expectedName := range expectedNames {
		name := reflect.ValueOf(commands[id]).FieldByName("name").String()
		if name != expectedName {
			t.Fatalf("expected command %d to be %q, got %q", id, expectedName, name)
		}
	}
}

func TestParseMoshConfigNormalizesHost(t *testing.T) {
	cfg, err := parseMoshConfig(configuration.Preset{Host: "example.com"})
	if err != nil {
		t.Fatalf("expected config parsing to succeed, got %v", err)
	}

	if cfg.Host != "example.com:22" {
		t.Fatalf("expected normalized host to be %q, got %q", "example.com:22", cfg.Host)
	}
}

func TestMoshBuildRemoteSessionPassesIPv4LiteralUnchanged(t *testing.T) {
	client := &moshClient{
		baseCtx: context.Background(),
		hostResolver: func(context.Context, string) ([]net.IP, error) {
			t.Fatal("expected IPv4 literal to bypass DNS resolution")
			return nil, nil
		},
	}

	var called bool
	client.sessionBuilder = func(host string, port int, key string) (moshSession, error) {
		called = true
		if host != "192.0.2.10" {
			t.Fatalf("expected session builder host to remain IPv4 literal, got %q", host)
		}
		if port != 60001 {
			t.Fatalf("expected port 60001, got %d", port)
		}
		if key != "secret" {
			t.Fatalf("expected key secret, got %q", key)
		}

		return &fakeMoshSession{}, nil
	}

	if _, err := client.buildRemoteSession("192.0.2.10:22", nil, 60001, "secret"); err != nil {
		t.Fatalf("expected remote session build to succeed, got %v", err)
	}

	if !called {
		t.Fatal("expected session builder to be called")
	}
}

func TestMoshBuildRemoteSessionResolvesHostnameToIPv4(t *testing.T) {
	client := &moshClient{
		baseCtx: context.Background(),
		cfg: command.Configuration{
			DialTimeout: 2 * time.Second,
		},
		hostResolver: func(ctx context.Context, host string) ([]net.IP, error) {
			deadline, hasDeadline := ctx.Deadline()
			if !hasDeadline {
				t.Fatal("expected hostname resolution context to carry a deadline")
			}
			if time.Until(deadline) <= 0 {
				t.Fatal("expected hostname resolution deadline to be in the future")
			}
			if host != "example.com" {
				t.Fatalf("expected resolver host example.com, got %q", host)
			}

			return []net.IP{
				net.ParseIP("2001:db8::1"),
				net.ParseIP("198.51.100.23"),
			}, nil
		},
	}

	var resolvedHost string
	client.sessionBuilder = func(host string, port int, key string) (moshSession, error) {
		resolvedHost = host
		return &fakeMoshSession{}, nil
	}

	if _, err := client.buildRemoteSession("example.com:22", nil, 60001, "secret"); err != nil {
		t.Fatalf("expected hostname-backed session build to succeed, got %v", err)
	}

	if resolvedHost != "198.51.100.23" {
		t.Fatalf("expected session builder to receive resolved IPv4 literal, got %q", resolvedHost)
	}
}

func TestMoshBuildRemoteSessionRejectsIPv6OnlyTargets(t *testing.T) {
	client := &moshClient{
		baseCtx: context.Background(),
		hostResolver: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("2001:db8::1")}, nil
		},
		sessionBuilder: func(string, int, string) (moshSession, error) {
			t.Fatal("expected IPv6-only target to fail before session dial")
			return nil, nil
		},
	}

	_, err := client.buildRemoteSession("example.com:22", nil, 60001, "secret")
	if err == nil {
		t.Fatal("expected IPv6-only target to be rejected")
	}
}

func TestMoshBuildRemoteSessionUsesReachedPeerIPv4WithoutResolver(t *testing.T) {
	client := &moshClient{
		baseCtx: context.Background(),
		hostResolver: func(context.Context, string) ([]net.IP, error) {
			t.Fatal("expected reached peer IP to bypass DNS resolution")
			return nil, nil
		},
	}

	var resolvedHost string
	client.sessionBuilder = func(host string, port int, key string) (moshSession, error) {
		resolvedHost = host
		if port != 60001 {
			t.Fatalf("expected port 60001, got %d", port)
		}
		if key != "secret" {
			t.Fatalf("expected key secret, got %q", key)
		}

		return &fakeMoshSession{}, nil
	}

	peer := &net.TCPAddr{IP: net.ParseIP("198.51.100.23"), Port: 22}
	if _, err := client.buildRemoteSession("example.com:22", peer, 60001, "secret"); err != nil {
		t.Fatalf("expected peer-backed session build to succeed, got %v", err)
	}

	if resolvedHost != "198.51.100.23" {
		t.Fatalf("expected session builder to receive reached peer IPv4, got %q", resolvedHost)
	}
}

func TestMoshValidateRemoteAllowedHonorsPresetRestriction(t *testing.T) {
	client := &moshClient{
		baseCtx: context.Background(),
		cfg: command.Configuration{
			OnlyAllowPresetRemotes: true,
			Presets: []configuration.Preset{
				{Host: "example.com:22"},
			},
		},
	}

	err := client.validateMoshRemoteAllowed("other.example.com:22")
	if !errors.Is(err, network.ErrAccessControlDialTargetHostNotAllowed) {
		t.Fatalf(
			"expected preset restriction error %v, got %v",
			network.ErrAccessControlDialTargetHostNotAllowed,
			err,
		)
	}
}

func TestMoshValidateRemoteAllowedUsesLivePresetRepository(t *testing.T) {
	repo := configuration.NewPresetRepository([]configuration.Preset{
		{Host: "example.com:22"},
	})
	client := &moshClient{
		baseCtx: context.Background(),
		cfg: command.Configuration{
			OnlyAllowPresetRemotes: true,
			Presets: []configuration.Preset{
				{Host: "example.com:22"},
			},
			PresetRepository: repo,
		},
	}
	repo.Replace([]configuration.Preset{
		{Host: "other.example.com:22"},
	})

	if err := client.validateMoshRemoteAllowed("other.example.com:22"); err != nil {
		t.Fatalf("expected live preset repository host to be allowed, got %v", err)
	}
}

func TestMoshBuildRemoteSessionDoesNotApplyPresetRestrictionAfterBootstrap(t *testing.T) {
	client := &moshClient{
		baseCtx: context.Background(),
		cfg: command.Configuration{
			OnlyAllowPresetRemotes: true,
			Presets: []configuration.Preset{
				{Host: "example.com:22"},
			},
		},
		hostResolver: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("198.51.100.23")}, nil
		},
	}

	var called bool
	client.sessionBuilder = func(host string, port int, key string) (moshSession, error) {
		called = true
		if host != "198.51.100.23" {
			t.Fatalf("expected resolved session host, got %q", host)
		}
		return &fakeMoshSession{}, nil
	}

	if _, err := client.buildRemoteSession("other.example.com:22", nil, 60001, "secret"); err != nil {
		t.Fatalf("expected session build to skip preset validation after bootstrap, got %v", err)
	}
	if !called {
		t.Fatal("expected session builder to be called")
	}
}

func TestMoshSSHBootstrapNetworkUsesIPv4DialForHostnames(t *testing.T) {
	networkName, err := moshSSHBootstrapNetwork("example.com:22")
	if err != nil {
		t.Fatalf("expected SSH bootstrap network selection to succeed, got %v", err)
	}

	if networkName != "tcp4" {
		t.Fatalf("expected tcp4 bootstrap network, got %q", networkName)
	}
}

func TestMoshSSHBootstrapNetworkRejectsIPv6Literal(t *testing.T) {
	_, err := moshSSHBootstrapNetwork("[2001:db8::1]:22")
	if err == nil {
		t.Fatal("expected IPv6 literal bootstrap address to be rejected")
	}
}

func TestMoshAwaitRemoteSessionReadyReturnsInitialOutput(t *testing.T) {
	client := &moshClient{
		baseCtx: context.Background(),
		cfg:     command.Configuration{DialTimeout: 250 * time.Millisecond},
	}
	session := &fakeMoshSession{
		awaitReady: func(ctx context.Context, timeout time.Duration) ([]byte, error) {
			if timeout != 250*time.Millisecond {
				t.Fatalf("expected readiness timeout 250ms, got %s", timeout)
			}
			if ctx == nil {
				t.Fatal("expected readiness context to be provided")
			}

			return []byte("ready"), nil
		},
	}

	output, err := client.awaitRemoteSessionReady(session)
	if err != nil {
		t.Fatalf("expected readiness to succeed, got %v", err)
	}

	if string(output) != "ready" {
		t.Fatalf("expected readiness output %q, got %q", "ready", output)
	}
}

func TestMoshAwaitRemoteSessionReadyFailsWithoutServerResponse(t *testing.T) {
	client := &moshClient{
		baseCtx: context.Background(),
		cfg:     command.Configuration{DialTimeout: 250 * time.Millisecond},
	}
	session := &fakeMoshSession{
		awaitReady: func(context.Context, time.Duration) ([]byte, error) {
			return nil, errors.New("timeout waiting for mosh server response")
		},
	}

	output, err := client.awaitRemoteSessionReady(session)
	if err == nil {
		t.Fatal("expected readiness failure to propagate")
	}

	if output != nil {
		t.Fatalf("expected nil output on readiness failure, got %q", output)
	}
}

func TestMoshAwaitRemoteSessionReadyAllowsQuietSession(t *testing.T) {
	client := &moshClient{
		baseCtx: context.Background(),
		cfg:     command.Configuration{DialTimeout: 250 * time.Millisecond},
	}
	session := &fakeMoshSession{
		awaitReady: func(ctx context.Context, timeout time.Duration) ([]byte, error) {
			if timeout != 250*time.Millisecond {
				t.Fatalf("expected readiness timeout 250ms, got %s", timeout)
			}
			if ctx == nil {
				t.Fatal("expected readiness context to be provided")
			}

			return nil, nil
		},
	}

	output, err := client.awaitRemoteSessionReady(session)
	if err != nil {
		t.Fatalf("expected quiet readiness to succeed, got %v", err)
	}

	if output != nil {
		t.Fatalf("expected nil initial output for quiet readiness, got %q", output)
	}
}

func TestParseMoshServerDetachedPID(t *testing.T) {
	output := `MOSH CONNECT 60002 6HuXgr4e2ThPQW14LBDYCw

mosh-server (mosh 1.4.0)
[mosh-server detached, pid = 63458]`

	pid, ok := parseMoshServerDetachedPID(output)
	if !ok {
		t.Fatal("expected detached mosh-server PID to parse")
	}

	if pid != 63458 {
		t.Fatalf("expected PID 63458, got %d", pid)
	}
}

func TestParseMoshServerDetachedPIDAllowsMissingPID(t *testing.T) {
	pid, ok := parseMoshServerDetachedPID("MOSH CONNECT 60002 key")
	if ok {
		t.Fatal("expected missing detached PID to be reported")
	}

	if pid != 0 {
		t.Fatalf("expected PID 0 when missing, got %d", pid)
	}
}

func TestRenderMoshServerMonitorCommandUsesPOSIXShell(t *testing.T) {
	commandText := renderMoshServerMonitorCommand(63458)
	want := `sh -c 'while kill -0 63458 2>/dev/null; do sleep 1; done; printf "%s\n" shellport-mosh-server-exited'`
	if commandText != want {
		t.Fatalf("expected POSIX shell monitor command %q, got %q", want, commandText)
	}
}

func TestMoshRemoteMonitorExitClosesSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := newBlockingFakeMoshSession()
	client := &moshClient{
		baseCtx:       ctx,
		baseCtxCancel: cancel,
		l:             log.Ditch{},
		session:       session,
	}
	monitorDone := make(chan bool, 1)
	monitorDone <- true

	client.closeSessionWhenRemoteMoshServerExits(monitorDone)

	if !session.closed {
		t.Fatal("expected ended remote mosh-server to close the local session")
	}

	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected ended remote mosh-server to cancel the base context")
	}
}

func TestMoshRemoteMonitorFailureKeepsSessionOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := newBlockingFakeMoshSession()
	client := &moshClient{
		baseCtx:       ctx,
		baseCtxCancel: cancel,
		l:             log.Ditch{},
		session:       session,
	}
	monitorDone := make(chan bool, 1)
	monitorDone <- false

	client.closeSessionWhenRemoteMoshServerExits(monitorDone)

	if session.closed {
		t.Fatal("expected monitor failure to leave the local session open")
	}

	select {
	case <-ctx.Done():
		t.Fatal("expected monitor failure to leave the base context active")
	default:
	}
}

func TestMoshBootupRejectsSocks5(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	client := newMosh(
		nil,
		command.NewHooks(configuration.HookSettings{}),
		command.StreamResponder{},
		command.Configuration{Socks5Configured: true},
		&bufferPool,
	)

	state, err := client.Bootup(nil, nil)
	if state != nil {
		t.Fatalf("expected bootup state to stay nil on SOCKS5 rejection, got %v", state)
	}

	if err.Error() != ErrMoshSocks5Unsupported.Error() {
		t.Fatalf("expected SOCKS5 bootup error, got %v", err)
	}

	if err.Code() != MoshRequestErrorUnsupportedProxy {
		t.Fatalf("expected unsupported proxy code, got %d", err.Code())
	}
}

func TestMoshBootupReportsBadMetadata(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	client := newMosh(
		nil,
		command.NewHooks(configuration.HookSettings{}),
		command.StreamResponder{},
		command.Configuration{},
		&bufferPool,
	)
	payload := buildMoshBootPayload(t, "alice", "example.com", 22, SSHAuthMethodNone)
	payload = append(payload, 5)

	state, err := client.Bootup(newLimitedReader(payload), nil)
	if state != nil {
		t.Fatalf("expected bootup state to stay nil on metadata rejection, got %v", state)
	}

	if err.Code() != MoshRequestErrorBadMetadata {
		t.Fatalf("expected bad metadata code, got %d", err.Code())
	}
}

func TestMoshBootupRejectsMoshServerArguments(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	client := newMosh(
		nil,
		command.NewHooks(configuration.HookSettings{}),
		command.StreamResponder{},
		command.Configuration{},
		&bufferPool,
	)
	payload := buildMoshBootPayload(t, "alice", "example.com", 22, SSHAuthMethodNone)
	payload = appendMoshString(t, payload, "/usr/local/bin/mosh-server --flag")

	state, err := client.Bootup(newLimitedReader(payload), nil)
	if state != nil {
		t.Fatalf("expected bootup state to stay nil on metadata rejection, got %v", state)
	}

	if err.Code() != MoshRequestErrorBadMetadata {
		t.Fatalf("expected bad metadata code, got %d", err.Code())
	}
}

func TestMoshBootupCopiesUsernameBeforeReusingScratchBuffer(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	machine := newMosh(
		log.Ditch{},
		command.NewHooks(configuration.HookSettings{}),
		command.StreamResponder{},
		command.Configuration{},
		&bufferPool,
	)
	client, ok := machine.(*moshClient)
	if !ok {
		t.Fatal("expected newMosh to return *moshClient")
	}

	remoteStarted := make(chan string, 1)
	client.remoteStarter = func(user string, address string, authMethodBuilder sshAuthMethodBuilder) {
		client.remoteCloseWait.Done()
		remoteStarted <- user
	}

	payload := buildMoshBootPayload(t, "pi", "atlantis.home", 22, SSHAuthMethodPrivateKey)
	payload = appendMoshString(t, payload, "mosh-server")

	state, err := client.Bootup(newLimitedReader(payload), nil)
	if err != command.NoFSMError() {
		t.Fatalf("expected bootup to succeed, got %v", err)
	}
	if state == nil {
		t.Fatal("expected bootup state")
	}

	select {
	case user := <-remoteStarted:
		if user != "pi" {
			t.Fatalf("expected copied username %q, got %q", "pi", user)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected remote startup")
	}
}

func TestParseMoshRequestMetaAcceptsMoshServerPath(t *testing.T) {
	payload := appendMoshString(t, nil, "/usr/local/bin/mosh-server")

	meta, err := parseMoshRequestMeta(newLimitedReader(payload), make([]byte, 128))
	if err != nil {
		t.Fatalf("expected mosh server path metadata to parse, got %v", err)
	}

	if meta["Mosh Server"] != "/usr/local/bin/mosh-server" {
		t.Fatalf("expected custom mosh server path, got %q", meta["Mosh Server"])
	}
}

func TestMoshLocalStdInWritesBufferedBytes(t *testing.T) {
	session := &fakeMoshSession{}
	client := &moshClient{session: session}

	if err := client.local(
		nil,
		newLimitedReader([]byte("typed input")),
		command.StreamHeader{MoshClientStdIn << 5, 0},
		make([]byte, 4),
	); err != nil {
		t.Fatalf("expected stdin handler to succeed, got %v", err)
	}

	if len(session.sent) != 1 {
		t.Fatalf("expected one send call, got %d", len(session.sent))
	}

	if !bytes.Equal(session.sent[0], []byte("typed input")) {
		t.Fatalf("expected sent payload %q, got %q", "typed input", session.sent[0])
	}
}

func TestMoshLocalStdInBeforeSessionDoesNotBlock(t *testing.T) {
	client := &moshClient{sessionReceive: make(chan moshSession, 1)}

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.local(
			nil,
			newLimitedReader([]byte("typed input")),
			command.StreamHeader{MoshClientStdIn << 5, 0},
			make([]byte, 4),
		)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected pre-session stdin to be discarded without error, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected pre-session stdin handler not to block")
	}
}

func TestMoshLocalResizeBeforeSessionDoesNotBlock(t *testing.T) {
	client := &moshClient{sessionReceive: make(chan moshSession, 1)}

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.local(
			nil,
			newLimitedReader([]byte{0x00, 0x18, 0x00, 0x50}),
			command.StreamHeader{MoshClientResize << 5, 0x04},
			make([]byte, 4),
		)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected pre-session resize to be discarded without error, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected pre-session resize handler not to block")
	}
}

func TestMoshCloseClosesBufferedSessionWithoutWaitingForReceiveTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := newBlockingFakeMoshSession()
	client := &moshClient{
		baseCtx:                        ctx,
		baseCtxCancel:                  cancel,
		credentialReceive:              make(chan []byte, 1),
		fingerprintVerifyResultReceive: make(chan bool, 1),
		sessionReceive:                 make(chan moshSession, 1),
		remoteCloseWait:                sync.WaitGroup{},
		remoteReadTimeoutRetryLock:     sync.Mutex{},
	}
	client.sessionReceive <- session
	client.cacheSession(session)

	client.remoteCloseWait.Add(1)
	go func() {
		defer client.remoteCloseWait.Done()
		<-session.closedCh
	}()

	done := make(chan error, 1)
	go func() {
		done <- client.Close()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected close to succeed, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected close to return promptly after closing buffered session")
	}

	if !session.closed {
		t.Fatal("expected buffered session to be closed")
	}
}

func TestMoshCloseCancelsBlockedReadinessAndClosesCachedSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := newBlockingFakeMoshSession()
	client := &moshClient{
		baseCtx:                        ctx,
		baseCtxCancel:                  cancel,
		cfg:                            command.Configuration{DialTimeout: 5 * time.Second},
		credentialReceive:              make(chan []byte, 1),
		fingerprintVerifyResultReceive: make(chan bool, 1),
		sessionReceive:                 make(chan moshSession, 1),
		remoteCloseWait:                sync.WaitGroup{},
		remoteReadTimeoutRetryLock:     sync.Mutex{},
	}
	entered := make(chan struct{})
	session.awaitReady = func(ctx context.Context, timeout time.Duration) ([]byte, error) {
		if timeout != 5*time.Second {
			t.Fatalf("expected readiness timeout 5s, got %s", timeout)
		}
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}

	client.cacheSession(session)

	readinessDone := make(chan error, 1)
	go func() {
		_, err := client.awaitRemoteSessionReady(session)
		readinessDone <- err
	}()

	select {
	case <-entered:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected readiness wait to start")
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- client.Close()
	}()

	select {
	case err := <-readinessDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected readiness cancellation, got %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected readiness wait to cancel promptly")
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("expected close to succeed, got %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected close to return promptly while readiness was blocked")
	}

	if !session.closed {
		t.Fatal("expected cached session to be closed")
	}
}

func TestMoshLocalStdInStopsAfterSendError(t *testing.T) {
	session := &fakeMoshSession{sendErr: errors.New("send failed")}
	client := &moshClient{
		l:       log.Ditch{},
		session: session,
	}

	err := client.local(
		nil,
		newLimitedReader([]byte("typed input")),
		command.StreamHeader{MoshClientStdIn << 5, 0},
		make([]byte, 4),
	)
	if !errors.Is(err, session.sendErr) {
		t.Fatalf("expected send error to be returned, got %v", err)
	}

	if len(session.sent) != 1 {
		t.Fatalf("expected one send attempt, got %d", len(session.sent))
	}

	if !session.closed {
		t.Fatal("expected failed session to be closed")
	}
}

type fakeMoshResize struct {
	cols uint16
	rows uint16
}

type recordedLogEntry struct {
	level   string
	message string
}

type recordingLogger struct {
	entries []recordedLogEntry
}

func (r *recordingLogger) Context(string) log.Logger {
	return r
}

func (r *recordingLogger) TitledContext(string, ...any) log.Logger {
	return r
}

func (r *recordingLogger) Write(b []byte) (int, error) {
	return len(b), nil
}

func (r *recordingLogger) Info(msg string, params ...any) {
	r.record("info", msg, params...)
}

func (r *recordingLogger) Debug(msg string, params ...any) {
	r.record("debug", msg, params...)
}

func (r *recordingLogger) Warning(msg string, params ...any) {
	r.record("warning", msg, params...)
}

func (r *recordingLogger) Error(msg string, params ...any) {
	r.record("error", msg, params...)
}

func (r *recordingLogger) record(level string, msg string, params ...any) {
	r.entries = append(r.entries, recordedLogEntry{
		level:   level,
		message: msg,
	})
}

func TestMoshLifecycleMonitorErrorsUseWarningLogs(t *testing.T) {
	logger := &recordingLogger{}
	err := errors.New("monitor failed")

	logRemoteMoshServerLifecycleUnavailable(logger, err)
	logRemoteMoshServerMonitorError(context.Background(), logger, err)
	logRemoteMoshServerCloseError(logger, err)

	if len(logger.entries) != 3 {
		t.Fatalf("expected 3 lifecycle log entries, got %d", len(logger.entries))
	}

	for _, entry := range logger.entries {
		if entry.level != "warning" {
			t.Fatalf("expected lifecycle error log level warning, got %q for %q", entry.level, entry.message)
		}
	}
}

func TestMoshLifecycleMonitorErrorAfterCancellationDoesNotWarn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	logger := &recordingLogger{}

	logRemoteMoshServerMonitorError(ctx, logger, errors.New("monitor failed"))

	if len(logger.entries) != 0 {
		t.Fatalf("expected canceled monitor error to be suppressed, got %#v", logger.entries)
	}
}

type fakeMoshSession struct {
	mu         sync.Mutex
	sent       [][]byte
	resizes    []fakeMoshResize
	closed     bool
	closedCh   chan struct{}
	sendErr    error
	awaitReady func(context.Context, time.Duration) ([]byte, error)
}

func (f *fakeMoshSession) Send(payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, append([]byte(nil), payload...))
	return f.sendErr
}

func (f *fakeMoshSession) Recv(time.Duration) ([]byte, error) {
	return nil, nil
}

func (f *fakeMoshSession) AwaitReady(ctx context.Context, timeout time.Duration) ([]byte, error) {
	if f.awaitReady != nil {
		return f.awaitReady(ctx, timeout)
	}

	return []byte("ready"), nil
}

func (f *fakeMoshSession) Resize(cols uint16, rows uint16) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resizes = append(f.resizes, fakeMoshResize{cols: cols, rows: rows})
	return nil
}

func (f *fakeMoshSession) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	if f.closedCh != nil {
		close(f.closedCh)
	}
	return nil
}

func newBlockingFakeMoshSession() *fakeMoshSession {
	return &fakeMoshSession{
		closedCh: make(chan struct{}),
	}
}

func newLimitedReader(payload []byte) *rw.LimitedReader {
	reader := rw.NewFetchReader(func() ([]byte, error) {
		return payload, nil
	})
	limited := rw.NewLimitedReader(&reader, len(payload))
	return &limited
}

func buildMoshBootPayload(
	t *testing.T,
	user string,
	host string,
	port uint16,
	authMethod byte,
) []byte {
	t.Helper()

	payload := make([]byte, 0, 128)
	buf := make([]byte, 128)

	userLen, err := NewString([]byte(user)).Marshal(buf)
	if err != nil {
		t.Fatalf("expected username marshal to succeed, got %v", err)
	}
	payload = append(payload, buf[:userLen]...)

	addrLen, err := NewAddress(HostNameAddr, []byte(host), port).Marshal(buf)
	if err != nil {
		t.Fatalf("expected address marshal to succeed, got %v", err)
	}
	payload = append(payload, buf[:addrLen]...)
	payload = append(payload, authMethod)

	return payload
}

func appendMoshString(t *testing.T, payload []byte, value string) []byte {
	t.Helper()

	buf := make([]byte, MaxInteger+MaxIntegerBytes)
	valueLen, err := NewString([]byte(value)).Marshal(buf)
	if err != nil {
		t.Fatalf("expected string marshal to succeed, got %v", err)
	}

	return append(payload, buf[:valueLen]...)
}
