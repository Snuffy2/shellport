// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Snuffy2/shellport/application/configuration"
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
			name: "blank admin key writes immediately",
			commonCfg: configuration.Common{
				SourceFile: writableSourceFile,
				AdminKey:   "",
			},
			role:                authRoleAdmin,
			wantWritable:        true,
			wantCanManage:       true,
			wantRequiresAdmin:   false,
			wantBlockedByPreset: false,
		},
		{
			name: "admin key prompt required for user role",
			commonCfg: configuration.Common{
				SourceFile: writableSourceFile,
				AdminKey:   "admin-secret",
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
				AdminKey:               "",
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
			if policy.RequiresAdminKey != tt.wantRequiresAdmin {
				t.Fatalf(
					"RequiresAdminKey = %v, want %v",
					policy.RequiresAdminKey,
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
	cfg := newSocketAccessConfiguration(
		[]configuration.Preset{
			{
				ID:    "preset-password",
				Title: "Password",
				Type:  "SSH",
				Host:  "example.com:22",
				Meta: map[string]string{
					"Authentication": "Password",
					configuration.PresetMetaEncryptedPassword: "v1:aes-256-gcm:nonce:ciphertext",
				},
			},
		},
		"",
		"",
		true,
		newPresetManagementPolicy(configuration.Common{
			SourceFile: "dummy.conf.json",
			AdminKey:   "admin-secret",
		}, authRoleAdmin),
	)

	decoded := decodeAccessConfigForTest(t, cfg)
	presets := decoded["presets"].([]any)
	preset := presets[0].(map[string]any)
	meta := preset["meta"].(map[string]any)

	if preset["has_saved_password"] != true {
		t.Fatalf("has_saved_password = %v, want true", preset["has_saved_password"])
	}
	if _, ok := meta[configuration.PresetMetaPassword]; ok {
		t.Fatal("plaintext password leaked into socket preset metadata")
	}
	if _, ok := meta[configuration.PresetMetaEncryptedPassword]; ok {
		t.Fatal("encrypted password leaked into socket preset metadata")
	}
}
