// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package main

import "testing"

func TestShouldPrintVersionAcceptsVersionFlags(t *testing.T) {
	for _, args := range [][]string{
		{"shellport", "-V"},
		{"shellport", "--version"},
	} {
		if !shouldPrintVersion(args) {
			t.Fatalf("shouldPrintVersion(%v) = false, want true", args)
		}
	}
}

func TestShouldPrintVersionRejectsOtherArgs(t *testing.T) {
	for _, args := range [][]string{
		{"shellport"},
		{"shellport", "--help"},
		{"shellport", "--version", "--help"},
	} {
		if shouldPrintVersion(args) {
			t.Fatalf("shouldPrintVersion(%v) = true, want false", args)
		}
	}
}

func TestDebugLoggingEnabledFromEnvironmentValue(t *testing.T) {
	t.Setenv("SHELLPORT_DEBUG", "1")

	if !debugLoggingEnabled() {
		t.Fatal("expected SHELLPORT_DEBUG to enable debug logging")
	}
}

func TestDebugLoggingDisabledWhenEnvironmentEmpty(t *testing.T) {
	t.Setenv("SHELLPORT_DEBUG", "")

	if debugLoggingEnabled() {
		t.Fatal("expected empty SHELLPORT_DEBUG to disable debug logging")
	}
}
