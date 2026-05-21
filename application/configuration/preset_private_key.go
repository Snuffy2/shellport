// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package configuration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	PresetMetaPrivateKey = "Private Key"
	privateKeyDirName    = "private_keys"
)

func PresetPrivateKeyDir(filePath string) (string, error) {
	if filePath == "" {
		return "", fmt.Errorf("private key directory requires a file-backed configuration")
	}
	resolvedPath, err := resolveConfigFilePath(filePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(resolvedPath), privateKeyDirName), nil
}

func ListPresetPrivateKeyFiles(filePath string) ([]string, error) {
	keyDir, err := PresetPrivateKeyDir(filePath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(keyDir)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	files := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, "file://"+filepath.Join(keyDir, entry.Name()))
	}
	return files, nil
}

func MigratePresetPrivateKeysToFiles(
	filePath string,
	presets []Preset,
) ([]Preset, bool, error) {
	if filePath == "" {
		return presets, false, nil
	}
	resolvedPath, err := resolveConfigFilePath(filePath)
	if err != nil {
		return nil, false, err
	}
	doc, err := readCommonInputFileDocument(resolvedPath)
	if err != nil {
		return nil, false, err
	}
	rawByID := presetInputIndexByID(doc.input.Presets)
	keyDir := filepath.Join(filepath.Dir(resolvedPath), privateKeyDirName)
	migrated := make([]Preset, len(presets))
	copy(migrated, presets)
	changed := false
	used := map[string]struct{}{}

	for i, preset := range migrated {
		if preset.Meta["Authentication"] != "Private Key" {
			continue
		}
		keyValue := preset.Meta[PresetMetaPrivateKey]
		if keyValue == "" {
			continue
		}
		rawIndex, rawOK := rawByID[strings.TrimSpace(preset.ID)]
		rawValue := String(keyValue)
		if rawOK && rawIndex < len(doc.input.Presets) {
			candidateRawValue := doc.input.Presets[rawIndex].Meta[PresetMetaPrivateKey]
			if shouldUseRawPrivateKeyValue(candidateRawValue, keyValue) {
				rawValue = candidateRawValue
			}
		}
		if !shouldMigratePrivateKeyRawValue(rawValue) {
			if hasSupportedPrivateKeyReference(rawValue) && keyValue != string(rawValue) {
				if migrated[i].Meta == nil {
					migrated[i].Meta = map[string]string{}
				}
				migrated[i].Meta[PresetMetaPrivateKey] = string(rawValue)
				changed = true
			}
			continue
		}
		keyPath, err := nextPresetPrivateKeyPath(keyDir, preset, used)
		if err != nil {
			return nil, false, err
		}
		if err := writePresetPrivateKeyFile(keyPath, keyValue); err != nil {
			return nil, false, err
		}
		if migrated[i].Meta == nil {
			migrated[i].Meta = map[string]string{}
		}
		fileRef := "file://" + keyPath
		migrated[i].Meta[PresetMetaPrivateKey] = fileRef
		if rawOK && rawIndex < len(doc.input.Presets) {
			doc.input.Presets[rawIndex].Meta[PresetMetaPrivateKey] = String(fileRef)
		}
		changed = true
	}
	if !changed {
		return presets, false, nil
	}
	if err := writeCommonInputFileDocument(resolvedPath, doc); err != nil {
		return nil, false, err
	}
	return migrated, true, nil
}

func shouldUseRawPrivateKeyValue(rawValue String, runtimeValue string) bool {
	if string(rawValue) == runtimeValue {
		return true
	}
	parsedValue, err := rawValue.Parse()
	return err == nil && parsedValue == runtimeValue
}

func hasSupportedPrivateKeyReference(rawValue String) bool {
	value := strings.TrimSpace(string(rawValue))
	schemeIndex := strings.Index(value, "://")
	if schemeIndex < 0 {
		return false
	}
	switch strings.ToLower(value[:schemeIndex]) {
	case "file", "environment":
		return true
	default:
		return false
	}
}

func shouldMigratePrivateKeyRawValue(rawValue String) bool {
	value := strings.TrimSpace(string(rawValue))
	schemeIndex := strings.Index(value, "://")
	if schemeIndex < 0 {
		return value != ""
	}
	switch strings.ToLower(value[:schemeIndex]) {
	case "file", "environment":
		return false
	case "literal":
		return true
	default:
		return value != ""
	}
}

func writePresetPrivateKeyFile(path string, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, writeErr := f.WriteString(value); writeErr != nil {
		f.Close()
		return writeErr
	}
	if syncErr := f.Sync(); syncErr != nil {
		f.Close()
		return syncErr
	}
	if closeErr := f.Close(); closeErr != nil {
		return closeErr
	}
	return os.Chmod(path, 0o600)
}

func nextPresetPrivateKeyPath(
	keyDir string,
	preset Preset,
	used map[string]struct{},
) (string, error) {
	base := slugifyPresetPrivateKeyName(preset.Title)
	if base == "" {
		base = slugifyPresetPrivateKeyName(preset.Host)
	}
	if base == "" {
		base = slugifyPresetPrivateKeyName(preset.ID)
	}
	if base == "" {
		base = "preset"
	}
	for i := 0; ; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s-%d", base, i+1)
		}
		path := filepath.Join(keyDir, name+".key")
		if _, ok := used[path]; ok {
			continue
		}
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			used[path] = struct{}{}
			return path, nil
		}
		if err != nil {
			return "", err
		}
	}
}

func slugifyPresetPrivateKeyName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
