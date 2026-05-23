// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package configuration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writePresetConfig(t *testing.T, path string, presets []map[string]any) {
	t.Helper()

	data := map[string]any{
		"Servers": []map[string]any{
			{"ListenInterface": "127.0.0.1", "ListenPort": 8182},
		},
		"Presets": presets,
	}
	content, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("yaml.Marshal returned error: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
}

func requirePresetCount(t *testing.T, presets []Preset, want int) {
	t.Helper()

	if len(presets) != want {
		t.Fatalf("preset count = %d, want %d", len(presets), want)
	}
}

func requireRawPresetCount(t *testing.T, presets presetInputs, want int) {
	t.Helper()

	if len(presets) != want {
		t.Fatalf("raw preset count = %d, want %d", len(presets), want)
	}
}

func TestSafePresetInputCapacity(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	tests := []struct {
		name      string
		rawLen    int
		presetLen int
		want      int
	}{
		{
			name:      "adds safe lengths",
			rawLen:    2,
			presetLen: 3,
			want:      5,
		},
		{
			name:      "drops capacity hint before overflow",
			rawLen:    maxInt,
			presetLen: 1,
			want:      0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := safePresetInputCapacity(test.rawLen, test.presetLen)
			if got != test.want {
				t.Fatalf("safePresetInputCapacity() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestLoadFileRecordsSourceFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	writePresetConfig(t, configPath, []map[string]any{
		{"ID": "preset-existing", "Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
	})

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	if cfg.SourceFile != configPath {
		t.Fatalf("SourceFile = %q, want %q", cfg.SourceFile, configPath)
	}
}

func TestLoadFileDoesNotReadAdminPasswordFromEnvironment(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "env-admin-password")
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	writePresetConfig(t, configPath, []map[string]any{
		{"ID": "preset-existing", "Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
	})

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	if cfg.AdminPassword != "" {
		t.Fatalf("AdminPassword = %q, want empty", cfg.AdminPassword)
	}
}

func TestPersistPresetIDsAddsMissingIDsToConfigFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	writePresetConfig(t, configPath, []map[string]any{
		{"Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
	})

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	presets, changed, err := EnsurePresetIDs(cfg.Presets)
	if err != nil {
		t.Fatalf("EnsurePresetIDs returned error: %v", err)
	}
	if !changed {
		t.Fatal("EnsurePresetIDs changed = false, want true")
	}
	if err := PersistPresetIDs(cfg.SourceFile, presets); err != nil {
		t.Fatalf("PersistPresetIDs returned error: %v", err)
	}

	_, reloaded, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("second loadFile returned error: %v", err)
	}
	requirePresetCount(t, reloaded.Presets, 1)
	if reloaded.Presets[0].ID == "" {
		t.Fatal("reloaded preset ID is empty")
	}
}

func TestPersistPresetIDsPreservesUnknownTopLevelFields(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
Presets:
  - Title: Atlantis
    Type: SSH
    Host: atlantis.home
FutureTopLevel:
  enabled: true
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	presets, _, err := EnsurePresetIDs(cfg.Presets)
	if err != nil {
		t.Fatalf("EnsurePresetIDs returned error: %v", err)
	}
	if err := PersistPresetIDs(cfg.SourceFile, presets); err != nil {
		t.Fatalf("PersistPresetIDs returned error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	raw, err := decodeYAMLMap(data)
	if err != nil {
		t.Fatalf("decodeYAMLMap returned error: %v", err)
	}
	if _, ok := raw["FutureTopLevel"]; !ok {
		t.Fatal("unknown top-level field was not preserved")
	}
}

func TestReplaceFilePresetsPreservesRawMetaValues(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY DATA"), 0o600); err != nil {
		t.Fatalf("os.WriteFile key returned error: %v", err)
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
				"Private Key":    "file://" + keyPath,
			},
		},
	})

	_, cfg, err := loadFile(configPath)
	if err != nil {
		t.Fatalf("loadFile returned error: %v", err)
	}
	requirePresetCount(t, cfg.Presets, 1)
	preset := cfg.Presets[0]
	preset.Meta["Fingerprint"] = "SHA256:abc"
	if err := ReplaceFilePresets(configPath, []Preset{preset}); err != nil {
		t.Fatalf("ReplaceFilePresets returned error: %v", err)
	}

	raw, _, err := readCommonInputFile(configPath)
	if err != nil {
		t.Fatalf("readCommonInputFile returned error: %v", err)
	}
	requireRawPresetCount(t, raw.Presets, 1)
	if raw.Presets[0].Meta["Private Key"] != String("file://"+keyPath) {
		t.Fatalf(
			"raw private key = %q, want file URI",
			raw.Presets[0].Meta["Private Key"],
		)
	}
	if raw.Presets[0].Meta["Fingerprint"] != "SHA256:abc" {
		t.Fatal("raw fingerprint was not updated")
	}
}

func TestReplaceFilePresetsPreservesUnsupportedRawPresets(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	writePresetConfig(t, configPath, []map[string]any{
		{"ID": "preset-atlantis", "Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home:22"},
		{"ID": "preset-future", "Title": "Future", "Type": "Future", "Host": "future.home"},
	})

	if err := ReplaceFilePresets(configPath, []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"Fingerprint": "SHA256:abc",
			},
		},
	}); err != nil {
		t.Fatalf("ReplaceFilePresets returned error: %v", err)
	}

	raw, _, err := readCommonInputFile(configPath)
	if err != nil {
		t.Fatalf("readCommonInputFile returned error: %v", err)
	}
	requireRawPresetCount(t, raw.Presets, 2)
	if raw.Presets[1].ID != "preset-future" {
		t.Fatalf("second raw preset ID = %q, want preset-future", raw.Presets[1].ID)
	}
}

func TestReplaceFilePresetsPreservesUnknownPresetFields(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
Presets:
  - ID: preset-atlantis
    Title: Atlantis
    Type: SSH
    Host: atlantis.home:22
    FuturePresetField:
      enabled: true
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := ReplaceFilePresets(configPath, []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"Fingerprint": "SHA256:abc",
			},
		},
	}); err != nil {
		t.Fatalf("ReplaceFilePresets returned error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	raw, err := decodeYAMLMap(data)
	if err != nil {
		t.Fatalf("decodeYAMLMap returned error: %v", err)
	}
	rawPresets, err := rawPresetMaps(raw["Presets"])
	if err != nil {
		t.Fatalf("rawPresetMaps returned error: %v", err)
	}
	if len(rawPresets) != 1 {
		t.Fatalf("raw preset count = %d, want 1", len(rawPresets))
	}
	if _, ok := rawPresets[0]["FuturePresetField"]; !ok {
		t.Fatal("unknown preset field was not preserved")
	}
}

func TestReplaceFilePresetsPreservesUnknownPresetScalarLexemes(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
Presets:
  - ID: preset-atlantis
    Title: Atlantis
    Type: SSH
    Host: atlantis.home:22
    FuturePresetCode: 0123
    FuturePresetFlag: yes
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := ReplaceFilePresets(configPath, []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"Fingerprint": "SHA256:abc",
			},
		},
	}); err != nil {
		t.Fatalf("ReplaceFilePresets returned error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(data), "FuturePresetCode: 0123") {
		t.Fatalf("future preset code scalar was not preserved:\n%s", data)
	}
	if !strings.Contains(string(data), "FuturePresetFlag: yes") {
		t.Fatalf("future preset flag scalar was not preserved:\n%s", data)
	}
}

func TestReplaceFilePresetsSkipsYAMLPresetMergeKeys(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
PresetDefaults: &presetDefaults
  FuturePresetFlag: true
Presets:
  - <<: *presetDefaults
    ID: preset-atlantis
    Title: Atlantis
    Type: SSH
    Host: atlantis.home:22
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := ReplaceFilePresets(configPath, []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"Fingerprint": "SHA256:abc",
			},
		},
	}); err != nil {
		t.Fatalf("ReplaceFilePresets returned error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if strings.Contains(string(data), "<<:") {
		t.Fatalf("merge key was preserved as a preset field:\n%s", data)
	}
	if !strings.Contains(string(data), "FuturePresetFlag: true") {
		t.Fatalf("merged future preset field was not materialized:\n%s", data)
	}
}

func TestReplaceFilePresetsMaterializesUnknownPresetAliases(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
FutureDefaults: &futureDefaults
  enabled: true
Presets:
  - ID: preset-atlantis
    Title: Atlantis
    Type: SSH
    Host: atlantis.home:22
    FuturePresetField: *futureDefaults
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := ReplaceFilePresets(configPath, []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"Fingerprint": "SHA256:abc",
			},
		},
	}); err != nil {
		t.Fatalf("ReplaceFilePresets returned error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if strings.Contains(string(data), "FuturePresetField: *futureDefaults") {
		t.Fatalf("unknown preset alias was not materialized:\n%s", data)
	}
	if !strings.Contains(string(data), "FuturePresetField:") ||
		!strings.Contains(string(data), "enabled: true") {
		t.Fatalf("materialized unknown preset field was not preserved:\n%s", data)
	}
}

func TestReplaceFilePresetsPreservesUnknownTopLevelFields(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
Presets:
  - ID: preset-atlantis
    Title: Atlantis
    Type: SSH
    Host: atlantis.home:22
FutureTopLevel:
  enabled: true
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := ReplaceFilePresets(configPath, []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"Fingerprint": "SHA256:abc",
			},
		},
	}); err != nil {
		t.Fatalf("ReplaceFilePresets returned error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	raw, err := decodeYAMLMap(data)
	if err != nil {
		t.Fatalf("decodeYAMLMap returned error: %v", err)
	}
	if _, ok := raw["FutureTopLevel"]; !ok {
		t.Fatal("unknown top-level field was not preserved")
	}
}

func TestReplaceFilePresetsPreservesUnchangedConfigComments(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	content := []byte(`# listener settings should keep this operator note
Servers:
  - ListenInterface: 127.0.0.1
    ListenPort: 8182
Presets:
  - ID: preset-atlantis
    Title: Atlantis
    Type: SSH
    Host: atlantis.home:22
# future setting should keep this operator note
FutureTopLevel:
  enabled: true
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := ReplaceFilePresets(configPath, []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"Fingerprint": "SHA256:abc",
			},
		},
	}); err != nil {
		t.Fatalf("ReplaceFilePresets returned error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if !strings.Contains(
		string(data),
		"# listener settings should keep this operator note",
	) {
		t.Fatalf("listener comment was not preserved:\n%s", data)
	}
	if !strings.Contains(
		string(data),
		"# future setting should keep this operator note",
	) {
		t.Fatal("future setting comment was not preserved")
	}
}

func TestReplaceFilePresetsWithRuntimeDoesNotResolveRawMeta(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY DATA"), 0o600); err != nil {
		t.Fatalf("os.WriteFile key returned error: %v", err)
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
				"Private Key":    "file://" + keyPath,
			},
		},
	})
	runtime := []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"User":           "pi",
				"Authentication": "Private Key",
				"Private Key":    "PRIVATE KEY DATA",
			},
		},
	}
	next := []Preset{
		{
			ID:       runtime[0].ID,
			Title:    runtime[0].Title,
			Type:     runtime[0].Type,
			Host:     runtime[0].Host,
			TabColor: runtime[0].TabColor,
			Meta: map[string]string{
				"User":           runtime[0].Meta["User"],
				"Authentication": runtime[0].Meta["Authentication"],
				"Private Key":    runtime[0].Meta["Private Key"],
				"Fingerprint":    "SHA256:abc",
			},
		},
	}

	if err := ReplaceFilePresetsWithRuntime(configPath, next, runtime, nil); err != nil {
		t.Fatalf("ReplaceFilePresetsWithRuntime returned error: %v", err)
	}

	raw, _, err := readCommonInputFile(configPath)
	if err != nil {
		t.Fatalf("readCommonInputFile returned error: %v", err)
	}
	requireRawPresetCount(t, raw.Presets, 1)
	if raw.Presets[0].Meta["Private Key"] != String("file://"+keyPath) {
		t.Fatalf("raw private key = %q, want file URI", raw.Presets[0].Meta["Private Key"])
	}
}

func TestReplaceFilePresetsWithRuntimePreservesRotatedRawMetaReference(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY DATA"), 0o600); err != nil {
		t.Fatalf("os.WriteFile key returned error: %v", err)
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
				"Private Key":    "file://" + keyPath,
			},
		},
	})
	runtime := []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"User":           "pi",
				"Authentication": "Private Key",
				"Private Key":    "PRIVATE KEY DATA",
			},
		},
	}
	if err := os.WriteFile(keyPath, []byte("UPDATED PRIVATE KEY DATA"), 0o600); err != nil {
		t.Fatalf("os.WriteFile rotated key returned error: %v", err)
	}
	next := []Preset{
		{
			ID:       runtime[0].ID,
			Title:    runtime[0].Title,
			Type:     runtime[0].Type,
			Host:     runtime[0].Host,
			TabColor: runtime[0].TabColor,
			Meta: map[string]string{
				"User":           runtime[0].Meta["User"],
				"Authentication": runtime[0].Meta["Authentication"],
				"Private Key":    "UPDATED PRIVATE KEY DATA",
				"Fingerprint":    "SHA256:abc",
			},
		},
	}

	if err := ReplaceFilePresetsWithRuntime(configPath, next, runtime, nil); err != nil {
		t.Fatalf("ReplaceFilePresetsWithRuntime returned error: %v", err)
	}

	raw, _, err := readCommonInputFile(configPath)
	if err != nil {
		t.Fatalf("readCommonInputFile returned error: %v", err)
	}
	requireRawPresetCount(t, raw.Presets, 1)
	if raw.Presets[0].Meta["Private Key"] != String("file://"+keyPath) {
		t.Fatalf("raw private key = %q, want file URI", raw.Presets[0].Meta["Private Key"])
	}
}

func TestReplaceFilePresetsWithRuntimeAllowsInlinePrivateKeyReplacement(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY DATA"), 0o600); err != nil {
		t.Fatalf("os.WriteFile key returned error: %v", err)
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
				"Private Key":    "file://" + keyPath,
			},
		},
	})
	runtime := []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"User":           "pi",
				"Authentication": "Private Key",
				"Private Key":    "PRIVATE KEY DATA",
			},
		},
	}
	next := []Preset{
		{
			ID:       runtime[0].ID,
			Title:    runtime[0].Title,
			Type:     runtime[0].Type,
			Host:     runtime[0].Host,
			TabColor: runtime[0].TabColor,
			Meta: map[string]string{
				"User":           runtime[0].Meta["User"],
				"Authentication": runtime[0].Meta["Authentication"],
				"Private Key":    "INLINE PRIVATE KEY DATA",
				"Fingerprint":    "SHA256:abc",
			},
		},
	}

	if err := ReplaceFilePresetsWithRuntime(configPath, next, runtime, nil); err != nil {
		t.Fatalf("ReplaceFilePresetsWithRuntime returned error: %v", err)
	}

	raw, _, err := readCommonInputFile(configPath)
	if err != nil {
		t.Fatalf("readCommonInputFile returned error: %v", err)
	}
	requireRawPresetCount(t, raw.Presets, 1)
	if raw.Presets[0].Meta["Private Key"] != String("INLINE PRIVATE KEY DATA") {
		t.Fatalf("raw private key = %q, want inline data", raw.Presets[0].Meta["Private Key"])
	}
}

func TestReplaceFilePresetsWithRuntimePreservesOmittedRawOnlyMeta(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	writePresetConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home:22",
			"Meta": map[string]any{
				"User":            "pi",
				"Future Raw Meta": "preserve-me",
			},
		},
	})
	runtime := []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"User": "pi",
			},
		},
	}
	next := []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"User":        "pi",
				"Fingerprint": "SHA256:abc",
			},
		},
	}

	if err := ReplaceFilePresetsWithRuntime(configPath, next, runtime, nil); err != nil {
		t.Fatalf("ReplaceFilePresetsWithRuntime returned error: %v", err)
	}

	raw, _, err := readCommonInputFile(configPath)
	if err != nil {
		t.Fatalf("readCommonInputFile returned error: %v", err)
	}
	requireRawPresetCount(t, raw.Presets, 1)
	if raw.Presets[0].Meta["Future Raw Meta"] != String("preserve-me") {
		t.Fatal("raw-only metadata was not preserved")
	}
	if raw.Presets[0].Meta["Fingerprint"] != "SHA256:abc" {
		t.Fatal("raw fingerprint was not updated")
	}
}

func TestReplaceFilePresetsWithRuntimeDropsKnownMetaForWrongType(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	writePresetConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home:22",
			"Meta": map[string]any{
				"User":            "pi",
				"Mosh Server":     "mosh-server",
				"ET Server Port":  "2022",
				"Future Raw Meta": "preserve-me",
			},
		},
	})
	runtime := []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"User":     "pi",
				"Encoding": "utf-8",
			},
		},
	}
	next := []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"User":     "pi",
				"Encoding": "utf-8",
			},
		},
	}

	if err := ReplaceFilePresetsWithRuntime(configPath, next, runtime, nil); err != nil {
		t.Fatalf("ReplaceFilePresetsWithRuntime returned error: %v", err)
	}

	raw, _, err := readCommonInputFile(configPath)
	if err != nil {
		t.Fatalf("readCommonInputFile returned error: %v", err)
	}
	requireRawPresetCount(t, raw.Presets, 1)
	if _, ok := raw.Presets[0].Meta["Mosh Server"]; ok {
		t.Fatal("Mosh Server metadata was preserved for SSH preset")
	}
	if _, ok := raw.Presets[0].Meta["ET Server Port"]; ok {
		t.Fatal("ET Server Port metadata was preserved for SSH preset")
	}
	if raw.Presets[0].Meta["Encoding"] != String("utf-8") {
		t.Fatal("default Encoding metadata was not persisted")
	}
	if raw.Presets[0].Meta["Future Raw Meta"] != String("preserve-me") {
		t.Fatal("unknown raw-only metadata was not preserved")
	}
}

func TestReplaceFilePresetsUpdatesSymlinkTarget(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "shellport.conf.yaml")
	linkPath := filepath.Join(tempDir, "linked.conf.yaml")
	writePresetConfig(t, configPath, []map[string]any{
		{"ID": "preset-atlantis", "Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home:22"},
	})
	if err := os.Symlink(configPath, linkPath); err != nil {
		t.Fatalf("os.Symlink returned error: %v", err)
	}

	if err := ReplaceFilePresets(linkPath, []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "columbia.home:22",
		},
	}); err != nil {
		t.Fatalf("ReplaceFilePresets returned error: %v", err)
	}

	linkInfo, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("os.Lstat returned error: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal("config symlink was replaced")
	}
	raw, _, err := readCommonInputFile(configPath)
	if err != nil {
		t.Fatalf("readCommonInputFile returned error: %v", err)
	}
	requireRawPresetCount(t, raw.Presets, 1)
	if raw.Presets[0].Host != "columbia.home:22" {
		t.Fatalf("target host = %q, want columbia.home:22", raw.Presets[0].Host)
	}
}

func TestReplaceFilePresetsPreservesOmittedRawMetaKeys(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.yaml")
	writePresetConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home:22",
			"Meta": map[string]any{
				"User":        "pi",
				"Fingerprint": "SHA256:abc",
			},
		},
	})

	if err := ReplaceFilePresets(configPath, []Preset{
		{
			ID:    "preset-atlantis",
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				"User": "pi",
			},
		},
	}); err != nil {
		t.Fatalf("ReplaceFilePresets returned error: %v", err)
	}

	raw, _, err := readCommonInputFile(configPath)
	if err != nil {
		t.Fatalf("readCommonInputFile returned error: %v", err)
	}
	requireRawPresetCount(t, raw.Presets, 1)
	if raw.Presets[0].Meta["Fingerprint"] != String("SHA256:abc") {
		t.Fatal("raw fingerprint was not preserved")
	}
}
