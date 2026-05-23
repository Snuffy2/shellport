// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"context"
	"errors"
	"io"
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

type failingTelnetConn struct {
	net.Conn
	writeErr error
	closed   bool
}

func (c *failingTelnetConn) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (c *failingTelnetConn) Write(_ []byte) (int, error) {
	return 0, c.writeErr
}

func (c *failingTelnetConn) Close() error {
	c.closed = true
	return nil
}

func (c *failingTelnetConn) LocalAddr() net.Addr {
	return nil
}

func (c *failingTelnetConn) RemoteAddr() net.Addr {
	return nil
}

func (c *failingTelnetConn) SetDeadline(_ time.Time) error {
	return nil
}

func (c *failingTelnetConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (c *failingTelnetConn) SetWriteDeadline(_ time.Time) error {
	return nil
}

// TestTelnetClientReturnsRemoteWriteErrors verifies stdin write failures
// surface to the stream handler instead of leaving the UI connected to dead IO.
func TestTelnetClientReturnsRemoteWriteErrors(t *testing.T) {
	writeErr := errors.New("remote write failed")
	remote := &failingTelnetConn{writeErr: writeErr}
	client := &telnetClient{l: log.NewDitch(), remoteConn: remote}

	err := client.client(
		nil,
		newLimitedReader([]byte("hello")),
		command.StreamHeader{},
		make([]byte, 16),
	)

	if !errors.Is(err, writeErr) {
		t.Fatalf("expected remote write error, got %v", err)
	}
	if !remote.closed {
		t.Fatal("expected remote connection to close after write failure")
	}
}
