// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestCommandsIncludesET(t *testing.T) {
	commands := New()
	expectedNames := map[byte]string{
		0x00: "Telnet",
		0x01: "SSH",
		0x02: "Mosh",
		0x03: "ET",
	}

	for id, expectedName := range expectedNames {
		name := reflect.ValueOf(commands[id]).FieldByName("name").String()
		if name != expectedName {
			t.Fatalf("expected command %d to be %q, got %q", id, expectedName, name)
		}
	}
}

func TestETRejectsSocks5Proxy(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	client := newET(
		log.NewDitch(),
		command.NewHooks(configuration.HookSettings{}),
		command.StreamResponder{},
		command.Configuration{Socks5Configured: true},
		&bufferPool,
	)
	state, err := client.Bootup(nil, nil)
	if state != nil {
		t.Fatalf("expected bootup state to stay nil on SOCKS5 rejection, got %v", state)
	}

	if err.Error() != ErrETSocks5Unsupported.Error() {
		t.Fatalf("expected SOCKS5 bootup error, got %v", err)
	}

	if err.Code() != ETRequestErrorUnsupportedProxy {
		t.Fatalf("expected unsupported proxy code, got %d", err.Code())
	}
}

func TestETAcceptsOnlyPrivateKeyAuth(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	clientMachine := newET(
		log.NewDitch(),
		command.NewHooks(configuration.HookSettings{}),
		command.StreamResponder{},
		command.Configuration{},
		&bufferPool,
	)
	client, ok := clientMachine.(*etClient)
	if !ok {
		t.Fatalf("expected newET to return *etClient")
	}

	authMethodBuilder, authMethodBuilderErr := client.buildAuthMethod(SSHAuthMethodNone, "", "alice", "example.com")
	if authMethodBuilderErr == nil {
		t.Fatal("expected SSHAuthMethodNone to be rejected")
	}
	if authMethodBuilder != nil {
		t.Fatalf("expected nil auth method builder for SSHAuthMethodNone, got %v", authMethodBuilder)
	}
	if authMethodBuilderErr != ErrETUnsupportedAuthMethod {
		t.Fatalf("expected %v, got %v", ErrETUnsupportedAuthMethod, authMethodBuilderErr)
	}

	authMethodBuilder, authMethodBuilderErr = client.buildAuthMethod(SSHAuthMethodPassphrase, "", "alice", "example.com")
	if authMethodBuilderErr == nil {
		t.Fatal("expected SSHAuthMethodPassphrase to be rejected")
	}
	if authMethodBuilder != nil {
		t.Fatalf("expected nil auth method builder for SSHAuthMethodPassphrase, got %v", authMethodBuilder)
	}
	if authMethodBuilderErr != ErrETUnsupportedAuthMethod {
		t.Fatalf("expected %v, got %v", ErrETUnsupportedAuthMethod, authMethodBuilderErr)
	}

	authMethodBuilder, authMethodBuilderErr = client.buildAuthMethod(0xff, "", "alice", "example.com")
	if authMethodBuilderErr == nil {
		t.Fatal("expected unknown auth method to be rejected")
	}
	if authMethodBuilder != nil {
		t.Fatalf("expected nil auth method builder for unknown auth method, got %v", authMethodBuilder)
	}
	if authMethodBuilderErr != ErrETUnsupportedAuthMethod {
		t.Fatalf("expected %v, got %v", ErrETUnsupportedAuthMethod, authMethodBuilderErr)
	}

	authMethodBuilder, authMethodBuilderErr = client.buildAuthMethod(SSHAuthMethodPrivateKey, "", "alice", "example.com")
	if authMethodBuilderErr != nil {
		t.Fatalf("expected SSHAuthMethodPrivateKey to be accepted, got %v", authMethodBuilderErr)
	}
	if authMethodBuilder == nil {
		t.Fatal("expected private-key auth method builder, got nil")
	}
}

func TestETClose(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	clientMachine := newET(
		log.NewDitch(),
		command.NewHooks(configuration.HookSettings{}),
		command.StreamResponder{},
		command.Configuration{},
		&bufferPool,
	)
	client, ok := clientMachine.(*etClient)
	if !ok {
		t.Fatalf("expected newET to return *etClient")
	}

	client.sendCredentialRequest = func([]byte) error {
		return nil
	}

	result := make(chan error, 1)
	go func() {
		_, err := client.requestPrivateKey(make([]byte, 128))
		result <- err
	}()

	proc := &fakeETProcess{stdout: make(chan []byte, 1)}
	client.process = proc
	if closeErr := client.Close(); closeErr != nil {
		t.Fatalf("expected close to succeed, got %v", closeErr)
	}
	if proc.closeCount != 1 {
		t.Fatalf("expected process close once, got %d", proc.closeCount)
	}

	select {
	case err := <-result:
		if err != ErrSSHAuthCancelled {
			t.Fatalf("expected requestPrivateKey to return %v, got %v", ErrSSHAuthCancelled, err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for requestPrivateKey result")
	}
}

func TestETRequestPrivateKeyEnablesTimeoutRetryWhileWaiting(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	client := &etClient{
		bufferPool:                     &bufferPool,
		baseCtx:                        context.Background(),
		baseCtxCancel:                  func() {},
		credentialReceive:              make(chan []byte, 1),
		fingerprintVerifyResultReceive: make(chan bool, 1),
		remoteReadTimeoutRetryLock:     sync.Mutex{},
	}

	retryEnabledInRequest := make(chan bool, 1)
	client.sendCredentialRequest = func([]byte) error {
		client.remoteReadTimeoutRetryLock.Lock()
		retryEnabledInRequest <- client.remoteReadTimeoutRetry
		client.remoteReadTimeoutRetryLock.Unlock()

		client.credentialReceive <- []byte("PRIVATE KEY\n")
		return nil
	}

	done := make(chan error, 1)
	go func() {
		_, err := client.requestPrivateKey(make([]byte, 128))
		done <- err
	}()

	select {
	case enabled := <-retryEnabledInRequest:
		if !enabled {
			t.Fatal("expected remote read-timeout retry to be enabled while requesting credential")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for credential request")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("requestPrivateKey() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for requestPrivateKey result")
	}

	client.remoteReadTimeoutRetryLock.Lock()
	retryEnabledAfterReturn := client.remoteReadTimeoutRetry
	client.remoteReadTimeoutRetryLock.Unlock()
	if retryEnabledAfterReturn {
		t.Fatal("expected remote read-timeout retry to be disabled after requestPrivateKey returns")
	}
}

func TestETBootupRejectsPasswordAuth(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	client := &etClient{
		bufferPool: &bufferPool,
		baseCtx:    context.Background(),
	}

	payload := buildETBootupPayload(
		t,
		"alice",
		"example.com",
		22,
		SSHAuthMethodPassphrase,
		"2022",
		"/usr/local/bin/et",
		"preset-ssh",
	)
	state, fsmErr := client.Bootup(newLimitedReader(payload), make([]byte, 4096))
	if state != nil {
		t.Fatalf("expected bootup state to stay nil on bad auth method, got %v", state)
	}
	if fsmErr.Code() != ETRequestErrorBadAuthMethod {
		t.Fatalf("expected bad auth method code, got %d, error=%v", fsmErr.Code(), fsmErr)
	}
}

func TestETBootupParsesMetadataAndPresetID(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	remoteStarterResult := make(chan string, 1)
	client := &etClient{
		bufferPool:    &bufferPool,
		baseCtx:       context.Background(),
		baseCtxCancel: func() {},
		remoteStarter: func(
			_ string,
			_ string,
			_ sshAuthMethodBuilder,
			metadata etMetadata,
			presetID string,
		) {
			if metadata.ServerPort != 22022 {
				t.Fatalf("remoteStarter metadata.ServerPort = %d, want 22022", metadata.ServerPort)
			}
			remoteStarterResult <- presetID
		},
	}
	payload := buildETBootupPayload(
		t,
		"alice",
		"example.com",
		22,
		SSHAuthMethodPrivateKey,
		"22022",
		"/usr/local/bin/et",
		"preset-et",
	)
	_, fsmErr := client.Bootup(newLimitedReader(payload), make([]byte, 4096))
	if !fsmErr.Succeed() {
		t.Fatalf("expected bootup to succeed, got %v", fsmErr)
	}
	if client.meta.ServerPort != 22022 {
		t.Fatalf("ServerPort = %d, want 22022", client.meta.ServerPort)
	}
	if client.meta.Command != "/usr/local/bin/et" {
		t.Fatalf("Command = %q, want /usr/local/bin/et", client.meta.Command)
	}

	select {
	case remoteStarterPresetID := <-remoteStarterResult:
		if remoteStarterPresetID != "preset-et" {
			t.Fatalf("remoteStarter presetID = %q, want preset-et", remoteStarterPresetID)
		}
	case <-time.After(time.Second):
		t.Fatal("expected remoteStarter to be called")
	}
}

func TestETCloseProcess(t *testing.T) {
	proc := &fakeETProcess{stdout: make(chan []byte, 1)}
	client := &etClient{
		process: proc,
	}

	if err := client.closeProcess(); err != nil {
		t.Fatalf("closeProcess() error = %v", err)
	}
	if proc.closeCount != 1 {
		t.Fatalf("proc.closeCount = %d, want 1", proc.closeCount)
	}
	if client.process != nil {
		t.Fatalf("client.process = %v, want nil after closeProcess", client.process)
	}

	if err := client.closeProcess(); err != nil {
		t.Fatalf("closeProcess() second call error = %v", err)
	}
	if proc.closeCount != 1 {
		t.Fatalf("proc.closeCount = %d, want 1 after second call", proc.closeCount)
	}
}

func TestETCloseProcessReturnsProcessError(t *testing.T) {
	procErr := errors.New("close failed")
	proc := &fakeETProcess{
		stdout:   make(chan []byte, 1),
		closeErr: procErr,
	}
	client := &etClient{process: proc}

	if err := client.closeProcess(); !errors.Is(err, procErr) {
		t.Fatalf("closeProcess() error = %v, want %v", err, procErr)
	}
}

func TestETCloseReturnsProcessCloseError(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	clientMachine := newET(
		log.NewDitch(),
		command.NewHooks(configuration.HookSettings{}),
		command.StreamResponder{},
		command.Configuration{},
		&bufferPool,
	)
	client, ok := clientMachine.(*etClient)
	if !ok {
		t.Fatalf("expected newET to return *etClient")
	}

	procErr := errors.New("process close failed")
	client.process = &fakeETProcess{
		stdout:   make(chan []byte, 1),
		closeErr: procErr,
	}

	client.sendCredentialRequest = func([]byte) error {
		return nil
	}
	result := make(chan error, 1)
	go func() {
		_, err := client.requestPrivateKey(make([]byte, 128))
		result <- err
	}()

	if err := client.Close(); !errors.Is(err, procErr) {
		t.Fatalf("Close() error = %v, want %v", err, procErr)
	}

	select {
	case err := <-result:
		if err != ErrSSHAuthCancelled {
			t.Fatalf("expected requestPrivateKey to return %v, got %v", ErrSSHAuthCancelled, err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for requestPrivateKey result")
	}
}

func TestETReleaseReturnsProcessCloseError(t *testing.T) {
	procErr := errors.New("process close failed")
	client := &etClient{
		process: &fakeETProcess{
			stdout:   make(chan []byte, 1),
			closeErr: procErr,
		},
		baseCtxCancel: func() {},
	}

	if err := client.Release(); !errors.Is(err, procErr) {
		t.Fatalf("Release() error = %v, want %v", err, procErr)
	}
}

func TestETReleaseClosesProcess(t *testing.T) {
	proc := &fakeETProcess{stdout: make(chan []byte, 1)}
	client := &etClient{
		process:       proc,
		baseCtxCancel: func() {},
	}

	if err := client.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if proc.closeCount != 1 {
		t.Fatalf("proc.closeCount = %d, want 1", proc.closeCount)
	}
	if client.process != nil {
		t.Fatalf("client.process = %v, want nil", client.process)
	}
}

func etTestPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()

	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	if err != nil {
		t.Fatalf("build public key: %v", err)
	}

	return publicKey
}

func TestETBuildKnownHostsLineUsesSSHMaterial(t *testing.T) {
	publicKey := etTestPublicKey(t)

	line, err := buildETKnownHostsLine("example.com:22", publicKey)
	if err != nil {
		t.Fatalf("buildETKnownHostsLine() error = %v", err)
	}

	want := knownhosts.Line([]string{"example.com"}, publicKey)
	if line != want {
		t.Fatalf("known hosts line = %q, want %q", line, want)
	}
	if strings.Contains(line, "SHA256") {
		t.Fatalf("known hosts line should include raw host key, got fingerprint: %q", line)
	}
}

func TestETBuildKnownHostsLineUsesPortInKnownHostsAddress(t *testing.T) {
	publicKey := etTestPublicKey(t)

	line, err := buildETKnownHostsLine("example.com:22022", publicKey)
	if err != nil {
		t.Fatalf("buildETKnownHostsLine() error = %v", err)
	}

	want := knownhosts.Line([]string{"[example.com]:22022"}, publicKey)
	if line != want {
		t.Fatalf("known hosts line = %q, want %q", line, want)
	}
}

func TestETBuildKnownHostsLineSupportsDefaultPortIPv6(t *testing.T) {
	publicKey := etTestPublicKey(t)

	line, err := buildETKnownHostsLine("[2001:db8::1]:22", publicKey)
	if err != nil {
		t.Fatalf("buildETKnownHostsLine() error = %v", err)
	}

	want := knownhosts.Line([]string{"2001:db8::1"}, publicKey)
	if line != want {
		t.Fatalf("known hosts line = %q, want %q", line, want)
	}
}

func TestETBuildKnownHostsLineSupportsIPv6HostPort(t *testing.T) {
	publicKey := etTestPublicKey(t)

	line, err := buildETKnownHostsLine("[2001:db8::1]:2022", publicKey)
	if err != nil {
		t.Fatalf("buildETKnownHostsLine() error = %v", err)
	}

	want := knownhosts.Line([]string{"[2001:db8::1]:2022"}, publicKey)
	if line != want {
		t.Fatalf("known hosts line = %q, want %q", line, want)
	}
}

func TestETBuildKnownHostsLineRequiresPublicKey(t *testing.T) {
	if _, err := buildETKnownHostsLine("example.com:22", nil); err == nil {
		t.Fatal("expected buildETKnownHostsLine() to fail with nil key")
	}
}

func TestETRemoteUsesCachedPrivateKeyAndMaterialInProcessStarter(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	privateKey := []byte("PRIVATE KEY\n")
	publicKey := etTestPublicKey(t)
	metadata := defaultETMetadata()
	metadata.Command = "/usr/local/bin/et"
	metadata.ServerPort = 22022
	fingerprintRequest := make(chan struct{}, 1)
	credentialRequest := make(chan struct{}, 1)
	credentialReady := make(chan struct{}, 1)

	etHost := "example.com:22022"
	processStarted := make(chan string, 1)
	clearConn := &testCloseTracker{}
	dialerConn := &testCloseTracker{}
	var materialDir string
	var materialConfigPath string
	process := &fakeETProcess{stdout: make(chan []byte, 1)}
	frames := &capturedFrameSink{}
	client := &etClient{
		w:                              command.StreamResponder{},
		l:                              log.NewDitch(),
		hooks:                          command.NewHooks(configuration.HookSettings{}),
		cfg:                            command.Configuration{DialTimeout: time.Second},
		bufferPool:                     &bufferPool,
		baseCtx:                        context.Background(),
		baseCtxCancel:                  func() {},
		credentialReceive:              make(chan []byte, 1),
		fingerprintVerifyResultReceive: make(chan bool, 1),
		sendToClient:                   false,
		sendFrameHook:                  frames.add,
		remoteDialer: func(_ string, address string, sshConfig *ssh.ClientConfig) (io.Closer, net.Addr, func(), error) {
			if sshConfig.HostKeyCallback == nil {
				t.Fatalf("expected HostKeyCallback")
			}
			fakeAddr := &testNetworkAddress{network: "tcp", str: address}
			fingerprintRequest <- struct{}{}
			if err := sshConfig.HostKeyCallback(address, fakeAddr, publicKey); err != nil {
				return nil, nil, nil, err
			}
			credentialRequest <- struct{}{}
			<-credentialReady
			return dialerConn, fakeAddr, func() {
				_ = clearConn.Close()
			}, nil
		},
		processStarter: func(
			_ context.Context,
			startedMetadata etMetadata,
			user string,
			address string,
			sshConfigPath string,
		) (etProcess, error) {
			if startedMetadata.ServerPort != metadata.ServerPort {
				t.Fatalf(
					"processStarter metadata.ServerPort = %d, want %d",
					startedMetadata.ServerPort,
					metadata.ServerPort,
				)
			}
			if startedMetadata.Command != metadata.Command {
				t.Fatalf(
					"processStarter metadata.Command = %q, want %q",
					startedMetadata.Command,
					metadata.Command,
				)
			}
			if user != "alice" {
				t.Fatalf("user = %q, want %q", user, "alice")
			}
			if address != etHost {
				t.Fatalf("address = %q, want %q", address, etHost)
			}
			if !filepath.IsAbs(sshConfigPath) {
				t.Fatalf("sshConfigPath = %q, must be absolute", sshConfigPath)
			}
			materialConfigPath = sshConfigPath
			materialDir = filepath.Dir(sshConfigPath)

			identityData, err := os.ReadFile(filepath.Join(materialDir, "identity"))
			if err != nil {
				t.Fatalf("read identity material: %v", err)
			}
			if string(identityData) != "PRIVATE KEY\n" {
				t.Fatalf("identity = %q, want %q", identityData, privateKey)
			}
			knownHostsData, err := os.ReadFile(filepath.Join(materialDir, "known_hosts"))
			if err != nil {
				t.Fatalf("read known_hosts material: %v", err)
			}
			wantKnownHostsLine := knownhosts.Line([]string{"[example.com]:22022"}, publicKey)
			gotKnownHostsLine := strings.TrimSpace(string(knownHostsData))
			if gotKnownHostsLine != wantKnownHostsLine {
				t.Fatalf("known_hosts line = %q, want %q", gotKnownHostsLine, wantKnownHostsLine)
			}
			if strings.Contains(gotKnownHostsLine, "SHA256") {
				t.Fatalf("known_hosts should contain raw host key material, got fingerprint line %q", gotKnownHostsLine)
			}
			configText, err := os.ReadFile(materialConfigPath)
			if err != nil {
				t.Fatalf("read ssh_config: %v", err)
			}
			if !strings.Contains(string(configText), "IdentityFile "+filepath.Join(materialDir, "identity")) {
				t.Fatalf("ssh config missing identity file")
			}
			if !strings.Contains(string(configText), "UserKnownHostsFile "+filepath.Join(materialDir, "known_hosts")) {
				t.Fatalf("ssh config missing known_hosts file")
			}

			processStarted <- materialConfigPath
			process.stdout <- []byte("connected\n")
			go func() {
				time.Sleep(10 * time.Millisecond)
				_ = process.Close()
			}()
			return process, nil
		},
	}
	client.remoteCloseWait.Add(1)
	go func() {
		<-fingerprintRequest
		header := command.StreamHeader{}
		header.Set(ETClientRespondFingerprint, 1)
		if err := client.local(nil, newLimitedReader([]byte{0}), header, make([]byte, 16)); err != nil {
			t.Fatalf("local fingerprint response failed: %v", err)
		}
	}()
	go func() {
		<-credentialRequest
		header := command.StreamHeader{}
		header.Set(ETClientRespondCredential, uint16(len(privateKey)))
		if err := client.local(nil, newLimitedReader(privateKey), header, make([]byte, 16)); err != nil {
			t.Fatalf("local credential response failed: %v", err)
		}
		client.cachePrivateKey(privateKey)
		credentialReady <- struct{}{}
	}()

	remoteDone := make(chan struct{})
	go func() {
		client.remote("alice", etHost, func(_ []byte) []ssh.AuthMethod {
			return []ssh.AuthMethod{}
		}, metadata, "preset-et")
		close(remoteDone)
	}()

	select {
	case configPath := <-processStarted:
		if configPath == "" {
			t.Fatal("expected processStarter to receive ssh config path")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for processStarter")
	}

	if !dialerConn.isClosed() {
		t.Fatal("expected remote dialer connection to be closed")
	}
	if !clearConn.isClosed() {
		t.Fatal("expected remote dialer clear timeout callback to run")
	}

	// give the remote goroutine a chance to finish so we can validate cleanup state.
	select {
	case <-remoteDone:
	case <-time.After(2 * time.Second):
		t.Fatal("remote goroutine did not exit")
	}

	if materialDir == "" {
		t.Fatal("expected materialDir to be set")
	}
	if _, err := os.Stat(materialDir); !os.IsNotExist(err) {
		if err != nil {
			t.Fatalf("expected temp material dir to be cleaned up, got stat error: %v", err)
		}
		t.Fatalf("expected temp material dir %q to be removed", materialDir)
	}

	frameFrames := frames.snap()
	sawSucceed := false
	sawStdOut := false
	for _, f := range frameFrames {
		marker, payload, ok := decodeETStreamFrame(f)
		if !ok {
			continue
		}
		switch marker {
		case ETServerConnectSucceed:
			sawSucceed = true
		case ETServerRemoteStdOut:
			if string(payload) == "connected\n" {
				sawStdOut = true
			}
		}
	}
	if !sawSucceed {
		t.Fatal("expected ETServerConnectSucceed frame")
	}
	if !sawStdOut {
		t.Fatal("expected ETServerRemoteStdOut frame with process output")
	}
	if process.closeCount != 1 {
		t.Fatalf("process closeCount = %d, want 1", process.closeCount)
	}
}

func TestETRemoteSendsConnectFailedWhenHandshakeFails(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	frames := &capturedFrameSink{}
	client := &etClient{
		w:             command.StreamResponder{},
		l:             log.NewDitch(),
		hooks:         command.NewHooks(configuration.HookSettings{}),
		cfg:           command.Configuration{DialTimeout: time.Second},
		bufferPool:    &bufferPool,
		baseCtx:       context.Background(),
		baseCtxCancel: func() {},
		sendToClient:  false,
		sendFrameHook: frames.add,
		remoteDialer: func(_ string, _ string, _ *ssh.ClientConfig) (io.Closer, net.Addr, func(), error) {
			return &testCloseTracker{}, &testNetworkAddress{network: "tcp", str: "example.com:22"}, func() {}, errors.New("dial failed")
		},
	}
	client.remoteCloseWait.Add(1)

	remoteDone := make(chan struct{})
	go func() {
		client.remote("alice", "example.com:22", func([]byte) []ssh.AuthMethod {
			return nil
		}, defaultETMetadata(), "preset-et")
		close(remoteDone)
	}()

	select {
	case <-remoteDone:
	case <-time.After(time.Second):
		t.Fatal("remote goroutine did not exit on dial failure")
	}

	snapshot := frames.snap()
	found := false
	for _, f := range snapshot {
		marker, payload, ok := decodeETStreamFrame(f)
		if !ok {
			continue
		}
		if marker == ETServerConnectFailed {
			found = strings.Contains(string(payload), "dial failed")
			break
		}
	}
	if !found {
		t.Fatalf("expected ETServerConnectFailed frame with error, got frames %#v", snapshot)
	}
}

func TestETLocalWriteErrorClosesProcess(t *testing.T) {
	processErr := errors.New("write error")
	proc := &fakeETProcess{
		stdout:   make(chan []byte, 1),
		writeErr: processErr,
	}
	client := &etClient{process: proc}

	stdinHeader := command.StreamHeader{}
	stdinHeader.Set(ETClientStdIn, uint16(5))
	if err := client.local(nil, newLimitedReader([]byte("hello")), stdinHeader, make([]byte, 16)); !errors.Is(err, processErr) {
		t.Fatalf("local() error = %v, want %v", err, processErr)
	}
	if proc.closeCount != 1 {
		t.Fatalf("proc.closeCount = %d, want 1", proc.closeCount)
	}
	if client.process != nil {
		t.Fatalf("client.process = %v, want nil", client.process)
	}
}

func TestETLocalWriteErrorClosesProcessWithCloseError(t *testing.T) {
	processErr := errors.New("write error")
	closeErr := errors.New("close error")
	proc := &fakeETProcess{
		stdout:   make(chan []byte, 1),
		writeErr: processErr,
		closeErr: closeErr,
	}
	client := &etClient{process: proc}

	stdinHeader := command.StreamHeader{}
	stdinHeader.Set(ETClientStdIn, uint16(5))
	err := client.local(nil, newLimitedReader([]byte("hello")), stdinHeader, make([]byte, 16))
	if !errors.Is(err, processErr) {
		t.Fatalf("local() error = %v, want %v", err, processErr)
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("local() error = %v, want %v", err, closeErr)
	}
	if proc.closeCount != 1 {
		t.Fatalf("proc.closeCount = %d, want 1", proc.closeCount)
	}
	if client.process != nil {
		t.Fatalf("client.process = %v, want nil", client.process)
	}
}

func TestETLocalRespondFingerprintWritesVerificationResult(t *testing.T) {
	client := &etClient{
		fingerprintVerifyResultReceive: make(chan bool, 1),
	}

	header := command.StreamHeader{}
	header.Set(ETClientRespondFingerprint, 1)
	if err := client.local(nil, newLimitedReader([]byte{0}), header, make([]byte, 16)); err != nil {
		t.Fatalf("local() error = %v", err)
	}

	select {
	case verified, ok := <-client.fingerprintVerifyResultReceive:
		if !ok {
			t.Fatal("fingerprint verify channel closed unexpectedly")
		}
		if !verified {
			t.Fatal("expected fingerprint byte 0 to verify")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fingerprint verification channel")
	}

	err := client.local(nil, newLimitedReader([]byte{0}), header, make([]byte, 16))
	if !errors.Is(err, ErrSSHUnexpectedFingerprintVerificationRespond) {
		t.Fatalf("expected duplicate response error, got %v", err)
	}
}

func TestETLocalRespondCredentialWritesPrivateKey(t *testing.T) {
	client := &etClient{
		credentialReceive: make(chan []byte, 1),
	}

	privateKey := []byte("PRIVATE KEY\n")
	header := command.StreamHeader{}
	header.Set(ETClientRespondCredential, uint16(len(privateKey)))
	if err := client.local(nil, newLimitedReader(privateKey), header, make([]byte, 16)); err != nil {
		t.Fatalf("local() error = %v", err)
	}

	select {
	case credential, ok := <-client.credentialReceive:
		if !ok {
			t.Fatal("credential channel closed unexpectedly")
		}
		if !bytes.Equal(credential, privateKey) {
			t.Fatalf("credential = %q, want %q", credential, privateKey)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for credential channel")
	}

	err := client.local(nil, newLimitedReader(privateKey), header, make([]byte, 16))
	if !errors.Is(err, ErrSSHUnexpectedCredentialDataRespond) {
		t.Fatalf("expected duplicate credential response error, got %v", err)
	}
}

func TestETLocalRespondAfterCloseRejectsResponses(t *testing.T) {
	client := &etClient{
		credentialReceive:              make(chan []byte, 1),
		fingerprintVerifyResultReceive: make(chan bool, 1),
		baseCtx:                        context.Background(),
		baseCtxCancel:                  func() {},
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	fingerprintHeader := command.StreamHeader{}
	fingerprintHeader.Set(ETClientRespondFingerprint, 1)
	if err := client.local(nil, newLimitedReader([]byte{1}), fingerprintHeader, make([]byte, 16)); !errors.Is(err, ErrSSHUnexpectedFingerprintVerificationRespond) {
		t.Fatalf("expected closed-state duplicate fingerprint response error, got %v", err)
	}

	credentialHeader := command.StreamHeader{}
	credentialHeader.Set(ETClientRespondCredential, 0)
	if err := client.local(nil, newLimitedReader([]byte{}), credentialHeader, make([]byte, 16)); !errors.Is(err, ErrSSHUnexpectedCredentialDataRespond) {
		t.Fatalf("expected closed-state duplicate credential response error, got %v", err)
	}
}

func buildETBootupPayload(
	t *testing.T,
	user string,
	host string,
	sshPort uint16,
	auth byte,
	etPort string,
	etCommand string,
	presetID string,
) []byte {
	t.Helper()

	payload := make([]byte, 0, 256)
	buf := make([]byte, 512)

	userLen, err := NewString([]byte(user)).Marshal(buf)
	if err != nil {
		t.Fatalf("marshal user: %v", err)
	}
	payload = append(payload, buf[:userLen]...)

	addrLen, err := NewAddress(HostNameAddr, []byte(host), sshPort).Marshal(buf)
	if err != nil {
		t.Fatalf("marshal address: %v", err)
	}
	payload = append(payload, buf[:addrLen]...)
	payload = append(payload, auth)
	payload = appendETString(t, payload, etPort)
	payload = appendETString(t, payload, etCommand)
	payload = appendETString(t, payload, presetID)
	return payload
}

func appendETString(t *testing.T, payload []byte, value string) []byte {
	t.Helper()

	buf := make([]byte, MaxInteger+MaxIntegerBytes+len(value))
	valueLen, err := NewString([]byte(value)).Marshal(buf)
	if err != nil {
		t.Fatalf("marshal string %q: %v", value, err)
	}

	return append(payload, buf[:valueLen]...)
}

type fakeETProcess struct {
	stdin      bytes.Buffer
	stdout     chan []byte
	writeErr   error
	resizes    []struct{ rows, cols uint16 }
	closeErr   error
	closeCount int
	closed     bool
	closeLock  sync.Mutex
	stdoutOnce sync.Once
}

type testETWriter struct {
	mu     sync.Mutex
	frames [][]byte
}

type capturedFrameSink struct {
	mu     sync.Mutex
	frames [][]byte
}

func (f *capturedFrameSink) add(marker byte, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	if len(dataCopy) >= 3 {
		header := command.StreamHeader{}
		header.Set(marker, uint16(len(dataCopy)-3))
		dataCopy[1] = header[0]
		dataCopy[2] = header[1]
	}
	f.frames = append(f.frames, dataCopy)
}

func (f *capturedFrameSink) snap() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()

	fr := make([][]byte, len(f.frames))
	for i := range f.frames {
		fr[i] = make([]byte, len(f.frames[i]))
		copy(fr[i], f.frames[i])
	}
	return fr
}

func (w *testETWriter) Write(p []byte) (int, error) {
	pCopy := make([]byte, len(p))
	copy(pCopy, p)

	w.mu.Lock()
	w.frames = append(w.frames, pCopy)
	w.mu.Unlock()

	return len(p), nil
}

func (w *testETWriter) framesCopy() [][]byte {
	w.mu.Lock()
	defer w.mu.Unlock()

	fr := make([][]byte, len(w.frames))
	for i := range w.frames {
		fr[i] = make([]byte, len(w.frames[i]))
		copy(fr[i], w.frames[i])
	}
	return fr
}

func (w *testETWriter) frameBytes() [][]byte {
	return w.framesCopy()
}

func decodeETStreamFrame(data []byte) (marker byte, payload []byte, ok bool) {
	if len(data) < 3 {
		return 0, nil, false
	}
	header := command.StreamHeader{data[1], data[2]}
	payloadLen := int(header.Length())
	if len(data) < payloadLen+3 {
		return 0, nil, false
	}
	return header.Marker(), append([]byte(nil), data[3:3+payloadLen]...), true
}

type testCloseTracker struct {
	mu     sync.Mutex
	closed bool
}

func (c *testCloseTracker) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	return nil
}

func (c *testCloseTracker) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

type testNetworkAddress struct {
	network string
	str     string
}

func (a *testNetworkAddress) Network() string {
	return a.network
}

func (a *testNetworkAddress) String() string {
	return a.str
}

func (f *fakeETProcess) Read(p []byte) (int, error) {
	data, ok := <-f.stdout
	if !ok {
		return 0, io.EOF
	}
	return copy(p, data), nil
}

func (f *fakeETProcess) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.stdin.Write(p)
}

func (f *fakeETProcess) Resize(rows uint16, cols uint16) error {
	f.resizes = append(f.resizes, struct{ rows, cols uint16 }{rows: rows, cols: cols})
	return nil
}

func (f *fakeETProcess) Close() error {
	f.closeLock.Lock()
	defer f.closeLock.Unlock()

	if f.closed {
		return nil
	}
	f.closeCount++
	f.closed = true
	if f.stdout != nil {
		f.stdoutOnce.Do(func() {
			close(f.stdout)
		})
	}
	return f.closeErr
}

func TestETLocalWritesStdinAndResize(t *testing.T) {
	proc := &fakeETProcess{stdout: make(chan []byte, 1)}
	client := &etClient{process: proc}

	stdinHeader := command.StreamHeader{}
	stdinHeader.Set(ETClientStdIn, 5)
	if err := client.local(nil, newLimitedReader([]byte("hello")), stdinHeader, make([]byte, 16)); err != nil {
		t.Fatalf("stdin local error = %v", err)
	}
	if got := proc.stdin.String(); got != "hello" {
		t.Fatalf("stdin = %q, want hello", got)
	}

	resizePayload := []byte{0, 40, 0, 120}
	resizeHeader := command.StreamHeader{}
	resizeHeader.Set(ETClientResize, uint16(len(resizePayload)))
	if err := client.local(nil, newLimitedReader(resizePayload), resizeHeader, make([]byte, 16)); err != nil {
		t.Fatalf("resize local error = %v", err)
	}
	if len(proc.resizes) != 1 || proc.resizes[0].rows != 40 || proc.resizes[0].cols != 120 {
		t.Fatalf("resizes = %#v, want 40x120", proc.resizes)
	}
}
