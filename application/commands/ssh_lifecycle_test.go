// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"context"
	"testing"
	"time"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
)

// TestSSHCommandKeepsBufferPoolScopedToSession verifies that SSH clients retain
// the per-session buffer pool supplied by the command handler.
func TestSSHCommandKeepsBufferPoolScopedToSession(t *testing.T) {
	bufferPool := command.NewBufferPool(4096)
	poolPtr := &bufferPool

	got := newSSH(
		log.NewDitch(),
		command.NewHooks(configuration.HookSettings{}),
		command.StreamResponder{},
		command.Configuration{},
		poolPtr,
	)
	client, ok := got.(*sshClient)
	if !ok {
		t.Fatalf("expected *sshClient, got %T", got)
	}

	if client.bufferPool != poolPtr {
		t.Fatalf(
			"expected ssh client buffer pool %p, got %p",
			poolPtr,
			client.bufferPool,
		)
	}
}

// TestSSHCloseCancelsBeforeWaitingForRemote verifies Close can unblock remote
// startup paths that only exit after the base context is cancelled.
func TestSSHCloseCancelsBeforeWaitingForRemote(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &sshClient{
		baseCtx:                              ctx,
		baseCtxCancel:                        cancel,
		credentialReceive:                    make(chan []byte),
		fingerprintVerifyResultReceive:       make(chan bool),
		remoteConnReceive:                    make(chan sshRemoteConn),
		credentialReceiveClosed:              false,
		fingerprintVerifyResultReceiveClosed: false,
	}
	client.remoteCloseWait.Add(1)

	go func() {
		<-ctx.Done()
		close(client.remoteConnReceive)
		client.remoteCloseWait.Done()
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
