// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package application

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
)

func TestWarnIfPresetConfigNotWritableLogsForUnwritableFileBackedConfig(
	t *testing.T,
) {
	configPath := filepath.Join(t.TempDir(), "missing-shellport.conf.json")
	var output bytes.Buffer

	warnIfPresetConfigNotWritable(
		configuration.Common{SourceFile: configPath},
		log.NewWriter("ShellPort", &output),
	)

	logOutput := output.String()
	if !strings.Contains(logOutput, "[WRN]") {
		t.Fatalf("log output = %q, want warning line", logOutput)
	}
	if !strings.Contains(logOutput, "Preset config file is not writable") {
		t.Fatalf("log output = %q, want preset writability warning", logOutput)
	}
	if !strings.Contains(logOutput, configPath) {
		t.Fatalf("log output = %q, want config path %q", logOutput, configPath)
	}
}

func TestWarnIfPresetConfigNotWritableSkipsWritableFileBackedConfig(
	t *testing.T,
) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	var output bytes.Buffer

	warnIfPresetConfigNotWritable(
		configuration.Common{SourceFile: configPath},
		log.NewWriter("ShellPort", &output),
	)

	if output.Len() != 0 {
		t.Fatalf("log output = %q, want no warning", output.String())
	}
}

func TestWarnIfPresetConfigNotWritableSkipsEnvironmentConfig(t *testing.T) {
	var output bytes.Buffer

	warnIfPresetConfigNotWritable(
		configuration.Common{},
		log.NewWriter("ShellPort", &output),
	)

	if output.Len() != 0 {
		t.Fatalf("log output = %q, want no warning", output.String())
	}
}
