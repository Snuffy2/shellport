// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package command

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
	"github.com/Snuffy2/shellport/application/rw"
)

func testDummyFetchGen(data []byte) rw.FetchReaderFetcher {
	current := 0

	return func() ([]byte, error) {
		if current >= len(data) {
			return nil, io.EOF
		}

		oldCurrent := current
		current++

		return data[oldCurrent:current], nil
	}
}

type dummyWriter struct {
	written []byte
}

func (d *dummyWriter) Write(b []byte) (int, error) {
	d.written = append(d.written, b...)

	return len(b), nil
}

func TestHandlerHandleEcho(t *testing.T) {
	w := dummyWriter{
		written: make([]byte, 0, 64),
	}
	s := []byte{
		byte(HeaderControl | 13),
		HeaderControlEcho,
		'H', 'E', 'L', 'L', 'O', ' ', 'W', 'O', 'R', 'L', 'D', '1',
		byte(HeaderControl | 13),
		HeaderControlEcho,
		'H', 'E', 'L', 'L', 'O', ' ', 'W', 'O', 'R', 'L', 'D', '2',
		byte(HeaderControl | HeaderMaxData),
		HeaderControlEcho,
		'1', '1', '1', '1', '1', '1', '1', '1', '1', '1',
		'1', '1', '1', '1', '1', '1', '1', '1', '1', '1',
		'1', '1', '1', '1', '1', '1', '1', '1', '1', '1',
		'1', '1', '1', '1', '1', '1', '1', '1', '1', '1',
		'1', '1', '1', '1', '1', '1', '1', '1', '1', '1',
		'1', '1', '1', '1', '1', '1', '1', '1', '1', '1',
		'2', '2',
		byte(HeaderControl | 13),
		HeaderControlEcho,
		'H', 'E', 'L', 'L', 'O', ' ', 'W', 'O', 'R', 'L', 'D', '3',
	}
	lock := sync.Mutex{}
	bufferPool := NewBufferPool(4096)
	handler := newHandler(
		Configuration{},
		nil,
		rw.NewFetchReader(testDummyFetchGen(s)),
		&w,
		&lock,
		0,
		0,
		log.NewDitch(),
		NewHooks(configuration.HookSettings{}),
		&bufferPool,
	)

	hErr := handler.Handle()

	if hErr != nil && hErr != io.EOF {
		t.Error("Failed to write due to error:", hErr)

		return
	}

	if !bytes.Equal(w.written, s) {
		t.Errorf("Expecting the data to be %d, got %d instead", s, w.written)

		return
	}
}

func TestHandlerRejectsUnknownControlSignal(t *testing.T) {
	w := dummyWriter{
		written: make([]byte, 0, 64),
	}
	s := []byte{
		byte(HeaderControl | 1),
		0xff,
	}
	lock := sync.Mutex{}
	bufferPool := NewBufferPool(4096)
	handler := newHandler(
		Configuration{},
		nil,
		rw.NewFetchReader(testDummyFetchGen(s)),
		&w,
		&lock,
		0,
		0,
		log.NewDitch(),
		NewHooks(configuration.HookSettings{}),
		&bufferPool,
	)

	hErr := handler.Handle()

	if !errors.Is(hErr, ErrHandlerUnknownControlSignal) {
		t.Fatalf("expected unknown control signal error, got %v", hErr)
	}
}
