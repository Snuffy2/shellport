// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"context"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
)

// TestTelnetCommandKeepsBufferPoolScopedToSession verifies that Telnet clients
// retain the per-session buffer pool supplied by the command handler.
func TestTelnetCommandKeepsBufferPoolScopedToSession(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	poolPtr := &bufferPool

	r := newTelnet(
		log.NewDitch(),
		command.NewHooks(configuration.HookSettings{}),
		command.StreamResponder{},
		command.Configuration{},
		poolPtr,
	)
	gotType := reflect.TypeOf(r)
	client, ok := r.(*telnetClient)
	if !ok {
		t.Fatalf("expected *telnetClient, got %v", gotType)
	}

	if client.bufferPool != poolPtr {
		t.Fatalf(
			"expected telnet client buffer pool %p, got %p",
			poolPtr,
			client.bufferPool,
		)
	}
}

// TestTelnetCloseCancelsBeforeWaitingForRemote verifies Close can unblock
// remote startup paths that only exit after the base context is cancelled.
func TestTelnetCloseCancelsBeforeWaitingForRemote(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &telnetClient{
		baseCtx:       ctx,
		baseCtxCancel: cancel,
		remoteChan:    make(chan net.Conn),
	}
	client.closeWait.Add(1)

	go func() {
		<-ctx.Done()
		close(client.remoteChan)
		client.closeWait.Done()
	}()

	done := make(chan struct{})
	go func() {
		_ = client.Close()
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Close did not cancel base context before waiting for remote")
	}

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Close did not return after remote shutdown")
	}
}
