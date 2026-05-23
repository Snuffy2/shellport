// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
	"github.com/Snuffy2/shellport/application/network"
	"github.com/Snuffy2/shellport/application/rw"
)

// Errors
var (
	ErrTelnetUnableToReceiveRemoteConn = errors.New(
		"unable to acquire remote connection handle")
)

// TelnetRequestErrorBadRemoteAddress is the stream error code returned during
// Bootup when the remote address cannot be parsed.
const (
	TelnetRequestErrorBadRemoteAddress = command.StreamError(0x01)
)

// telnetDefaultPortString is the default TCP port appended to preset hosts
// that do not already include an explicit port number.
const (
	telnetDefaultPortString = "23"
)

// Server-to-client stream marker constants.
// TelnetServerRemoteBand carries inbound data from the remote server.
// TelnetServerHookOutputBeforeConnecting forwards hook stdout to the client.
// TelnetServerDialFailed signals that the TCP dial failed.
// TelnetServerDialConnected signals that the TCP connection is established.
const (
	TelnetServerRemoteBand                 = 0x00
	TelnetServerHookOutputBeforeConnecting = 0x01
	TelnetServerDialFailed                 = 0x02
	TelnetServerDialConnected              = 0x03
)

// telnetClient implements command.FSMMachine for a Telnet session. The remote
// goroutine dials the TCP endpoint and forwards received data; the client FSM
// state forwards client frames to the TCP connection.
type telnetClient struct {
	// l is the logger for this session.
	l log.Logger
	// hooks holds hook instances for connection lifecycle events.
	hooks command.Hooks
	// w is the stream responder for sending data to the browser client.
	w command.StreamResponder
	// cfg holds the network dial configuration.
	cfg command.Configuration
	// bufferPool is the shared buffer pool for this session.
	bufferPool *command.BufferPool
	// baseCtx is cancelled when the session should be torn down.
	baseCtx context.Context
	// baseCtxCancel cancels baseCtx; it is wrapped with sync.OnceFunc.
	baseCtxCancel func()
	// remoteChan delivers the established net.Conn from the remote goroutine.
	remoteChan chan net.Conn
	// remoteConn caches the received net.Conn after first retrieval.
	remoteConn net.Conn
	// closeWait is decremented when the remote goroutine exits.
	closeWait sync.WaitGroup
}

// newTelnet is the command.Command factory for the Telnet command. It returns a
// fully initialised telnetClient ready for Bootup.
func newTelnet(
	l log.Logger,
	hooks command.Hooks,
	w command.StreamResponder,
	cfg command.Configuration,
	bufferPool *command.BufferPool,
) command.FSMMachine {
	ctx, ctxCancel := context.WithCancel(context.Background())
	return &telnetClient{
		l:             l,
		hooks:         hooks,
		w:             w,
		cfg:           cfg,
		bufferPool:    bufferPool,
		baseCtx:       ctx,
		baseCtxCancel: sync.OnceFunc(ctxCancel),
		remoteChan:    make(chan net.Conn, 1),
		remoteConn:    nil,
		closeWait:     sync.WaitGroup{},
	}
}

// parseTelnetConfig is the configuration.PresetReloader for the Telnet command.
// It normalises the preset host by appending the default Telnet port when no
// explicit port is present.
func parseTelnetConfig(p configuration.Preset) (configuration.Preset, error) {
	p = configuration.NormalizePresetMeta(p, map[string]string{
		"Encoding": "utf-8",
	})
	return normalizePresetHost(p, telnetDefaultPortString), nil
}

// telnetMaxHostnameLen is the maximum byte length accepted when parsing the
// remote address hostname field during Bootup.
const (
	telnetMaxHostnameLen = 255
)

func (d *telnetClient) Bootup(
	r *rw.LimitedReader,
	b []byte) (command.FSMState, command.FSMError) {
	sBuf := d.bufferPool.Get()
	defer d.bufferPool.Put(sBuf)

	addr, addrErr := ParseAddress(r.Read, (*sBuf)[:telnetMaxHostnameLen])
	if addrErr != nil {
		return nil, command.ToFSMError(
			addrErr, TelnetRequestErrorBadRemoteAddress)
	}

	details := connectionDebugDetails{
		Protocol: "Telnet",
		Address:  addr.String(),
		Network:  "tcp",
	}
	debugConnectionAttempt(d.l, details)

	d.closeWait.Add(1)
	go d.remote(addr.String(), details)

	return d.client, command.NoFSMError()
}

// remote runs in a dedicated goroutine for the lifetime of the Telnet session.
// It executes the before-connecting hooks, dials the remote TCP endpoint, sends
// TelnetServerDialConnected, and then reads inbound data to forward to the
// client. On exit it signals HeaderClose and closes the remoteChan.
func (d *telnetClient) remote(addr string, details connectionDebugDetails) {
	u := d.bufferPool.Get()
	defer d.bufferPool.Put(u)

	defer func() {
		debugConnectionDisconnected(d.l, details, "remote goroutine exited", nil)
		d.w.Signal(command.HeaderClose)
		close(d.remoteChan)
		d.baseCtxCancel()
		d.closeWait.Done()
	}()

	err := d.hooks.Run(
		d.baseCtx,
		configuration.HOOK_BEFORE_CONNECTING,
		command.NewHookParameters(2).
			Insert("Remote Type", "Telnet").
			Insert("Remote Address", addr),
		command.NewDefaultHookOutput(d.l, func(
			b []byte,
		) (wLen int, wErr error) {
			wLen = len(b)
			dLen := copy((*u)[d.w.HeaderSize():], b) + d.w.HeaderSize()
			wErr = d.w.SendManual(
				TelnetServerHookOutputBeforeConnecting,
				(*u)[:dLen],
			)
			return
		}),
	)
	if err != nil {
		errLen := copy((*u)[d.w.HeaderSize():], err.Error()) + d.w.HeaderSize()
		d.w.SendManual(TelnetServerDialFailed, (*u)[:errLen])
		debugConnectionFailed(d.l, details, err)
		return
	}

	dialCtx, dialCtxCancel := context.WithTimeout(d.baseCtx, d.cfg.DialTimeout)
	defer dialCtxCancel()
	d.l.Debug("Dialing Telnet remote: %s", details.fields())
	clientConn, err := d.cfg.Dial(dialCtx, "tcp", addr)
	if err != nil {
		errLen := copy((*u)[d.w.HeaderSize():], err.Error()) + d.w.HeaderSize()
		d.w.SendManual(TelnetServerDialFailed, (*u)[:errLen])
		debugConnectionFailed(d.l, details, err)
		return
	}
	defer clientConn.Close()

	err = d.w.SendManual(TelnetServerDialConnected, (*u)[:d.w.HeaderSize()])
	if err != nil {
		debugConnectionDisconnected(d.l, details, "connect-success response failed", err)
		return
	}
	debugConnectionEstablished(d.l, details)

	// Set timeout for writer, otherwise the Timeout writer will never
	// be triggered
	clientConn.SetWriteDeadline(time.Now().Add(d.cfg.DialTimeout))
	timeoutClientConn := network.NewWriteTimeoutConn(
		clientConn, d.cfg.DialTimeout)

	d.remoteChan <- &timeoutClientConn

	for {
		rLen, err := clientConn.Read((*u)[d.w.HeaderSize():])
		if err != nil {
			debugConnectionDisconnected(d.l, details, "remote read ended", err)
			return
		}

		wErr := d.w.SendManual(
			TelnetServerRemoteBand, (*u)[:rLen+d.w.HeaderSize()])
		if wErr != nil {
			debugConnectionDisconnected(d.l, details, "client send failed", wErr)
			return
		}
	}
}

// getRemote returns the established net.Conn, blocking on remoteChan if the
// remote goroutine has not yet delivered it. Subsequent calls return the cached
// value. It returns ErrTelnetUnableToReceiveRemoteConn if the channel was
// closed before a connection was delivered.
func (d *telnetClient) getRemote() (net.Conn, error) {
	if d.remoteConn != nil {
		return d.remoteConn, nil
	}

	remoteConn, ok := <-d.remoteChan
	if !ok {
		return nil, ErrTelnetUnableToReceiveRemoteConn
	}
	d.remoteConn = remoteConn

	return d.remoteConn, nil
}

// client is the FSMState for the Telnet command's main event loop. It forwards
// all incoming stream data directly to the remote TCP connection without
// interpreting it.
func (d *telnetClient) client(
	f *command.FSM,
	r *rw.LimitedReader,
	h command.StreamHeader,
	b []byte,
) error {
	remoteConn, remoteConnErr := d.getRemote()
	if remoteConnErr != nil {
		return remoteConnErr
	}

	// All Telnet requests are in-band, so we just directly send them all
	// to the server
	for !r.Completed() {
		rBuf, rErr := r.Buffered()
		if rErr != nil {
			return rErr
		}

		_, wErr := remoteConn.Write(rBuf)
		if wErr != nil {
			remoteConn.Close()
			d.l.Debug("Failed to write data to remote: %s", wErr)
			return wErr
		}
	}

	return nil
}

func (d *telnetClient) Close() error {
	d.baseCtxCancel()

	remoteConn, remoteConnErr := d.getRemote()
	if remoteConnErr == nil {
		remoteConn.Close()
	}

	d.closeWait.Wait()
	return nil
}

func (d *telnetClient) Release() error {
	d.baseCtxCancel()
	return nil
}
