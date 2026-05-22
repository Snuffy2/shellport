// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"path/filepath"
	"testing"

	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
)

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

func TestConfigLoadersCreateConfiguredMissingFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "custom-shellport.conf.json")
	t.Setenv("SHELLPORT_CONFIG", configPath)

	_, cfg, err := configuration.Redundant(configLoaders()...)(log.NewDitch())
	if err != nil {
		t.Fatalf("config loader returned error: %v", err)
	}
	if cfg.SourceFile != configPath {
		t.Fatalf("SourceFile = %q, want %q", cfg.SourceFile, configPath)
	}
	if len(cfg.Servers) == 0 {
		t.Fatal("expected at least one default server in generated config")
	}
	if cfg.Servers[0].ListenInterface != "0.0.0.0" {
		t.Fatalf("ListenInterface = %q, want 0.0.0.0", cfg.Servers[0].ListenInterface)
	}
}
