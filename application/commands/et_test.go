// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"context"
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

	if closeErr := client.Close(); closeErr != nil {
		t.Fatalf("expected close to succeed, got %v", closeErr)
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
