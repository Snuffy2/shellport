// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
)

func decodeAccessConfigForTest(t *testing.T, cfg socketAccessConfiguration) map[string]any {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal(buildAccessConfigRespondBody(cfg), &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	return decoded
}

func TestSocketAccessConfigurationIncludesPresetManagementPolicy(t *testing.T) {
	writableSourceFile := filepath.Join(t.TempDir(), "shellport.conf.json")
	if err := os.WriteFile(writableSourceFile, []byte("{}"), 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	tests := []struct {
		name                string
		commonCfg           configuration.Common
		role                authRole
		wantWritable        bool
		wantCanManage       bool
		wantRequiresAdmin   bool
		wantBlockedByPreset bool
	}{
		{
			name: "non writable config",
			commonCfg: configuration.Common{
				SourceFile: "",
			},
			role:                authRoleAdmin,
			wantWritable:        false,
			wantCanManage:       false,
			wantRequiresAdmin:   false,
			wantBlockedByPreset: false,
		},
		{
			name: "blank admin password writes immediately",
			commonCfg: configuration.Common{
				SourceFile:    writableSourceFile,
				AdminPassword: "",
			},
			role:                authRoleAdmin,
			wantWritable:        true,
			wantCanManage:       true,
			wantRequiresAdmin:   false,
			wantBlockedByPreset: false,
		},
		{
			name: "admin password prompt required for user role",
			commonCfg: configuration.Common{
				SourceFile:    writableSourceFile,
				AdminPassword: "admin-secret",
			},
			role:                authRoleUser,
			wantWritable:        true,
			wantCanManage:       true,
			wantRequiresAdmin:   true,
			wantBlockedByPreset: false,
		},
		{
			name: "preset restriction blocks management",
			commonCfg: configuration.Common{
				SourceFile:             writableSourceFile,
				AdminPassword:          "",
				OnlyAllowPresetRemotes: true,
			},
			role:                authRoleAdmin,
			wantWritable:        true,
			wantCanManage:       false,
			wantRequiresAdmin:   false,
			wantBlockedByPreset: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := newPresetManagementPolicy(tt.commonCfg, tt.role)
			if policy.Writable != tt.wantWritable {
				t.Fatalf("Writable = %v, want %v", policy.Writable, tt.wantWritable)
			}
			if policy.CanManage != tt.wantCanManage {
				t.Fatalf("CanManage = %v, want %v", policy.CanManage, tt.wantCanManage)
			}
			if policy.RequiresAdminPassword != tt.wantRequiresAdmin {
				t.Fatalf(
					"RequiresAdminPassword = %v, want %v",
					policy.RequiresAdminPassword,
					tt.wantRequiresAdmin,
				)
			}
			if policy.BlockedByPresetRestriction != tt.wantBlockedByPreset {
				t.Fatalf(
					"BlockedByPresetRestriction = %v, want %v",
					policy.BlockedByPresetRestriction,
					tt.wantBlockedByPreset,
				)
			}
		})
	}
}

func TestSocketAccessConfigurationMarksHiddenSavedPassword(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("os.WriteFile config returned error: %v", err)
	}
	cfg := newSocketAccessConfiguration(
		[]configuration.Preset{
			{
				ID:    "preset-password",
				Title: "Password",
				Type:  "SSH",
				Host:  "example.com:22",
				Meta: map[string]string{
					"Authentication":                          "Password",
					configuration.PresetMetaPrivateKey:        "file:///config/private_keys/atlantis",
					configuration.PresetMetaEncryptedPassword: "v1:aes-256-gcm:nonce:ciphertext",
				},
			},
		},
		"",
		"",
		true,
		newPresetManagementPolicy(configuration.Common{
			SourceFile:    configPath,
			AdminPassword: "admin-secret",
		}, authRoleAdmin),
	)

	decoded := decodeAccessConfigForTest(t, cfg)
	presets := decoded["presets"].([]any)
	preset := presets[0].(map[string]any)
	meta := preset["meta"].(map[string]any)

	if preset["has_saved_password"] != true {
		t.Fatalf("has_saved_password = %v, want true", preset["has_saved_password"])
	}
	if preset["has_saved_private_key"] != true {
		t.Fatalf("has_saved_private_key = %v, want true", preset["has_saved_private_key"])
	}
	if preset["private_key_file"] != "file:///config/private_keys/atlantis" {
		t.Fatalf(
			"private_key_file = %q, want file:///config/private_keys/atlantis",
			preset["private_key_file"],
		)
	}
	if preset["private_key_filename"] != "atlantis" {
		t.Fatalf(
			"private_key_filename = %q, want atlantis",
			preset["private_key_filename"],
		)
	}
	if _, ok := meta[configuration.PresetMetaPassword]; ok {
		t.Fatal("plaintext password leaked into socket preset metadata")
	}
	if _, ok := meta[configuration.PresetMetaEncryptedPassword]; ok {
		t.Fatal("encrypted password leaked into socket preset metadata")
	}
	if _, ok := meta[configuration.PresetMetaPrivateKey]; ok {
		t.Fatal("private key leaked into socket preset metadata")
	}
}

func TestSocketAccessConfigurationHidesPrivateKeyFileUntilManageAllowed(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("os.WriteFile config returned error: %v", err)
	}
	preset := configuration.Preset{
		ID:    "preset-atlantis",
		Title: "Atlantis",
		Type:  "SSH",
		Host:  "atlantis.home:22",
		Meta: map[string]string{
			"Authentication":                   "Private Key",
			configuration.PresetMetaPrivateKey: "file:///config/private_keys/atlantis.key",
		},
	}

	cfg := newSocketAccessConfiguration(
		[]configuration.Preset{preset},
		"",
		"",
		false,
		newPresetManagementPolicy(configuration.Common{
			SourceFile:    configPath,
			AdminPassword: "admin-secret",
		}, authRoleUser),
	)
	if cfg.Presets[0].PrivateKeyFile != "" {
		t.Fatalf("user PrivateKeyFile = %q, want empty", cfg.Presets[0].PrivateKeyFile)
	}
	if cfg.Presets[0].PrivateKeyFilename != "atlantis.key" {
		t.Fatalf(
			"user PrivateKeyFilename = %q, want atlantis.key",
			cfg.Presets[0].PrivateKeyFilename,
		)
	}

	cfg = newSocketAccessConfiguration(
		[]configuration.Preset{preset},
		"",
		"",
		false,
		newPresetManagementPolicy(configuration.Common{
			SourceFile:    configPath,
			AdminPassword: "admin-secret",
		}, authRoleAdmin),
	)
	if cfg.Presets[0].PrivateKeyFile != "file:///config/private_keys/atlantis.key" {
		t.Fatalf(
			"admin PrivateKeyFile = %q, want file:///config/private_keys/atlantis.key",
			cfg.Presets[0].PrivateKeyFile,
		)
	}
	if cfg.Presets[0].PrivateKeyFilename != "atlantis.key" {
		t.Fatalf(
			"admin PrivateKeyFilename = %q, want atlantis.key",
			cfg.Presets[0].PrivateKeyFilename,
		)
	}
}

func TestSocketAccessConfigurationHandlesFilePrivateKeyURIVariants(t *testing.T) {
	preset := configuration.Preset{
		ID:    "preset-atlantis",
		Title: "Atlantis",
		Type:  "SSH",
		Host:  "atlantis.home:22",
		Meta: map[string]string{
			"Authentication":                   "Private Key",
			configuration.PresetMetaPrivateKey: `FILE://C:\config\private_keys\atlantis.key`,
		},
	}

	cfg := newSocketAccessConfiguration(
		[]configuration.Preset{preset},
		"",
		"",
		false,
		presetManagementPolicy{
			Writable:  true,
			CanManage: true,
		},
	)

	if cfg.Presets[0].PrivateKeyFile != `FILE://C:\config\private_keys\atlantis.key` {
		t.Fatalf(
			"PrivateKeyFile = %q, want original FILE URI",
			cfg.Presets[0].PrivateKeyFile,
		)
	}
	if cfg.Presets[0].PrivateKeyFilename != "atlantis.key" {
		t.Fatalf(
			"PrivateKeyFilename = %q, want atlantis.key",
			cfg.Presets[0].PrivateKeyFilename,
		)
	}
}

func TestSocketAccessConfigurationListsPrivateKeyFilesOnlyWhenManageable(
	t *testing.T,
) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "shellport.conf.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("os.WriteFile config returned error: %v", err)
	}
	keyDir := filepath.Join(configDir, "private_keys")
	if err := os.Mkdir(keyDir, 0o700); err != nil {
		t.Fatalf("os.Mkdir keyDir returned error: %v", err)
	}
	keyPath := filepath.Join(keyDir, "atlantis.key")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY DATA"), 0o600); err != nil {
		t.Fatalf("os.WriteFile key returned error: %v", err)
	}

	cfg := newSocketAccessConfiguration(
		nil,
		"",
		"",
		false,
		newPresetManagementPolicy(configuration.Common{
			SourceFile:             configPath,
			OnlyAllowPresetRemotes: true,
		}, authRoleAdmin),
	)
	cfg = socketAccessConfigurationWithPrivateKeyFiles(cfg, configuration.Common{
		SourceFile:             configPath,
		OnlyAllowPresetRemotes: true,
	})
	if len(cfg.PrivateKeyFiles) != 0 {
		t.Fatalf("blocked policy PrivateKeyFiles count = %d, want 0", len(cfg.PrivateKeyFiles))
	}

	cfg = newSocketAccessConfiguration(
		nil,
		"",
		"",
		false,
		newPresetManagementPolicy(configuration.Common{
			SourceFile:    configPath,
			AdminPassword: "admin-secret",
		}, authRoleUser),
	)
	cfg = socketAccessConfigurationWithPrivateKeyFiles(cfg, configuration.Common{
		SourceFile:    configPath,
		AdminPassword: "admin-secret",
	})
	if len(cfg.PrivateKeyFiles) != 0 {
		t.Fatalf("admin prompt policy PrivateKeyFiles count = %d, want 0", len(cfg.PrivateKeyFiles))
	}

	cfg = newSocketAccessConfiguration(
		nil,
		"",
		"",
		false,
		newPresetManagementPolicy(configuration.Common{
			SourceFile: configPath,
		}, authRoleAdmin),
	)
	cfg = socketAccessConfigurationWithPrivateKeyFiles(cfg, configuration.Common{
		SourceFile: configPath,
	})
	if len(cfg.PrivateKeyFiles) != 1 {
		t.Fatalf("manageable policy PrivateKeyFiles count = %d, want 1", len(cfg.PrivateKeyFiles))
	}
	resolvedKeyPath, err := filepath.EvalSymlinks(keyPath)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks returned error: %v", err)
	}
	if cfg.PrivateKeyFiles[0] != "file://"+resolvedKeyPath {
		t.Fatalf("PrivateKeyFiles[0] = %q, want file://%s", cfg.PrivateKeyFiles[0], resolvedKeyPath)
	}
}

func TestSocketVerificationAdvertisesReadAndHeartbeatTimeoutHeaders(t *testing.T) {
	serverCfg := configuration.Server{
		HeartbeatTimeout: 15 * time.Second,
		ReadTimeout:      2 * time.Minute,
	}
	verification := newSocketVerification(
		socket{},
		serverCfg,
		configuration.Common{},
	)

	request := httptest.NewRequest("GET", "/shellport/socket/verify", nil)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := verification.Get(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	gotHeartbeat := recorder.Result().Header.Get("X-Heartbeat")
	wantHeartbeat := strconv.FormatFloat(serverCfg.HeartbeatTimeout.Seconds(), 'g', 2, 64)
	if gotHeartbeat != wantHeartbeat {
		t.Fatalf("X-Heartbeat = %q, want %q", gotHeartbeat, wantHeartbeat)
	}

	gotTimeout := recorder.Result().Header.Get("X-Timeout")
	wantTimeout := strconv.FormatFloat(serverCfg.ReadTimeout.Seconds(), 'g', 2, 64)
	if gotTimeout != wantTimeout {
		t.Fatalf("X-Timeout = %q, want %q", gotTimeout, wantTimeout)
	}
}
