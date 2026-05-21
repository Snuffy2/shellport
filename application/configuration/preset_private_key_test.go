// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package configuration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigratePresetPrivateKeysToFilesReplacesExistingFileReferenceWithPastedKey(
	t *testing.T,
) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "shellport.conf.json")
	oldKeyPath := filepath.Join(configDir, "old.key")
	if err := os.WriteFile(oldKeyPath, []byte("OLD PRIVATE KEY DATA"), 0o600); err != nil {
		t.Fatalf("os.WriteFile old key returned error: %v", err)
	}
	writePresetConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home:22",
			"Meta": map[string]any{
				"User":           "pi",
				"Authentication": "Private Key",
				"Private Key":    "file://" + oldKeyPath,
			},
		},
	})

	presets, changed, err := MigratePresetPrivateKeysToFiles(configPath, []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"User":           "pi",
				"Authentication": "Private Key",
				"Private Key":    "NEW PRIVATE KEY DATA",
			},
		},
	})
	if err != nil {
		t.Fatalf("MigratePresetPrivateKeysToFiles returned error: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}

	resolvedConfigDir, err := filepath.EvalSymlinks(configDir)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks returned error: %v", err)
	}
	newKeyPath := filepath.Join(resolvedConfigDir, "private_keys", "atlantis.key")
	if presets[0].Meta["Private Key"] != "file://"+newKeyPath {
		t.Fatalf("preset private key = %q, want new file ref", presets[0].Meta["Private Key"])
	}
	data, err := os.ReadFile(newKeyPath)
	if err != nil {
		t.Fatalf("os.ReadFile new key returned error: %v", err)
	}
	if string(data) != "NEW PRIVATE KEY DATA" {
		t.Fatalf("new key data = %q, want new private key data", string(data))
	}
	info, err := os.Stat(newKeyPath)
	if err != nil {
		t.Fatalf("os.Stat new key returned error: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("new key mode = %o, want 0600", info.Mode().Perm())
	}

	raw, _, err := readCommonInputFile(configPath)
	if err != nil {
		t.Fatalf("readCommonInputFile returned error: %v", err)
	}
	requireRawPresetCount(t, raw.Presets, 1)
	if raw.Presets[0].Meta["Private Key"] != String("file://"+newKeyPath) {
		t.Fatalf("raw private key = %q, want new file ref", raw.Presets[0].Meta["Private Key"])
	}
}
