// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
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
	if f.closed {
		return nil
	}
	f.closeCount++
	f.closed = true
	if f.stdout != nil {
		close(f.stdout)
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
