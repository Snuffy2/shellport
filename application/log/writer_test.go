// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package log

import (
	"bytes"
	"regexp"
	"testing"
	"time"
)

func TestWriterUsesLocalTimestampBeforeLogLevel(t *testing.T) {
	originalLocal := time.Local
	time.Local = time.FixedZone("LOCAL", -4*60*60)
	t.Cleanup(func() {
		time.Local = originalLocal
	})

	var output bytes.Buffer
	logger := NewWriter("ShellPort", &output)

	logger.Info("listening on %s", "127.0.0.1")

	matches, err := regexp.MatchString(
		`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} \[INF\] listening on 127\.0\.0\.1\r\n$`,
		output.String(),
	)
	if err != nil {
		t.Fatalf("compile log format regexp: %v", err)
	}
	if !matches {
		t.Fatalf("unexpected log line format: %q", output.String())
	}
}

func TestWriterOmitsRootContextFromChildLogs(t *testing.T) {
	var output bytes.Buffer
	logger := NewWriter("ShellPort", &output).Context("Server")

	logger.Info("Serving")

	if regexp.MustCompile(`ShellPort\s*>`).MatchString(output.String()) {
		t.Fatalf("expected log output to omit root context: %q", output.String())
	}
	if !regexp.MustCompile(`\[INF\] Server: Serving\r\n$`).MatchString(output.String()) {
		t.Fatalf("unexpected child context log output: %q", output.String())
	}
}
