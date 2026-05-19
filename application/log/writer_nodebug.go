// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package log

import (
	"fmt"
	"io"
)

// NonDebugWriter is a Writer variant that suppresses Debug-level messages.
// Info, Warning, and Error output is forwarded to the underlying io.Writer
// unchanged; Context and TitledContext return new NonDebugWriter instances
// so the suppression is inherited by child loggers.
type NonDebugWriter struct {
	Writer
}

// NewNonDebugWriter creates a NonDebugWriter that writes to w under the given
// initial context label.
func NewNonDebugWriter(context string, w io.Writer) NonDebugWriter {
	return NonDebugWriter{
		Writer: NewWriter(context, w),
	}
}

// NewDebugOrNonDebugWriter returns a Writer when useDebug is true or a
// NonDebugWriter when false, allowing callers to select the log verbosity at
// runtime without branching everywhere.
func NewDebugOrNonDebugWriter(
	useDebug bool, context string, w io.Writer) Logger {
	if useDebug {
		return NewWriter(context, w)
	}
	return NewNonDebugWriter(context, w)
}

// Context returns a child NonDebugWriter with name appended to the context
// path, preserving debug suppression in the child.
func (w NonDebugWriter) Context(name string) Logger {
	return NonDebugWriter{
		Writer: Writer{
			c: appendContext(w.c, name),
			w: w.w,
		},
	}
}

// TitledContext returns a child NonDebugWriter with a formatted name appended
// to the context path, preserving debug suppression in the child.
func (w NonDebugWriter) TitledContext(
	name string,
	params ...any,
) Logger {
	return NonDebugWriter{
		Writer: Writer{
			c: appendContext(w.c, fmt.Sprintf(name, params...)),
			w: w.w,
		},
	}
}

// Debug is a no-op in NonDebugWriter; debug messages are silently discarded.
func (w NonDebugWriter) Debug(msg string, params ...any) {}
