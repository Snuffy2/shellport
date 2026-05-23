// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package configuration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Snuffy2/shellport/application/log"
)

func TestLoadFileRejectsPresetSecretKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`HostName: localhost
PresetSecretKey: not-allowed
Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
`)
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
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`HostName: localhost
SHELLPORT_PRESET_SECRET_KEY: not-allowed
Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
`)
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

func TestLoadFileAcceptsYAMLConfigFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`# ShellPort accepts comments in YAML config files.
HostName: localhost
Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	if cfg.HostName != "localhost" {
		t.Fatalf("HostName = %q, want localhost", cfg.HostName)
	}
	if cfg.Servers[0].ListenPort != 8182 {
		t.Fatalf("ListenPort = %d, want 8182", cfg.Servers[0].ListenPort)
	}
}

func TestExampleConfigFileIsLoadable(t *testing.T) {
	configPath := filepath.Join("..", "..", "shellport.conf.example.yml")

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("server count = %d, want 1", len(cfg.Servers))
	}
	if len(cfg.Presets) <= 0 {
		t.Fatal("preset count = 0, want example presets")
	}
}

func TestDevConfigTemplateIsLoadable(t *testing.T) {
	configPath := filepath.Join("..", "..", "scripts", "shellport.dev.conf.yml")

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("server count = %d, want 1", len(cfg.Servers))
	}
	if cfg.Servers[0].ListenInterface != "127.0.0.1" {
		t.Fatalf(
			"ListenInterface = %q, want 127.0.0.1",
			cfg.Servers[0].ListenInterface,
		)
	}
}

func TestLoadFileReadsServerTitle(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`HostName: localhost
Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
    ServerTitle: Homelab Shells
    ServerMessage: Pick a host
`)
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

func TestLoadFilePreservesUnquotedYAMLStringScalars(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`HostName: 010
UserPassword: 0123
AdminPassword: true
Socks5User: 0007
Socks5Password: false
Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
    ServerTitle: 2026
Presets:
  - ID: 0001
    Title: 0010
    Type: SSH
    Host: atlantis.home
    TabColor: 0099
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	if cfg.HostName != "010" {
		t.Fatalf("HostName = %q, want 010", cfg.HostName)
	}
	if cfg.UserPassword != "0123" {
		t.Fatalf("UserPassword = %q, want 0123", cfg.UserPassword)
	}
	if cfg.AdminPassword != "true" {
		t.Fatalf("AdminPassword = %q, want true", cfg.AdminPassword)
	}
	if cfg.Socks5User != "0007" {
		t.Fatalf("Socks5User = %q, want 0007", cfg.Socks5User)
	}
	if cfg.Socks5Password != "false" {
		t.Fatalf("Socks5Password = %q, want false", cfg.Socks5Password)
	}
	if cfg.Servers[0].ServerTitle != "2026" {
		t.Fatalf("ServerTitle = %q, want 2026", cfg.Servers[0].ServerTitle)
	}
	if cfg.Presets[0].ID != "0001" {
		t.Fatalf("ID = %q, want 0001", cfg.Presets[0].ID)
	}
	if cfg.Presets[0].Title != "0010" {
		t.Fatalf("Title = %q, want 0010", cfg.Presets[0].Title)
	}
	if cfg.Presets[0].TabColor != "0099" {
		t.Fatalf("TabColor = %q, want 0099", cfg.Presets[0].TabColor)
	}
}

func TestLoadFileAcceptsUnquotedYAMLMetaScalarsAsStrings(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
Presets:
  - Title: Atlantis
    Type: ET
    Host: atlantis.home
    Meta:
      User: pi
      ET Server Port: 2022
      Password: 0123
      Enabled: true
      Empty: null
      Tilde: ~
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	if cfg.Presets[0].Meta["ET Server Port"] != "2022" {
		t.Fatalf("ET Server Port = %q, want 2022", cfg.Presets[0].Meta["ET Server Port"])
	}
	if cfg.Presets[0].Meta["Password"] != "0123" {
		t.Fatalf("Password = %q, want 0123", cfg.Presets[0].Meta["Password"])
	}
	if cfg.Presets[0].Meta["Enabled"] != "true" {
		t.Fatalf("Enabled = %q, want true", cfg.Presets[0].Meta["Enabled"])
	}
	if cfg.Presets[0].Meta["Empty"] != "" {
		t.Fatalf("Empty = %q, want empty", cfg.Presets[0].Meta["Empty"])
	}
	if cfg.Presets[0].Meta["Tilde"] != "" {
		t.Fatalf("Tilde = %q, want empty", cfg.Presets[0].Meta["Tilde"])
	}
}

func TestLoadFileSkipsYAMLMetaMergeKeys(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
MetaDefaults: &metaDefaults
  User: 010
FallbackMetaDefaults: &fallbackMetaDefaults
  User: 020
Presets:
  - Title: Atlantis
    Type: SSH
    Host: atlantis.home
    Meta:
      <<: [*metaDefaults, *fallbackMetaDefaults]
      Password: secret
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	if cfg.Presets[0].Meta["User"] != "010" {
		t.Fatalf("User = %q, want 010", cfg.Presets[0].Meta["User"])
	}
	if _, ok := cfg.Presets[0].Meta["<<"]; ok {
		t.Fatal("merge key was preserved as preset metadata")
	}
}

func TestDefaultFileSearchListUsesConfigDirectoryOnly(t *testing.T) {
	searchList := defaultFileSearchList()

	expected := []string{filepath.Join("/", "config", "shellport.conf.yml")}
	if len(searchList) != len(expected) {
		t.Fatalf("defaultFileSearchList() length = %d, want %d: %v", len(searchList), len(expected), searchList)
	}
	for i := range expected {
		if searchList[i] != expected[i] {
			t.Fatalf("defaultFileSearchList()[%d] = %q, want %q", i, searchList[i], expected[i])
		}
	}
}

func TestCreateDefaultConfigFileWritesLoadableConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config", "shellport.conf.yml")

	if err := createDefaultConfigFile(configPath); err != nil {
		t.Fatalf("createDefaultConfigFile returned error: %v", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("os.Stat returned error: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("created config mode = %v, want 0600", info.Mode().Perm())
	}

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	if cfg.SourceFile != configPath {
		t.Fatalf("SourceFile = %q, want %q", cfg.SourceFile, configPath)
	}
	if cfg.UserPassword != "" {
		t.Fatalf("UserPassword = %q, want empty", cfg.UserPassword)
	}
	if cfg.AdminPassword != "" {
		t.Fatalf("AdminPassword = %q, want empty", cfg.AdminPassword)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("server count = %d, want 1", len(cfg.Servers))
	}
	if cfg.Servers[0].ListenInterface != "0.0.0.0" {
		t.Fatalf("ListenInterface = %q, want 0.0.0.0", cfg.Servers[0].ListenInterface)
	}
	if cfg.Servers[0].ListenPort != 8182 {
		t.Fatalf("ListenPort = %d, want 8182", cfg.Servers[0].ListenPort)
	}
	if len(cfg.Presets) != 0 {
		t.Fatalf("preset count = %d, want 0", len(cfg.Presets))
	}
}

func TestCreateDefaultConfigFileDoesNotOverwriteExistingConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	original := []byte("Servers:\n  - ListenInterface: 127.0.0.1\n    ListenPort: 8182\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := createDefaultConfigFile(configPath); err == nil {
		t.Fatal("createDefaultConfigFile returned nil error, want existing file error")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if string(content) != string(original) {
		t.Fatalf("config content = %q, want %q", content, original)
	}
}

func TestAutoCreateDefaultFileLoadsExistingConfigAfterCreateRace(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	original := []byte("Servers:\n  - ListenInterface: 127.0.0.1\n    ListenPort: 8182\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := AutoCreateDefaultFile(configPath)(log.NewDitch())
	if err != nil {
		t.Fatalf("AutoCreateDefaultFile returned error: %v", err)
	}
	if cfg.SourceFile != configPath {
		t.Fatalf("SourceFile = %q, want %q", cfg.SourceFile, configPath)
	}
	if cfg.Servers[0].ListenInterface != "127.0.0.1" {
		t.Fatalf("ListenInterface = %q, want 127.0.0.1", cfg.Servers[0].ListenInterface)
	}
}
