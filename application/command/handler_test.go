// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package command

import (
	"bytes"
	"io"
	"sync"
	"testing"

	"github.com/Snuffy2/shellport/application/log"
	"github.com/Snuffy2/shellport/application/rw"
)

func TestHandlerHandleCloseInactiveStreamIsIgnored(t *testing.T) {
	lock := sync.Mutex{}
	bufferPool := NewBufferPool(4096)
	output := bytes.NewBuffer(make([]byte, 0, 1))
	h := newHandler(
		Configuration{},
		&Commands{},
		rw.NewFetchReader(func() ([]byte, error) {
			return nil, io.EOF
		}),
		output,
		&lock,
		0,
		0,
		log.NewDitch(),
		Hooks{},
		&bufferPool,
	)
	hd := HeaderClose
	hd.Set(1)

	if err := h.handleClose(hd, 1, log.NewDitch()); err != nil {
		t.Fatalf("inactive close returned error: %v", err)
	}

	expected := []byte{byte(HeaderCompleted | 1)}
	if !bytes.Equal(output.Bytes(), expected) {
		t.Fatalf("expected completed frame %v, got %v", expected, output.Bytes())
	}
}
