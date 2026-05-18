// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package log

import (
	"fmt"
	"io"
	"time"
)

// Writer is a Logger implementation that formats each message with a prefix
// containing the sortable timestamp, log level, and the hierarchical context
// path, and writes it to the underlying io.Writer. All four severity levels are
// active, including Debug.
type Writer struct {
	// c is the accumulated context path, e.g. "Root > Server > Request".
	c string
	// w is the output destination.
	w io.Writer
}

// NewWriter creates a Writer that writes to w with the given initial context
// label.
func NewWriter(context string, w io.Writer) Writer {
	return Writer{
		c: context,
		w: w,
	}
}

// Context returns a child Writer with name appended to the context path.
func (w Writer) Context(name string) Logger {
	return NewWriter(w.c+" > "+name, w.w)
}

// TitledContext returns a child Writer with a formatted name appended to the
// context path.
func (w Writer) TitledContext(name string, params ...any) Logger {
	return NewWriter(w.c+" > "+fmt.Sprintf(name, params...), w.w)
}

// Write satisfies io.Writer by logging b at the "DEF" severity level.
func (w Writer) Write(b []byte) (int, error) {
	_, wErr := w.write("DEF", string(b))

	if wErr != nil {
		return 0, wErr
	}

	return len(b), nil
}

// write formats and emits a single log line with the current sortable
// timestamp, the given prefix tag (e.g. "INF", "DBG"), the context path, and
// the message. It returns the number of bytes written and any write error.
func (w Writer) write(
	prefix string, msg string, params ...any) (int, error) {
	return fmt.Fprintf(w.w, time.Now().Format("2006-01-02 15:04:05")+
		" ["+prefix+"] "+w.c+": "+msg+"\r\n", params...)
}

// Info write an info message
func (w Writer) Info(msg string, params ...any) {
	w.write("INF", msg, params...)
}

// Debug write an debug message
func (w Writer) Debug(msg string, params ...any) {
	w.write("DBG", msg, params...)
}

// Warning write an warning message
func (w Writer) Warning(msg string, params ...any) {
	w.write("WRN", msg, params...)
}

// Error write an error message
func (w Writer) Error(msg string, params ...any) {
	w.write("ERR", msg, params...)
}
