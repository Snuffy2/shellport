// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package configuration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFileRejectsPresetSecretKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	content := []byte(`{
  "HostName": "localhost",
  "PresetSecretKey": "not-allowed",
  "Servers": [
    {"ListenInterface": "127.0.0.1", "ListenPort": 8182}
  ]
}`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, _, err := loadFile(configPath)
	if err == nil {
		t.Fatal("loadFile returned nil error, want preset secret key error")
	}
	if !strings.Contains(err.Error(), PresetSecretKeyEnv) {
		t.Fatalf("loadFile error = %q, want %s", err, PresetSecretKeyEnv)
	}
}

func TestLoadFileRejectsPresetSecretKeyEnvName(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	content := []byte(`{
  "HostName": "localhost",
  "SHELLPORT_PRESET_SECRET_KEY": "not-allowed",
  "Servers": [
    {"ListenInterface": "127.0.0.1", "ListenPort": 8182}
  ]
}`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, _, err := loadFile(configPath)
	if err == nil {
		t.Fatal("loadFile returned nil error, want preset secret key error")
	}
	if !strings.Contains(err.Error(), PresetSecretKeyEnv) {
		t.Fatalf("loadFile error = %q, want %s", err, PresetSecretKeyEnv)
	}
}

func TestLoadFileReadsServerTitle(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	content := []byte(`{
  "HostName": "localhost",
  "Servers": [
    {
      "ListenInterface": "127.0.0.1",
      "ListenPort": 8182,
      "ServerTitle": "Homelab Shells",
      "ServerMessage": "Pick a host"
    }
  ]
}`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	if cfg.Servers[0].ServerTitle != "Homelab Shells" {
		t.Fatalf("ServerTitle = %q, want Homelab Shells", cfg.Servers[0].ServerTitle)
	}
	if cfg.Servers[0].ServerMessage != "Pick a host" {
		t.Fatalf("ServerMessage = %q, want Pick a host", cfg.Servers[0].ServerMessage)
	}
}

func TestDefaultFileSearchListPrefersEtcShellportDirectory(t *testing.T) {
	searchList := defaultFileSearchList("/home/shellport", "/opt/shellport/shellport")

	expected := []string{
		filepath.Join("/", "etc", "shellport", "shellport.conf.json"),
		filepath.Join("/home/shellport", ".config", "shellport.conf.json"),
		filepath.Join("/opt/shellport", "shellport.conf.json"),
	}
	if len(searchList) != len(expected) {
		t.Fatalf("defaultFileSearchList() length = %d, want %d: %v", len(searchList), len(expected), searchList)
	}
	for i := range expected {
		if searchList[i] != expected[i] {
			t.Fatalf("defaultFileSearchList()[%d] = %q, want %q", i, searchList[i], expected[i])
		}
	}
}
