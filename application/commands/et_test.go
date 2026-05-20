// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
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
