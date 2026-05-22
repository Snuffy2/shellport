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

func TestNormalizeStartupPresetsAllowsReadOnlyInlinePrivateKey(t *testing.T) {
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
			{
				"ID":    "preset-atlantis",
				"Title": "Atlantis",
				"Type":  "SSH",
				"Host":  "atlantis.home",
				"Meta": map[string]string{
					"Authentication": "Private Key",
					"Private Key":    "INLINE PRIVATE KEY DATA",
				},
			},
		},
	}
	content, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(configPath, content, 0o400); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	if err := os.Chmod(configDir, 0o500); err != nil {
		t.Fatalf("os.Chmod returned error: %v", err)
	}
	defer os.Chmod(configDir, 0o700)
	defer os.Chmod(configPath, 0o600)

	_, cfg, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	normalized, err := normalizeStartupPresets(cfg, commands.New())
	if err != nil {
		t.Fatalf("normalizeStartupPresets returned error: %v", err)
	}
	if normalized.Presets[0].Meta["Private Key"] != "INLINE PRIVATE KEY DATA" {
		t.Fatal("normalized preset did not preserve inline private key")
	}
	if _, err := os.Stat(filepath.Join(configDir, "private_keys")); !os.IsNotExist(err) {
		t.Fatalf("private_keys directory stat error = %v, want not exist", err)
	}
}

func TestNormalizeStartupPresetsMigratesPlaintextPrivateKeysToFiles(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "shellport.conf.json")
	configData := map[string]any{
		"Servers": []map[string]any{
			{"ListenInterface": "127.0.0.1", "ListenPort": 8182},
		},
		"Presets": []map[string]any{
			{
				"ID":    "preset-atlantis",
				"Title": "Atlantis Main",
				"Type":  "SSH",
				"Host":  "atlantis.home",
				"Meta": map[string]string{
					"Authentication": "Private Key",
					"Private Key":    "INLINE PRIVATE KEY DATA",
				},
			},
			{
				"ID":    "preset-literal",
				"Title": "Literal Key",
				"Type":  "Mosh",
				"Host":  "literal.home",
				"Meta": map[string]string{
					"Authentication": "Private Key",
					"Private Key":    "literal://LITERAL PRIVATE KEY DATA",
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

	resolvedConfigDir, err := filepath.EvalSymlinks(configDir)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks returned error: %v", err)
	}
	keyDir := filepath.Join(resolvedConfigDir, "private_keys")
	for _, tc := range []struct {
		name string
		file string
		want string
	}{
		{name: "inline", file: "atlantis-main.key", want: "INLINE PRIVATE KEY DATA"},
		{name: "literal", file: "literal-key.key", want: "LITERAL PRIVATE KEY DATA"},
	} {
		keyPath := filepath.Join(keyDir, tc.file)
		data, err := os.ReadFile(keyPath)
		if err != nil {
			t.Fatalf("%s key os.ReadFile returned error: %v", tc.name, err)
		}
		if string(data) != tc.want {
			t.Fatalf("%s key data = %q, want %q", tc.name, string(data), tc.want)
		}
		info, err := os.Stat(keyPath)
		if err != nil {
			t.Fatalf("%s key os.Stat returned error: %v", tc.name, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s key mode = %o, want 0600", tc.name, info.Mode().Perm())
		}
	}
	if normalized.Presets[0].Meta["Private Key"] != "file://"+filepath.Join(keyDir, "atlantis-main.key") {
		t.Fatal("normalized inline preset did not use migrated private key file")
	}
	if normalized.Presets[1].Meta["Private Key"] != "file://"+filepath.Join(keyDir, "literal-key.key") {
		t.Fatal("normalized literal preset did not use migrated private key file")
	}

	var raw struct {
		Presets []struct {
			Meta map[string]configuration.String
		}
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile config returned error: %v", err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if raw.Presets[0].Meta["Private Key"] != configuration.String("file://"+filepath.Join(keyDir, "atlantis-main.key")) {
		t.Fatalf("raw inline private key = %q", raw.Presets[0].Meta["Private Key"])
	}
	if raw.Presets[1].Meta["Private Key"] != configuration.String("file://"+filepath.Join(keyDir, "literal-key.key")) {
		t.Fatalf("raw literal private key = %q", raw.Presets[1].Meta["Private Key"])
	}
}

func TestNormalizeStartupPresetsPreservesEnvironmentPrivateKeys(t *testing.T) {
	t.Setenv("SHELLPORT_TEST_PRIVATE_KEY", "ENV PRIVATE KEY DATA")
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "shellport.conf.json")
	configData := map[string]any{
		"Servers": []map[string]any{
			{"ListenInterface": "127.0.0.1", "ListenPort": 8182},
		},
		"Presets": []map[string]any{
			{
				"ID":    "preset-env",
				"Title": "Env Key",
				"Type":  "SSH",
				"Host":  "env.home",
				"Meta": map[string]string{
					"Authentication": "Private Key",
					"Private Key":    "environment://SHELLPORT_TEST_PRIVATE_KEY",
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
	if normalized.Presets[0].Meta["Private Key"] != "environment://SHELLPORT_TEST_PRIVATE_KEY" {
		t.Fatal("normalized env preset did not keep environment private key reference")
	}
	if _, err := os.Stat(filepath.Join(configDir, "private_keys")); !os.IsNotExist(err) {
		t.Fatalf("private_keys directory stat error = %v, want not exist", err)
	}

	var raw struct {
		Presets []struct {
			Meta map[string]configuration.String
		}
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile config returned error: %v", err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if raw.Presets[0].Meta["Private Key"] != "environment://SHELLPORT_TEST_PRIVATE_KEY" {
		t.Fatalf("raw private key = %q", raw.Presets[0].Meta["Private Key"])
	}
}
