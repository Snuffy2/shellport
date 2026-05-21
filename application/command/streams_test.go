// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package command

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
	"github.com/Snuffy2/shellport/application/rw"
)

func TestStreamInitialHeader(t *testing.T) {
	hd := streamInitialHeader{}

	hd.set(15, 128, true)

	if hd.command() != 15 {
		t.Errorf("Expecting command to be %d, got %d instead",
			15, hd.command())

		return
	}

	if hd.data() != 128 {
		t.Errorf("Expecting data to be %d, got %d instead", 128, hd.data())

		return
	}

	if hd.success() != true {
		t.Errorf("Expecting success to be %v, got %v instead",
			true, hd.success())

		return
	}

	hd.set(0, 2047, false)

	if hd.command() != 0 {
		t.Errorf("Expecting command to be %d, got %d instead",
			0, hd.command())

		return
	}

	if hd.data() != 2047 {
		t.Errorf("Expecting data to be %d, got %d instead", 2047, hd.data())

		return
	}

	if hd.success() != false {
		t.Errorf("Expecting success to be %v, got %v instead",
			false, hd.success())

		return
	}
}

func TestStreamHeader(t *testing.T) {
	s := StreamHeader{}

	s.Set(StreamHeaderMaxMarker, StreamHeaderMaxLength)

	if s.Marker() != StreamHeaderMaxMarker {
		t.Errorf("Expecting the marker to be %d, got %d instead",
			StreamHeaderMaxMarker, s.Marker())

		return
	}

	if s.Length() != StreamHeaderMaxLength {
		t.Errorf("Expecting the length to be %d, got %d instead",
			StreamHeaderMaxLength, s.Length())

		return
	}

	if s[0] != s[1] || s[0] != 0xff {
		t.Errorf("Expecting the header to be 255, 255, got %d, %d instead",
			s[0], s[1])

		return
	}
}

func TestStreamResponderSendSegmentsLargePayload(t *testing.T) {
	output := bytes.NewBuffer(nil)
	lock := sync.Mutex{}
	sender := streamHandlerSender{
		handlerSender: &handlerSender{
			writer:   output,
			lock:     &lock,
			needWait: false,
			sign:     sync.NewCond(&lock),
		},
	}
	header := HeaderStream
	header.Set(7)
	responder := newStreamResponder(sender, header)
	payload := bytes.Repeat([]byte("a"), 9000)

	if err := responder.Send(2, payload, make([]byte, 128)); err != nil {
		t.Fatalf("expected segmented send to succeed, got %v", err)
	}

	var decoded []byte
	for remaining := output.Bytes(); len(remaining) > 0; {
		if remaining[0] != byte(header) {
			t.Fatalf("expected stream header %d, got %d", header, remaining[0])
		}

		streamHeader := StreamHeader{remaining[1], remaining[2]}
		if streamHeader.Marker() != 2 {
			t.Fatalf("expected marker 2, got %d", streamHeader.Marker())
		}

		frameLen := int(streamHeader.Length())
		if frameLen > 125 {
			t.Fatalf("expected frame payload length <= 125, got %d", frameLen)
		}

		decoded = append(decoded, remaining[3:3+frameLen]...)
		remaining = remaining[3+frameLen:]
	}

	if !bytes.Equal(decoded, payload) {
		t.Fatalf("expected decoded payload length %d, got %d", len(payload), len(decoded))
	}
}

// testStreamMachine is a configurable FSM implementation used to observe stream
// close and release lifecycle calls.
type testStreamMachine struct {
	state        FSMState
	bootupErr    FSMError
	closeErr     error
	releaseErr   error
	closeCalls   int
	releaseCalls int
}

// Bootup returns the configured boot result for a test stream FSM.
func (m *testStreamMachine) Bootup(
	_ *rw.LimitedReader,
	_ []byte,
) (FSMState, FSMError) {
	if !m.bootupErr.Succeed() {
		return nil, m.bootupErr
	}

	return m.state, NoFSMError()
}

// Close records a close call and returns the configured close error.
func (m *testStreamMachine) Close() error {
	m.closeCalls++

	return m.closeErr
}

// Release records a release call and returns the configured release error.
func (m *testStreamMachine) Release() error {
	m.releaseCalls++

	return m.releaseErr
}

// newTestStreamFetcher builds a fetch reader that returns chunks in order and
// fails if the test reads past the provided input.
func newTestStreamFetcher(chunks ...[]byte) *rw.FetchReader {
	current := 0

	reader := rw.NewFetchReader(func() ([]byte, error) {
		if current >= len(chunks) {
			return nil, errors.New("unexpected fetch")
		}

		chunk := chunks[current]
		current++

		return chunk, nil
	})

	return &reader
}

// newBootedTestStream returns a stream with the supplied machine booted and
// installed as its active FSM.
func newBootedTestStream(
	t *testing.T,
	machine *testStreamMachine,
) stream {
	t.Helper()

	st := newStream()
	st.f = newFSM(machine)

	bootReader := newTestStreamFetcher()
	bootLimitedReader := rw.NewLimitedReader(bootReader, 0)
	bootErr := st.f.bootup(&bootLimitedReader, nil)

	if !bootErr.Succeed() {
		t.Fatalf("expected bootup to succeed, got %v", bootErr)
	}

	return st
}

// TestStreamCloseIsIdempotent verifies that duplicate close frames do not
// re-close an already closing stream or emit duplicate completion frames.
func TestStreamCloseIsIdempotent(t *testing.T) {
	const streamID = 7

	machine := &testStreamMachine{
		state: func(_ *FSM, _ *rw.LimitedReader, _ StreamHeader, _ []byte) error {
			return nil
		},
		bootupErr: NoFSMError(),
	}
	output := bytes.NewBuffer(make([]byte, 0, 2))
	bufferPool := NewBufferPool(4096)
	handler := newHandler(
		Configuration{},
		&Commands{},
		*newTestStreamFetcher(),
		output,
		&sync.Mutex{},
		0,
		0,
		log.NewDitch(),
		NewHooks(configuration.HookSettings{}),
		&bufferPool,
	)

	handler.streams[streamID] = newBootedTestStream(t, machine)

	header := HeaderClose
	header.Set(streamID)

	if closeErr := handler.handleClose(header, streamID, log.NewDitch()); closeErr != nil {
		t.Fatalf("expected first close to succeed, got %v", closeErr)
	}

	closeErr := handler.handleClose(header, streamID, log.NewDitch())
	if closeErr != nil {
		t.Fatalf("expected duplicate close to be ignored, got %v", closeErr)
	}

	expected := []byte{
		byte(HeaderCompleted | streamID),
		byte(HeaderCompleted | streamID),
	}
	if !bytes.Equal(output.Bytes(), expected) {
		t.Fatalf("expected duplicate close completion frames %v, got %v", expected, output.Bytes())
	}

	if machine.closeCalls != 1 {
		t.Fatalf("expected Close to be called once, got %d", machine.closeCalls)
	}

	if releaseErr := handler.streams[streamID].release(); releaseErr != nil {
		t.Fatalf("expected release to succeed, got %v", releaseErr)
	}

	if closeErr = handler.handleClose(header, streamID, log.NewDitch()); closeErr != nil {
		t.Fatalf("expected released stream close to be ignored, got %v", closeErr)
	}

	expected = append(expected, byte(HeaderCompleted|streamID))
	if !bytes.Equal(output.Bytes(), expected) {
		t.Fatalf("expected completion frames %v, got %v", expected, output.Bytes())
	}

	if machine.releaseCalls != 1 {
		t.Fatalf(
			"expected Release to be called once, got %d",
			machine.releaseCalls,
		)
	}
}

// TestStreamErrorCompletesStream verifies that shutdown still closes and
// releases streams whose state handler returned an error.
func TestStreamErrorCompletesStream(t *testing.T) {
	const streamID = 9

	streamErr := errors.New("stream tick failed")
	machine := &testStreamMachine{
		state: func(_ *FSM, _ *rw.LimitedReader, _ StreamHeader, _ []byte) error {
			return streamErr
		},
		bootupErr: NoFSMError(),
	}
	output := bytes.NewBuffer(make([]byte, 0, 1))
	bufferPool := NewBufferPool(4096)
	handler := newHandler(
		Configuration{},
		&Commands{},
		*newTestStreamFetcher(),
		output,
		&sync.Mutex{},
		0,
		0,
		log.NewDitch(),
		NewHooks(configuration.HookSettings{}),
		&bufferPool,
	)

	handler.streams[streamID] = newBootedTestStream(t, machine)

	streamHeader := StreamHeader{}
	streamHeader.Set(0, 0)

	handler.receiver = *newTestStreamFetcher(streamHeader[:])
	header := HeaderStream
	header.Set(streamID)
	tickErr := handler.handleStream(header, streamID, log.NewDitch())

	if !errors.Is(tickErr, streamErr) {
		t.Fatalf("expected tick to return %v, got %v", streamErr, tickErr)
	}

	if !handler.streams[streamID].running() {
		t.Fatal("expected stream to remain active until close/release after tick error")
	}

	handler.streams.shutdown()

	if handler.streams[streamID].running() {
		t.Fatal("expected stream to be inactive after shutdown")
	}

	futureTickErr := handler.streams[streamID].tick(
		header,
		newTestStreamFetcher(streamHeader[:]),
		make([]byte, 1),
	)
	if !errors.Is(futureTickErr, ErrStreamsStreamOperateInactiveStream) {
		t.Fatalf(
			"expected future write after shutdown to return %v, got %v",
			ErrStreamsStreamOperateInactiveStream,
			futureTickErr,
		)
	}

	if machine.closeCalls != 1 {
		t.Fatalf("expected Close to be called once, got %d", machine.closeCalls)
	}

	if machine.releaseCalls != 1 {
		t.Fatalf("expected Release to be called once, got %d", machine.releaseCalls)
	}
}
