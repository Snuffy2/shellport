// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Snuffy2/shellport/application/configuration"
)

type fakeWebsocketConnection struct {
	mu                  sync.Mutex
	readDeadlines       []time.Time
	writeDeadlines      []time.Time
	writeMessageTypes   []int
	pongHandler         func(string) error
	writeMessageErr     error
	setReadDeadlineErr  error
	setWriteDeadlineErr error
	closeErr            error
	closeCh             chan struct{}
	closeOnce           sync.Once
	setReadDeadlineFn   func(time.Time) error
	setWriteDeadlineFn  func(time.Time) error
	writeMessageFn      func(int, []byte) error
}

func newFakeWebsocketConnection() *fakeWebsocketConnection {
	return &fakeWebsocketConnection{closeCh: make(chan struct{}, 1)}
}

func (f *fakeWebsocketConnection) SetReadDeadline(t time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readDeadlines = append(f.readDeadlines, t)
	if f.setReadDeadlineFn != nil {
		return f.setReadDeadlineFn(t)
	}
	return f.setReadDeadlineErr
}

func (f *fakeWebsocketConnection) SetWriteDeadline(t time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeDeadlines = append(f.writeDeadlines, t)
	if f.setWriteDeadlineFn != nil {
		return f.setWriteDeadlineFn(t)
	}
	return f.setWriteDeadlineErr
}

func (f *fakeWebsocketConnection) SetPongHandler(h func(string) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pongHandler = h
}

func (f *fakeWebsocketConnection) WriteMessage(messageType int, b []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeMessageTypes = append(f.writeMessageTypes, messageType)
	if f.writeMessageFn != nil {
		return f.writeMessageFn(messageType, b)
	}
	return f.writeMessageErr
}

func waitForWriteEvent(writeCh <-chan struct{}, timeout time.Duration, t *testing.T) {
	t.Helper()

	select {
	case <-writeCh:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for write event")
	}
}

func readWriteEvent(writeCh <-chan string, timeout time.Duration, t *testing.T) string {
	t.Helper()

	select {
	case event := <-writeCh:
		return event
	case <-time.After(timeout):
		t.Fatal("timed out waiting for write event")
	}
	return ""
}

func (f *fakeWebsocketConnection) Close() error {
	f.closeOnce.Do(func() {
		close(f.closeCh)
	})
	return f.closeErr
}

type fakeWebsocketTicker struct {
	ch      chan time.Time
	stopped bool
}

func (f *fakeWebsocketTicker) C() <-chan time.Time {
	return f.ch
}

func (f *fakeWebsocketTicker) Stop() {
	f.stopped = true
}

func newFakeWebsocketTicker() (*fakeWebsocketTicker, func(time.Duration) websocketTicker) {
	ft := &fakeWebsocketTicker{ch: make(chan time.Time, 1)}
	factory := func(d time.Duration) websocketTicker {
		return ft
	}
	return ft, factory
}

func TestConfigureWebsocketLivenessExtendsReadDeadlineOnPong(t *testing.T) {
	conn := newFakeWebsocketConnection()
	fixedNow := time.Unix(1_750_000_000, 0)
	fs, tickerFactory := newFakeWebsocketTicker()
	controller := socket{serverCfg: configuration.Server{ReadTimeout: 7 * time.Second}}

	stop, err := controller.configureWebsocketLiveness(
		conn,
		func() time.Time { return fixedNow },
		tickerFactory,
		nil,
	)
	if err != nil {
		t.Fatalf("configureWebsocketLiveness returned error: %v", err)
	}
	defer stop()

	if len(conn.readDeadlines) != 1 {
		t.Fatalf("readDeadline calls = %d, want 1", len(conn.readDeadlines))
	}
	if conn.readDeadlines[0] != fixedNow.Add(7*time.Second) {
		t.Fatalf("initial read deadline = %v, want %v", conn.readDeadlines[0], fixedNow.Add(7*time.Second))
	}

	conn.mu.Lock()
	handler := conn.pongHandler
	conn.mu.Unlock()
	if handler == nil {
		t.Fatal("pong handler not set")
	}
	if err := handler("pong"); err != nil {
		t.Fatalf("pong handler returned error: %v", err)
	}
	if len(conn.readDeadlines) != 2 {
		t.Fatalf("readDeadline calls after pong = %d, want 2", len(conn.readDeadlines))
	}
	if conn.readDeadlines[1] != fixedNow.Add(7*time.Second) {
		t.Fatalf("pong read deadline = %v, want %v", conn.readDeadlines[1], fixedNow.Add(7*time.Second))
	}

	if fs.stopped {
		t.Fatalf("ticker stopped too early")
	}
}

func TestConfigureWebsocketLivenessClosesConnectionOnPingWriteFailure(t *testing.T) {
	conn := newFakeWebsocketConnection()
	conn.writeMessageErr = errors.New("ping write failed")
	ticker := &fakeWebsocketTicker{ch: make(chan time.Time, 1)}
	controller := socket{serverCfg: configuration.Server{
		ReadTimeout:      2 * time.Second,
		WriteTimeout:     1 * time.Second,
		HeartbeatTimeout: 1 * time.Second,
	}}

	tickerFactory := func(_ time.Duration) websocketTicker {
		return ticker
	}
	stop, err := controller.configureWebsocketLiveness(
		conn,
		func() time.Time { return time.Unix(1_750_000_000, 0) },
		tickerFactory,
		nil,
	)
	if err != nil {
		t.Fatalf("configureWebsocketLiveness returned error: %v", err)
	}
	defer stop()
	ticker.ch <- time.Now()

	select {
	case <-conn.closeCh:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected websocket close after ping failure")
	}
	if got := len(conn.writeMessageTypes); got != 1 {
		t.Fatalf("writeMessage calls = %d, want 1", got)
	}
	if conn.writeMessageTypes[0] != websocket.PingMessage {
		t.Fatalf("first control message = %d, want %d", conn.writeMessageTypes[0], websocket.PingMessage)
	}
}

func TestConfigureWebsocketLivenessPingWritesSetWriteTimeoutBeforeWrite(t *testing.T) {
	conn := newFakeWebsocketConnection()
	writeEvents := make(chan string, 2)
	conn.writeMessageFn = func(messageType int, _ []byte) error {
		if messageType != websocket.PingMessage {
			t.Fatalf("write message type = %d, want %d", messageType, websocket.PingMessage)
		}
		writeEvents <- "write"
		return nil
	}

	conn.setWriteDeadlineFn = func(deadline time.Time) error {
		writeEvents <- "deadline"
		if deadline != time.Unix(1_750_000_000, 0).Add(5*time.Second) {
			t.Fatalf("write deadline = %v, want %v", deadline, time.Unix(1_750_000_000, 0).Add(5*time.Second))
		}
		return nil
	}

	ticker := &fakeWebsocketTicker{ch: make(chan time.Time, 1)}
	controller := socket{serverCfg: configuration.Server{
		ReadTimeout:      2 * time.Second,
		WriteTimeout:     5 * time.Second,
		HeartbeatTimeout: 1 * time.Second,
	}}

	stop, err := controller.configureWebsocketLiveness(
		conn,
		func() time.Time { return time.Unix(1_750_000_000, 0) },
		func(_ time.Duration) websocketTicker { return ticker },
		nil,
	)
	if err != nil {
		t.Fatalf("configureWebsocketLiveness returned error: %v", err)
	}
	defer stop()

	ticker.ch <- time.Unix(1_750_000_001, 0)

	first := readWriteEvent(writeEvents, 100*time.Millisecond, t)
	second := readWriteEvent(writeEvents, 100*time.Millisecond, t)
	if second != "write" {
		t.Fatalf("expected second event to be write, got %q", second)
	}
	if first != "deadline" {
		t.Fatalf("expected first write operation to be deadline, got %q", first)
	}
}

func TestWebsocketWriterWriteSetsWriteDeadline(t *testing.T) {
	writer := websocketWriter{
		writeTimeout: 3 * time.Second,
		now:          func() time.Time { return time.Unix(1_750_000_000, 0) },
		writeMessage: func(mt int, payload []byte) error {
			if mt != websocket.BinaryMessage {
				t.Fatalf("write message type = %d, want %d", mt, websocket.BinaryMessage)
			}
			if string(payload) != "hello" {
				t.Fatalf("payload = %q, want %q", string(payload), "hello")
			}
			return nil
		},
		setWriteDeadline: func(deadline time.Time) error {
			if deadline != time.Unix(1_750_000_000, 0).Add(3*time.Second) {
				t.Fatalf("write deadline = %v, want %v", deadline, time.Unix(1_750_000_000, 0).Add(3*time.Second))
			}
			return nil
		},
	}

	if n, err := writer.Write([]byte("hello")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	} else if n != 5 {
		t.Fatalf("Write returned n=%d, want 5", n)
	}
}

func TestWebsocketWriterDoesNotCarrySharedLock(t *testing.T) {
	writerType := reflect.TypeOf(websocketWriter{})
	for i := 0; i < writerType.NumField(); i++ {
		if writerType.Field(i).Name == "writeMu" {
			t.Fatalf("websocketWriter has unexpected shared-lock field %q", writerType.Field(i).Name)
		}
	}
}

func TestWriteServerNonceUsesSharedSenderLockAndSetsWriteDeadline(t *testing.T) {
	writeMu := &sync.Mutex{}
	events := make(chan string, 3)
	writeDone := make(chan error, 1)

	conn := newFakeWebsocketConnection()
	conn.setWriteDeadlineFn = func(deadline time.Time) error {
		events <- "deadline"
		return nil
	}
	conn.writeMessageFn = func(messageType int, payload []byte) error {
		if messageType != websocket.BinaryMessage {
			t.Fatalf("write message type = %d, want %d", messageType, websocket.BinaryMessage)
		}
		if len(payload) != 12 {
			t.Fatalf("write payload len = %d, want 12", len(payload))
		}
		events <- "write"
		return nil
	}

	writer := websocketWriter{
		writeTimeout: 3 * time.Second,
		now:          func() time.Time { return time.Unix(1_750_000_000, 0) },
		writeMessage: func(mt int, payload []byte) error {
			return conn.WriteMessage(mt, payload)
		},
		setWriteDeadline: func(deadline time.Time) error {
			return conn.SetWriteDeadline(deadline)
		},
	}

	controller := socket{}

	writeMu.Lock()
	go func() {
		writeDone <- controller.writeServerNonce(
			writeMu,
			writer,
			make([]byte, 12),
		)
	}()

	select {
	case <-writeDone:
		t.Fatal("nonce write completed while sender lock was held; expected lock to be respected")
	case <-time.After(30 * time.Millisecond):
	}

	writeMu.Unlock()

	first := readWriteEvent(events, 100*time.Millisecond, t)
	second := readWriteEvent(events, 100*time.Millisecond, t)
	if first != "deadline" {
		t.Fatalf("first write operation = %q, want deadline", first)
	}
	if second != "write" {
		t.Fatalf("second write operation = %q, want write", second)
	}

	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("writeServerNonce returned error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("nonce write did not complete after sender lock released")
	}
}

func TestConfigureWebsocketLivenessStopUnblocksHeartbeatWrite(t *testing.T) {
	conn := newFakeWebsocketConnection()
	ticker := &fakeWebsocketTicker{ch: make(chan time.Time, 2)}
	writeCh := make(chan struct{}, 1)
	conn.writeMessageFn = func(_ int, _ []byte) error {
		writeCh <- struct{}{}
		<-conn.closeCh
		return nil
	}

	controller := socket{serverCfg: configuration.Server{
		ReadTimeout:      2 * time.Second,
		WriteTimeout:     5 * time.Second,
		HeartbeatTimeout: 1 * time.Second,
	}}
	stop, err := controller.configureWebsocketLiveness(
		conn,
		func() time.Time { return time.Unix(1_750_000_000, 0) },
		func(_ time.Duration) websocketTicker { return ticker },
		nil,
	)
	if err != nil {
		t.Fatalf("configureWebsocketLiveness returned error: %v", err)
	}
	ticker.ch <- time.Unix(1_750_000_000, 0)
	waitForWriteEvent(writeCh, 100*time.Millisecond, t)

	stopDone := make(chan struct{})
	go func() {
		stop()
		close(stopDone)
	}()
	select {
	case <-stopDone:
	case <-time.After(150 * time.Millisecond):
		t.Fatal("stop did not return while heartbeat write was blocked")
	}

	ticker.ch <- time.Unix(1_750_000_001, 0)
	if got := len(writeCh); got != 0 {
		t.Fatalf("unexpected buffered heartbeat write(s) remained after stop: %d", got)
	}
	select {
	case <-writeCh:
		t.Fatal("received ping write after stop")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestConfigureWebsocketLivenessPingSharesWriteLock(t *testing.T) {
	conn := newFakeWebsocketConnection()
	writeMu := &sync.Mutex{}
	ticker := &fakeWebsocketTicker{ch: make(chan time.Time, 1)}
	writeEvent := make(chan struct{}, 1)
	conn.writeMessageFn = func(messageType int, _ []byte) error {
		if messageType != websocket.PingMessage {
			t.Fatalf("write message type = %d, want %d", messageType, websocket.PingMessage)
		}
		writeEvent <- struct{}{}
		return nil
	}

	controller := socket{serverCfg: configuration.Server{
		ReadTimeout:      2 * time.Second,
		WriteTimeout:     5 * time.Second,
		HeartbeatTimeout: 1 * time.Second,
	}}
	stop, err := controller.configureWebsocketLiveness(
		conn,
		func() time.Time { return time.Unix(1_750_000_000, 0) },
		func(_ time.Duration) websocketTicker { return ticker },
		writeMu,
	)
	if err != nil {
		t.Fatalf("configureWebsocketLiveness returned error: %v", err)
	}
	defer stop()

	writeMu.Lock()
	ticker.ch <- time.Unix(1_750_000_000, 0)
	select {
	case <-writeEvent:
		t.Fatal("ping write happened while shared write lock was held")
	case <-time.After(40 * time.Millisecond):
	}
	writeMu.Unlock()

	waitForWriteEvent(writeEvent, 100*time.Millisecond, t)
}
