// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package application

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Snuffy2/shellport/application/commands"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
)

func TestNormalizeStartupPresetIDsPersistsFileBackedIDs(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	configData := map[string]any{
		"Servers": []map[string]any{
			{"ListenInterface": "127.0.0.1", "ListenPort": 8182},
		},
		"Presets": []map[string]any{
			{"Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
		},
	}
	content, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	normalized, err := normalizeStartupPresets(cfg, commands.New())
	if err != nil {
		t.Fatalf("normalizeStartupPresets returned error: %v", err)
	}
	if normalized.Presets[0].ID == "" {
		t.Fatal("normalized preset ID is empty")
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("second CustomFile returned error: %v", err)
	}
	if reloaded.Presets[0].ID != normalized.Presets[0].ID {
		t.Fatalf("persisted ID = %q, want %q",
			reloaded.Presets[0].ID,
			normalized.Presets[0].ID,
		)
	}
}

func TestNormalizeStartupPresetsPersistsMetaCleanupAndDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	configData := map[string]any{
		"Servers": []map[string]any{
			{"ListenInterface": "127.0.0.1", "ListenPort": 8182},
		},
		"Presets": []map[string]any{
			{
				"ID":    "preset-atlantis",
				"Title": "Atlantis",
				"Type":  "SSH",
				"Host":  "atlantis.home",
				"Meta": map[string]string{
					"User":           "pi",
					"Authentication": "Private Key",
					"Mosh Server":    "mosh-server",
					"ET Server Port": "2022",
					"Future Meta":    "preserve-me",
				},
			},
		},
	}
	content, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	normalized, err := normalizeStartupPresets(cfg, commands.New())
	if err != nil {
		t.Fatalf("normalizeStartupPresets returned error: %v", err)
	}
	meta := normalized.Presets[0].Meta
	if _, ok := meta["Mosh Server"]; ok {
		t.Fatal("normalized SSH preset still contains Mosh Server")
	}
	if _, ok := meta["ET Server Port"]; ok {
		t.Fatal("normalized SSH preset still contains ET Server Port")
	}
	if meta["Encoding"] != "utf-8" {
		t.Fatalf("normalized Encoding = %q, want utf-8", meta["Encoding"])
	}
	if meta["Future Meta"] != "preserve-me" {
		t.Fatal("normalized preset did not preserve unknown metadata")
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("second CustomFile returned error: %v", err)
	}
	meta = reloaded.Presets[0].Meta
	if _, ok := meta["Mosh Server"]; ok {
		t.Fatal("persisted SSH preset still contains Mosh Server")
	}
	if _, ok := meta["ET Server Port"]; ok {
		t.Fatal("persisted SSH preset still contains ET Server Port")
	}
	if meta["Encoding"] != "utf-8" {
		t.Fatalf("persisted Encoding = %q, want utf-8", meta["Encoding"])
	}
	if meta["Future Meta"] != "preserve-me" {
		t.Fatal("persisted preset did not preserve unknown metadata")
	}
}

func TestNormalizeStartupPresetsKeepsBlankAdminKeyWhenSharedKeyIsSet(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	configData := map[string]any{
		"SharedKey": "test-shared-key",
		"Servers": []map[string]any{
			{"ListenInterface": "127.0.0.1", "ListenPort": 8182},
		},
		"Presets": []map[string]any{
			{"ID": "preset-atlantis", "Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
		},
	}
	content, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	normalized, err := normalizeStartupPresets(cfg, commands.New())
	if err != nil {
		t.Fatalf("normalizeStartupPresets returned error: %v", err)
	}
	if normalized.AdminKey != "" {
		t.Fatalf("normalized AdminKey = %q, want empty", normalized.AdminKey)
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("second CustomFile returned error: %v", err)
	}
	if reloaded.AdminKey != "" {
		t.Fatalf("persisted AdminKey = %q, want empty", reloaded.AdminKey)
	}
}

func TestNormalizeStartupPresetsKeepsExplicitAdminKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	configData := map[string]any{
		"SharedKey": "test-shared-key",
		"AdminKey":  "existing-admin-key",
		"Servers": []map[string]any{
			{"ListenInterface": "127.0.0.1", "ListenPort": 8182},
		},
		"Presets": []map[string]any{
			{"ID": "preset-atlantis", "Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
		},
	}
	content, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	normalized, err := normalizeStartupPresets(cfg, commands.New())
	if err != nil {
		t.Fatalf("normalizeStartupPresets returned error: %v", err)
	}
	if normalized.AdminKey != "existing-admin-key" {
		t.Fatalf(
			"normalized AdminKey = %q, want existing-admin-key",
			normalized.AdminKey,
		)
	}
}

func TestNormalizeStartupPresetsKeepsEnvAdminKeyOutOfFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	configData := map[string]any{
		"SharedKey": "test-shared-key",
		"Servers": []map[string]any{
			{"ListenInterface": "127.0.0.1", "ListenPort": 8182},
		},
		"Presets": []map[string]any{
			{"ID": "preset-atlantis", "Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
		},
	}
	content, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	cfg.AdminKey = "env-admin-key"
	normalized, err := normalizeStartupPresets(cfg, commands.New())
	if err != nil {
		t.Fatalf("normalizeStartupPresets returned error: %v", err)
	}
	if normalized.AdminKey != "env-admin-key" {
		t.Fatalf(
			"normalized AdminKey = %q, want env-admin-key",
			normalized.AdminKey,
		)
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("second CustomFile returned error: %v", err)
	}
	if reloaded.AdminKey != "" {
		t.Fatalf("persisted AdminKey = %q, want empty", reloaded.AdminKey)
	}
}

func TestNormalizeStartupPresetIDsMigratesPlaintextPresetPassword(t *testing.T) {
	t.Setenv(
		configuration.PresetSecretKeyEnv,
		base64.StdEncoding.EncodeToString(
			[]byte("0123456789abcdef0123456789abcdef"),
		),
	)
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	configData := map[string]any{
		"Servers": []map[string]any{
			{"ListenInterface": "127.0.0.1", "ListenPort": 8182},
		},
		"Presets": []map[string]any{
			{
				"Title": "Atlantis",
				"Type":  "SSH",
				"Host":  "atlantis.home",
				"Meta": map[string]string{
					"Authentication": "Password",
					"Password":       "mypassword",
				},
			},
		},
	}
	content, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	normalized, err := normalizeStartupPresets(cfg, commands.New())
	if err != nil {
		t.Fatalf("normalizeStartupPresets returned error: %v", err)
	}
	if normalized.Presets[0].SecretMeta["Password"] != "mypassword" {
		t.Fatal("normalized preset did not keep decrypted password in SecretMeta")
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("second CustomFile returned error: %v", err)
	}
	if _, ok := reloaded.Presets[0].Meta["Password"]; ok {
		t.Fatal("persisted config still contains plaintext Password")
	}
	if reloaded.Presets[0].Meta["Encrypted Password"] == "" {
		t.Fatal("persisted config is missing Encrypted Password")
	}
	if len(reloaded.Presets) != 1 {
		t.Fatalf("persisted preset count = %d, want 1", len(reloaded.Presets))
	}
}

func TestNormalizeStartupPresetIDsAllowsEnvPlaintextPresetPassword(t *testing.T) {
	t.Setenv(
		configuration.PresetSecretKeyEnv,
		base64.StdEncoding.EncodeToString(
			[]byte("0123456789abcdef0123456789abcdef"),
		),
	)
	cfg := configuration.Configuration{
		Presets: []configuration.Preset{
			{
				Title: "Atlantis",
				Type:  "SSH",
				Host:  "atlantis.home",
				Meta: map[string]string{
					"Authentication": "Password",
					"Password":       "mypassword",
				},
			},
		},
	}

	normalized, err := normalizeStartupPresets(cfg, commands.New())
	if err != nil {
		t.Fatalf("normalizeStartupPresets returned error: %v", err)
	}
	if normalized.Presets[0].SecretMeta["Password"] != "mypassword" {
		t.Fatal("normalized preset did not keep decrypted password in SecretMeta")
	}
	if _, ok := normalized.Presets[0].Meta["Password"]; ok {
		t.Fatal("normalized env preset still contains plaintext Password")
	}
	if normalized.Presets[0].Meta["Encrypted Password"] == "" {
		t.Fatal("normalized env preset is missing Encrypted Password")
	}
}

func TestNormalizeStartupPresetsIgnoresUnsupportedEncryptedPassword(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	configData := map[string]any{
		"Servers": []map[string]any{
			{"ListenInterface": "127.0.0.1", "ListenPort": 8182},
		},
		"Presets": []map[string]any{
			{"Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
			{
				"Title": "Future",
				"Type":  "Future",
				"Host":  "future.home",
				"Meta": map[string]string{
					"Encrypted Password": "v1:aes-256-gcm:nonce:ciphertext",
				},
			},
		},
	}
	content, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	normalized, err := normalizeStartupPresets(cfg, commands.New())
	if err != nil {
		t.Fatalf("normalizeStartupPresets returned error: %v", err)
	}
	if len(normalized.Presets) != 1 {
		t.Fatalf("normalized preset count = %d, want 1", len(normalized.Presets))
	}
	if normalized.Presets[0].Type != "SSH" {
		t.Fatalf("normalized preset type = %q, want SSH", normalized.Presets[0].Type)
	}
}

func TestNormalizeStartupPresetIDsAllowsReadOnlyFileBackedIDs(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "readonly")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatalf("os.Mkdir returned error: %v", err)
	}
	configPath := filepath.Join(configDir, "shellport.conf.json")
	configData := map[string]any{
		"Servers": []map[string]any{
			{"ListenInterface": "127.0.0.1", "ListenPort": 8182},
		},
		"Presets": []map[string]any{
			{"Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
		},
	}
	content, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	if err := os.Chmod(configDir, 0o500); err != nil {
		t.Fatalf("os.Chmod returned error: %v", err)
	}
	defer os.Chmod(configDir, 0o700)

	_, cfg, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	normalized, err := normalizeStartupPresets(cfg, commands.New())
	if err != nil {
		t.Fatalf("normalizeStartupPresets returned error: %v", err)
	}
	if normalized.Presets[0].ID == "" {
		t.Fatal("normalized preset ID is empty")
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("second CustomFile returned error: %v", err)
	}
	if reloaded.Presets[0].ID != "" {
		t.Fatalf("persisted ID = %q, want empty", reloaded.Presets[0].ID)
	}
}
