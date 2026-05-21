// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Snuffy2/shellport/application/commands"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
)

func writePresetAPIConfig(t *testing.T, path string, presets []map[string]any) {
	t.Helper()

	data := map[string]any{
		"Servers": []map[string]any{
			{"ListenInterface": "127.0.0.1", "ListenPort": 8182},
		},
		"Presets": presets,
	}
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
}

func newTestPresetConfig(t *testing.T, configPath string) presetConfig {
	t.Helper()

	return newTestPresetConfigPair(t, configPath)[0]
}

func newTestPresetConfigPair(t *testing.T, configPath string) [2]presetConfig {
	t.Helper()

	_, cfg, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	cfg, err = normalizeStartupPresetIDsForTest(cfg)
	if err != nil {
		t.Fatalf("normalizeStartupPresetIDsForTest returned error: %v", err)
	}
	cfg.Presets, err = commands.New().Reconfigure(cfg.Presets)
	if err != nil {
		t.Fatalf("Reconfigure returned error: %v", err)
	}
	commonCfg := cfg.Common()
	cmds := commands.New()
	return [2]presetConfig{
		newPresetConfig(commonCfg, cmds),
		newPresetConfig(commonCfg, cmds),
	}
}

func newAuthenticatedTestPresetConfig(t *testing.T, configPath string) presetConfig {
	t.Helper()

	controller := newTestPresetConfig(t, configPath)
	controller.commonCfg.SharedKey = "test-shared-key"
	return controller
}

func newAdminTestPresetConfig(t *testing.T, configPath string) presetConfig {
	t.Helper()

	controller := newAuthenticatedTestPresetConfig(t, configPath)
	controller.commonCfg.AdminKey = "test-admin-key"
	return controller
}

func authorizePresetConfigRequest(controller presetConfig, request *http.Request) {
	authorizePresetConfigRequestWithKey(
		controller,
		request,
		controller.commonCfg.SharedKey,
	)
}

func authorizeAdminRequest(controller presetConfig, request *http.Request) {
	authorizePresetConfigRequestWithKey(
		controller,
		request,
		controller.commonCfg.AdminKey,
	)
}

func authorizePresetConfigRequestWithKey(
	controller presetConfig,
	request *http.Request,
	key string,
) {
	waitForStableSocketAuthWindow()
	verifier := newSocketVerification(
		socket{commonCfg: controller.commonCfg},
		configuration.Server{},
		controller.commonCfg,
	)
	request.Header.Set(
		"X-Key",
		base64.StdEncoding.EncodeToString(verifier.authKeyForSecret(request, key)),
	)
}

func waitForStableSocketAuthWindow() {
	if time.Now().Unix()%100 < 98 {
		return
	}
	time.Sleep(3 * time.Second)
}

func normalizeStartupPresetIDsForTest(
	cfg configuration.Configuration,
) (configuration.Configuration, error) {
	presets, _, err := configuration.EnsurePresetIDs(cfg.Presets)
	if err != nil {
		return configuration.Configuration{}, err
	}
	cfg.Presets = presets
	return cfg, nil
}

func TestPresetConfigGetReturnsPresetIDs(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{"ID": "preset-atlantis", "Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
	})
	controller := newTestPresetConfig(t, configPath)
	request := httptest.NewRequest(http.MethodGet, "/shellport/config/presets", nil)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Get(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	var response presetConfigResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("json Decode returned error: %v", err)
	}
	if response.Presets[0].ID != "preset-atlantis" {
		t.Fatalf("preset ID = %q, want preset-atlantis", response.Presets[0].ID)
	}
}

func TestPresetConfigGetMarksHiddenSavedPassword(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				configuration.PresetMetaEncryptedPassword: "v1:aes-256-gcm:nonce:ciphertext",
				"Authentication": "Password",
			},
		},
	})
	controller := newTestPresetConfig(t, configPath)
	request := httptest.NewRequest(http.MethodGet, "/shellport/config/presets", nil)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Get(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	var response presetConfigResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("json Decode returned error: %v", err)
	}
	if len(response.Presets) != 1 {
		t.Fatalf("response preset count = %d, want 1", len(response.Presets))
	}
	if !response.Presets[0].HasSavedPassword {
		t.Fatal("HasSavedPassword = false, want true")
	}
	if _, ok := response.Presets[0].Meta[configuration.PresetMetaPassword]; ok {
		t.Fatal("response exposed plaintext Password")
	}
	if _, ok := response.Presets[0].Meta[configuration.PresetMetaEncryptedPassword]; ok {
		t.Fatal("response exposed Encrypted Password")
	}
}

func TestPresetConfigPutAddsMissingIDsAndPersists(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{"ID": "preset-atlantis", "Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
	})
	controller := newAdminTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"title":"Columbia","type":"SSH","host":"columbia.home","meta":{"User":"pi"}}]}`)
	request := httptest.NewRequest(http.MethodPut, "/shellport/config/presets", bytes.NewReader(body))
	authorizePresetConfigRequest(controller, request)
	authorizeAdminRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	var response presetConfigResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("json Decode returned error: %v", err)
	}
	if response.Presets[0].ID == "" {
		t.Fatal("response preset ID is empty")
	}
	if response.Presets[0].Host != "columbia.home:22" {
		t.Fatalf("response host = %q, want columbia.home:22", response.Presets[0].Host)
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	if reloaded.Presets[0].ID != response.Presets[0].ID {
		t.Fatalf("persisted ID = %q, want %q", reloaded.Presets[0].ID, response.Presets[0].ID)
	}
}

func TestPresetConfigPutRemovesSupportedPresetsAndPreservesRawUnsupported(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{"ID": "preset-atlantis", "Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
		{"ID": "preset-future", "Title": "Future", "Type": "Future", "Host": "future.home"},
	})
	controller := newAdminTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-columbia","title":"Columbia","type":"SSH","host":"columbia.home","meta":{"User":"pi"}}]}`)
	request := httptest.NewRequest(http.MethodPut, "/shellport/config/presets", bytes.NewReader(body))
	authorizePresetConfigRequest(controller, request)
	authorizeAdminRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	var raw struct {
		Presets []struct {
			ID string
		}
	}
	f, err := os.Open(configPath)
	if err != nil {
		t.Fatalf("os.Open returned error: %v", err)
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&raw); err != nil {
		t.Fatalf("json Decode returned error: %v", err)
	}
	if len(raw.Presets) != 2 {
		t.Fatalf("raw preset count = %d, want 2", len(raw.Presets))
	}
	if raw.Presets[0].ID != "preset-columbia" {
		t.Fatalf("first raw preset ID = %q, want preset-columbia", raw.Presets[0].ID)
	}
	if raw.Presets[1].ID != "preset-future" {
		t.Fatalf("second raw preset ID = %q, want preset-future", raw.Presets[1].ID)
	}
}

func TestPresetConfigPutRejectsDuplicateIDs(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, nil)
	controller := newAdminTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"dup","title":"A","type":"SSH","host":"a.home"},{"id":"dup","title":"B","type":"SSH","host":"b.home"}]}`)
	request := httptest.NewRequest(http.MethodPut, "/shellport/config/presets", bytes.NewReader(body))
	authorizePresetConfigRequest(controller, request)
	authorizeAdminRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want duplicate ID error")
	}
}

func TestPresetConfigPutAllowsAdminWhenBothKeysAreBlank(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, nil)
	controller := newTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home"}]}`)
	request := httptest.NewRequest(http.MethodPut, "/shellport/config/presets", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
}

func TestPresetConfigPutRequiresAdminKeyForFullReplacement(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{"ID": "preset-atlantis", "Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
	})
	controller := newAdminTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-columbia","title":"Columbia","type":"SSH","host":"columbia.home"}]}`)
	request := httptest.NewRequest(http.MethodPut, "/shellport/config/presets", bytes.NewReader(body))
	authorizePresetConfigRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want admin authentication error")
	}
}

func TestPresetConfigPutPreserveHeaderRequiresAdminKeyWithoutPresetID(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{"ID": "preset-atlantis", "Title": "Atlantis", "Type": "SSH", "Host": "atlantis.home"},
	})
	controller := newAdminTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-columbia","title":"Columbia","type":"SSH","host":"columbia.home","meta":{"User":"pi","Authentication":"Password"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want admin authentication error")
	}
}

func TestPresetConfigPutRejectsClearHiddenPasswordIDsOnFingerprintSave(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User":           "pi",
				"Authentication": "Password",
				"Password":       "mypassword",
			},
		},
	})
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","meta":{"Fingerprint":"SHA256:12345"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	request.Header.Set(presetFingerprintIDHeader, "preset-atlantis")
	request.Header.Set(clearHiddenPresetPasswordsHeader, "preset-atlantis")
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want clear-header fingerprint save rejection")
	}

	if _, ok := controller.commonCfg.CurrentPresets()[0].Meta[configuration.PresetMetaPassword]; !ok {
		t.Fatal("live preset lost password after rejected fingerprint save")
	}

	_, reloaded, customErr := configuration.CustomFile(configPath)(log.Ditch{})
	if customErr != nil {
		t.Fatalf("CustomFile returned error: %v", customErr)
	}
	if _, ok := reloaded.Presets[0].Meta[configuration.PresetMetaPassword]; !ok {
		t.Fatal("persisted preset lost password after rejected fingerprint save")
	}
}

func TestPresetConfigPutAllowsSharedKeyAdminWhenAdminKeyIsBlank(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, nil)
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-columbia","title":"Columbia","type":"SSH","host":"columbia.home"}]}`)
	request := httptest.NewRequest(http.MethodPut, "/shellport/config/presets", bytes.NewReader(body))
	authorizePresetConfigRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
}

func TestPresetConfigPutAllowsAnonymousUserButNotAdminWhenSharedKeyBlank(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, nil)
	controller := newTestPresetConfig(t, configPath)
	controller.commonCfg.AdminKey = "test-admin-key"
	body := []byte(`{"presets":[{"id":"preset-columbia","title":"Columbia","type":"SSH","host":"columbia.home"}]}`)
	request := httptest.NewRequest(http.MethodPut, "/shellport/config/presets", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want admin authentication error")
	}

	request = httptest.NewRequest(http.MethodPut, "/shellport/config/presets", bytes.NewReader(body))
	authorizeAdminRequest(controller, request)
	recorder = httptest.NewRecorder()
	writer = newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put with AdminKey returned error: %v", err)
	}
}

func TestPresetConfigPutRejectsIDsDuplicatedAfterTrimming(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, nil)
	controller := newAdminTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":" dup ","title":"A","type":"SSH","host":"a.home"},{"id":"dup","title":"B","type":"SSH","host":"b.home"}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	authorizeAdminRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want duplicate ID error")
	}
}

func TestPresetConfigPutEncryptsPlaintextPasswordsWhenKeyIsSet(t *testing.T) {
	t.Setenv(
		configuration.PresetSecretKeyEnv,
		base64.StdEncoding.EncodeToString(
			[]byte("0123456789abcdef0123456789abcdef"),
		),
	)
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, nil)
	controller := newAdminTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home","meta":{"User":"pi","Authentication":"Password","Password":"mypassword"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	authorizeAdminRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	var response presetConfigResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("json Decode returned error: %v", err)
	}
	if _, ok := response.Presets[0].Meta[configuration.PresetMetaPassword]; ok {
		t.Fatal("response still contains plaintext Password")
	}
	if _, ok := response.Presets[0].Meta[configuration.PresetMetaEncryptedPassword]; ok {
		t.Fatal("response exposed Encrypted Password")
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	if _, ok := reloaded.Presets[0].Meta[configuration.PresetMetaPassword]; ok {
		t.Fatal("persisted config still contains plaintext Password")
	}
	if reloaded.Presets[0].Meta[configuration.PresetMetaEncryptedPassword] == "" {
		t.Fatal("persisted config missing Encrypted Password")
	}
}

func TestPresetConfigPutPreservesHiddenPasswordOnFingerprintSave(t *testing.T) {
	t.Setenv(
		configuration.PresetSecretKeyEnv,
		base64.StdEncoding.EncodeToString(
			[]byte("0123456789abcdef0123456789abcdef"),
		),
	)
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User":           "pi",
				"Authentication": "Password",
				"Password":       "mypassword",
			},
		},
	})
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home:22","meta":{"User":"pi","Authentication":"Password","Fingerprint":"SHA256:abc"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	request.Header.Set(presetFingerprintIDHeader, "preset-atlantis")
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	live := controller.commonCfg.CurrentPresets()
	if live[0].SecretMeta[configuration.PresetMetaPassword] != "mypassword" {
		t.Fatal("live preset lost hidden password")
	}
	if live[0].Meta["Fingerprint"] != "SHA256:abc" {
		t.Fatal("live preset missing fingerprint")
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	if _, ok := reloaded.Presets[0].Meta[configuration.PresetMetaPassword]; ok {
		t.Fatal("persisted config still contains plaintext Password")
	}
	if reloaded.Presets[0].Meta[configuration.PresetMetaEncryptedPassword] == "" {
		t.Fatal("persisted config missing Encrypted Password")
	}
	if reloaded.Presets[0].Meta["Fingerprint"] != "SHA256:abc" {
		t.Fatal("persisted config missing fingerprint")
	}
}

func TestPresetConfigPutPreservesHiddenPasswordOnFullAdminReplacement(t *testing.T) {
	t.Setenv(
		configuration.PresetSecretKeyEnv,
		base64.StdEncoding.EncodeToString(
			[]byte("0123456789abcdef0123456789abcdef"),
		),
	)
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User":           "pi",
				"Authentication": "Password",
				"Password":       "mypassword",
			},
		},
	})
	controller := newAdminTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis Edited","type":"SSH","host":"atlantis.home:22","meta":{"User":"pi","Authentication":"Password"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizeAdminRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	live := controller.commonCfg.CurrentPresets()
	if live[0].SecretMeta[configuration.PresetMetaPassword] != "mypassword" {
		t.Fatal("live preset lost hidden password")
	}
	if live[0].Title != "Atlantis Edited" {
		t.Fatalf("live title = %q, want Atlantis Edited", live[0].Title)
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	if reloaded.Presets[0].Meta[configuration.PresetMetaEncryptedPassword] == "" {
		t.Fatal("persisted config missing Encrypted Password")
	}
}

func TestPresetConfigPutCanClearOneHiddenPasswordWhilePreservingOthers(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User":           "pi",
				"Authentication": "Password",
				"Password":       "mypassword-atlantis",
			},
		},
		{
			"ID":    "preset-columbia",
			"Title": "Columbia",
			"Type":  "SSH",
			"Host":  "columbia.home",
			"Meta": map[string]string{
				"User":           "pi",
				"Authentication": "Password",
				"Password":       "mypassword-columbia",
			},
		},
	})
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home:22","meta":{"User":"pi","Authentication":"Password"}},{"id":"preset-columbia","title":"Columbia","type":"SSH","host":"columbia.home:22","meta":{"User":"pi","Authentication":"Password"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	request.Header.Set(clearHiddenPresetPasswordsHeader, "preset-atlantis")
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	live := controller.commonCfg.CurrentPresets()
	liveByID := make(map[string]configuration.Preset, len(live))
	for _, preset := range live {
		liveByID[preset.ID] = preset
	}

	if _, ok := liveByID["preset-atlantis"].SecretMeta[configuration.PresetMetaPassword]; ok {
		t.Fatal("preset-atlantis still contains hidden Password after clear header")
	}
	if _, ok := liveByID["preset-atlantis"].Meta[configuration.PresetMetaPassword]; ok {
		t.Fatal("preset-atlantis still contains plaintext Password after clear header")
	}
	if _, ok := liveByID["preset-atlantis"].Meta[configuration.PresetMetaEncryptedPassword]; ok {
		t.Fatal("preset-atlantis still contains Encrypted Password after clear header")
	}
	if _, ok := liveByID["preset-columbia"].SecretMeta[configuration.PresetMetaPassword]; !ok {
		t.Fatal("preset-columbia lost hidden Password when clear-header targeted another preset")
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	reloadedByID := make(map[string]configuration.Preset, len(reloaded.Presets))
	for _, preset := range reloaded.Presets {
		reloadedByID[preset.ID] = preset
	}
	if _, ok := reloadedByID["preset-atlantis"].Meta[configuration.PresetMetaPassword]; ok {
		t.Fatal("persisted preset-atlantis contains plaintext Password after clear header")
	}
	if _, ok := reloadedByID["preset-atlantis"].Meta[configuration.PresetMetaEncryptedPassword]; ok {
		t.Fatal("persisted preset-atlantis still contains Encrypted Password after clear header")
	}
	if _, ok := reloadedByID["preset-columbia"].Meta[configuration.PresetMetaPassword]; !ok {
		t.Fatal("persisted preset-columbia lost plaintext Password when another preset was cleared")
	}
}

func TestPresetConfigPutAcceptsCompactFingerprintSaveWithLargeMeta(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	largePrivateKey := strings.Repeat("k", maxPresetConfigStringBytes+1)
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User":           "pi",
				"Authentication": "Private Key",
				"Private Key":    largePrivateKey,
			},
		},
	})
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","meta":{"Fingerprint":"SHA256:abc"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	request.Header.Set(presetFingerprintIDHeader, "preset-atlantis")
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	if reloaded.Presets[0].Meta["Private Key"] != largePrivateKey {
		t.Fatal("persisted config lost large private key")
	}
	if reloaded.Presets[0].Meta["Fingerprint"] != "SHA256:abc" {
		t.Fatal("persisted config missing fingerprint")
	}
}

func TestPresetConfigPutCompactFingerprintSavePreservesPrivateKeyFileReference(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "shellport.conf.json")
	keyPath := filepath.Join(tempDir, "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY DATA"), 0o600); err != nil {
		t.Fatalf("os.WriteFile key returned error: %v", err)
	}
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User":           "pi",
				"Authentication": "Private Key",
				"Private Key":    "file://" + keyPath,
			},
		},
	})
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","meta":{"Fingerprint":"SHA256:abc"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	request.Header.Set(presetFingerprintIDHeader, "preset-atlantis")
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	var raw struct {
		Presets []struct {
			Meta map[string]configuration.String
		}
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if raw.Presets[0].Meta["Private Key"] != configuration.String("file://"+keyPath) {
		t.Fatalf(
			"raw private key = %q, want file URI",
			raw.Presets[0].Meta["Private Key"],
		)
	}
	if raw.Presets[0].Meta["Fingerprint"] != "SHA256:abc" {
		t.Fatal("raw fingerprint was not updated")
	}
}

func TestPresetConfigPutFullUpdatePreservesRotatedPrivateKeyFileReference(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "shellport.conf.json")
	keyPath := filepath.Join(tempDir, "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY DATA"), 0o600); err != nil {
		t.Fatalf("os.WriteFile key returned error: %v", err)
	}
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User":           "pi",
				"Authentication": "Private Key",
				"Private Key":    "file://" + keyPath,
			},
		},
	})
	controller := newAdminTestPresetConfig(t, configPath)
	if err := os.WriteFile(keyPath, []byte("UPDATED PRIVATE KEY DATA"), 0o600); err != nil {
		t.Fatalf("os.WriteFile rotated key returned error: %v", err)
	}
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home:22","meta":{"User":"pi","Authentication":"Private Key","Private Key":"UPDATED PRIVATE KEY DATA","Fingerprint":"SHA256:abc"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizeAdminRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	var raw struct {
		Presets []struct {
			Meta map[string]configuration.String
		}
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if raw.Presets[0].Meta["Private Key"] != configuration.String("file://"+keyPath) {
		t.Fatalf(
			"raw private key = %q, want file URI",
			raw.Presets[0].Meta["Private Key"],
		)
	}
	if raw.Presets[0].Meta["Fingerprint"] != "SHA256:abc" {
		t.Fatal("raw fingerprint was not updated")
	}
}

func TestPresetConfigPutAcceptsCompactFingerprintSaveForLargePresetList(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	rawPresets := make([]map[string]any, maxPresetConfigPresets+1)
	for i := range rawPresets {
		rawPresets[i] = map[string]any{
			"ID":    fmt.Sprintf("preset-%03d", i),
			"Title": fmt.Sprintf("Preset %03d", i),
			"Type":  "SSH",
			"Host":  fmt.Sprintf("host-%03d.home", i),
		}
	}
	writePresetAPIConfig(t, configPath, rawPresets)
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-000","meta":{"Fingerprint":"SHA256:abc"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	request.Header.Set(presetFingerprintIDHeader, "preset-000")
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	if len(reloaded.Presets) != maxPresetConfigPresets+1 {
		t.Fatalf("persisted preset count = %d, want %d", len(reloaded.Presets), maxPresetConfigPresets+1)
	}
	if reloaded.Presets[0].Meta["Fingerprint"] != "SHA256:abc" {
		t.Fatal("persisted config missing fingerprint")
	}
}

func TestPresetConfigPutRejectsFingerprintSaveChangingHost(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User": "pi",
			},
		},
	})
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"evil.home:22","meta":{"User":"pi","Fingerprint":"SHA256:abc"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	request.Header.Set(presetFingerprintIDHeader, "preset-atlantis")
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want fingerprint-only validation error")
	}
}

func TestPresetConfigPutRejectsFingerprintSaveChangingMultipleFingerprints(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User":        "pi",
				"Fingerprint": "SHA256:old-atlantis",
			},
		},
		{
			"ID":    "preset-columbia",
			"Title": "Columbia",
			"Type":  "SSH",
			"Host":  "columbia.home",
			"Meta": map[string]string{
				"User":        "pi",
				"Fingerprint": "SHA256:old-columbia",
			},
		},
	})
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home:22","meta":{"User":"pi","Fingerprint":"SHA256:new-atlantis"}},{"id":"preset-columbia","title":"Columbia","type":"SSH","host":"columbia.home:22","meta":{"User":"pi","Fingerprint":"SHA256:new-columbia"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	request.Header.Set(presetFingerprintIDHeader, "preset-atlantis")
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want multi-fingerprint validation error")
	}
}

func TestPresetConfigPutRejectsStaleFingerprintSaveDeletingAnotherFingerprint(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User": "pi",
			},
		},
		{
			"ID":    "preset-columbia",
			"Title": "Columbia",
			"Type":  "SSH",
			"Host":  "columbia.home",
			"Meta": map[string]string{
				"User":        "pi",
				"Fingerprint": "SHA256:current-columbia",
			},
		},
	})
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home:22","meta":{"User":"pi","Fingerprint":"SHA256:new-atlantis"}},{"id":"preset-columbia","title":"Columbia","type":"SSH","host":"columbia.home:22","meta":{"User":"pi"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	request.Header.Set(presetFingerprintIDHeader, "preset-atlantis")
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want stale fingerprint validation error")
	}
}

func TestSamePresetMetaExceptFingerprintRequiresMatchingKeyPresence(t *testing.T) {
	tests := []struct {
		name    string
		next    map[string]string
		current map[string]string
	}{
		{
			name:    "reject add empty metadata",
			next:    map[string]string{"User": "pi", "Comment": ""},
			current: map[string]string{"User": "pi"},
		},
		{
			name:    "reject remove empty metadata",
			next:    map[string]string{"User": "pi"},
			current: map[string]string{"User": "pi", "Comment": ""},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if samePresetMetaExceptFingerprint(test.next, test.current) {
				t.Fatal("samePresetMetaExceptFingerprint returned true, want false")
			}
		})
	}
}

func TestPresetConfigPutSerializesConcurrentFingerprintSaves(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User": "pi",
			},
		},
		{
			"ID":    "preset-columbia",
			"Title": "Columbia",
			"Type":  "SSH",
			"Host":  "columbia.home",
			"Meta": map[string]string{
				"User": "pi",
			},
		},
	})
	controllers := newTestPresetConfigPair(t, configPath)
	controllers[0].commonCfg.SharedKey = "test-shared-key"
	controllers[1].commonCfg.SharedKey = "test-shared-key"
	bodies := [][]byte{
		[]byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home:22","meta":{"User":"pi","Fingerprint":"SHA256:atlantis"}},{"id":"preset-columbia","title":"Columbia","type":"SSH","host":"columbia.home:22","meta":{"User":"pi"}}]}`),
		[]byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home:22","meta":{"User":"pi"}},{"id":"preset-columbia","title":"Columbia","type":"SSH","host":"columbia.home:22","meta":{"User":"pi","Fingerprint":"SHA256:columbia"}}]}`),
	}
	targetIDs := []string{"preset-atlantis", "preset-columbia"}
	errs := make([]error, len(bodies))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range bodies {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			request := httptest.NewRequest(
				http.MethodPut,
				"/shellport/config/presets",
				bytes.NewReader(bodies[i]),
			)
			controller := controllers[i]
			authorizePresetConfigRequest(controller, request)
			request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
			request.Header.Set(presetFingerprintIDHeader, targetIDs[i])
			recorder := httptest.NewRecorder()
			writer := newResponseWriter(recorder)

			<-start
			errs[i] = controller.Put(&writer, request, log.Ditch{})
		}(i)
	}
	close(start)
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent saves = %d, want 1; errs = %v", successes, errs)
	}
}

func TestPresetConfigPutRejectsOversizedRequest(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, nil)
	controller := newAdminTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"` +
		strings.Repeat("a", maxPresetConfigRequestBytes) +
		`","type":"SSH","host":"atlantis.home"}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	authorizeAdminRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want oversized body error")
	}
}

func TestPresetConfigPutRejectsOversizedPresetID(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, nil)
	controller := newAdminTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"` +
		strings.Repeat("a", configuration.MaxPresetIDLength+1) +
		`","title":"Atlantis","type":"SSH","host":"atlantis.home"}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	authorizeAdminRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want oversized preset ID error")
	}
}

func TestPresetConfigPutRejectsInvalidFingerprintFormat(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User": "pi",
			},
		},
	})
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home:22","meta":{"User":"pi","Fingerprint":"not-a-fingerprint"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	request.Header.Set(presetFingerprintIDHeader, "preset-atlantis")
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want invalid fingerprint error")
	}
}

func TestPresetConfigPutRejectsOversizedFingerprint(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User": "pi",
			},
		},
	})
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home:22","meta":{"User":"pi","Fingerprint":"SHA256:` + strings.Repeat("a", maxPresetFingerprintBytes) + `"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	request.Header.Set(presetFingerprintIDHeader, "preset-atlantis")
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	err := controller.Put(&writer, request, log.Ditch{})
	if err == nil {
		t.Fatal("Put returned nil error, want oversized fingerprint error")
	}
}

func TestPresetConfigPutCanDeleteHiddenPassword(t *testing.T) {
	t.Setenv(
		configuration.PresetSecretKeyEnv,
		base64.StdEncoding.EncodeToString(
			[]byte("0123456789abcdef0123456789abcdef"),
		),
	)
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]string{
				"User":           "pi",
				"Authentication": "Password",
				"Password":       "mypassword",
			},
		},
	})
	controller := newAdminTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home:22","meta":{"User":"pi","Authentication":"None"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	authorizeAdminRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	live := controller.commonCfg.CurrentPresets()
	if _, ok := live[0].Meta[configuration.PresetMetaEncryptedPassword]; ok {
		t.Fatal("live preset still contains Encrypted Password")
	}
	if _, ok := live[0].SecretMeta[configuration.PresetMetaPassword]; ok {
		t.Fatal("live preset still contains hidden Password")
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	if _, ok := reloaded.Presets[0].Meta[configuration.PresetMetaEncryptedPassword]; ok {
		t.Fatal("persisted config still contains Encrypted Password")
	}
	if _, ok := reloaded.Presets[0].Meta[configuration.PresetMetaPassword]; ok {
		t.Fatal("persisted config still contains Password")
	}
}

func TestPresetConfigPutPreservesPlaintextPasswordWhenEncryptedAlsoPresentWithoutKey(t *testing.T) {
	t.Setenv(configuration.PresetSecretKeyEnv, "")
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, nil)
	controller := newAdminTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-atlantis","title":"Atlantis","type":"SSH","host":"atlantis.home","meta":{"User":"pi","Authentication":"Password","Password":"mypassword","Encrypted Password":"v1:aes-256-gcm:nonce:ciphertext"}}]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/shellport/config/presets",
		bytes.NewReader(body),
	)
	authorizePresetConfigRequest(controller, request)
	authorizeAdminRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}
	if reloaded.Presets[0].Meta[configuration.PresetMetaPassword] != "mypassword" {
		t.Fatal("persisted config missing plaintext Password")
	}
	if _, ok := reloaded.Presets[0].Meta[configuration.PresetMetaEncryptedPassword]; ok {
		t.Fatal("persisted config kept Encrypted Password without key")
	}
}

func TestSocketAccessConfigurationIncludesPresetConfigWritable(t *testing.T) {
	accessConfig := newSocketAccessConfiguration(nil, "", "", true)

	if !accessConfig.PresetConfigWritable {
		t.Fatal("PresetConfigWritable = false, want true")
	}
}

func TestSocketAccessConfigurationIncludesServerTitle(t *testing.T) {
	accessConfig := newSocketAccessConfiguration(nil, "Homelab Shells", "", false)

	if accessConfig.ServerTitle != "Homelab Shells" {
		t.Fatalf("ServerTitle = %q, want Homelab Shells", accessConfig.ServerTitle)
	}
}

func TestSocketAccessConfigurationLeavesServerTitleUnescaped(t *testing.T) {
	accessConfig := newSocketAccessConfiguration(nil, "Ops & Lab <Prod>", "", false)

	if accessConfig.ServerTitle != "Ops & Lab <Prod>" {
		t.Fatalf(
			"ServerTitle = %q, want Ops & Lab <Prod>",
			accessConfig.ServerTitle,
		)
	}
}

func TestSocketVerificationAdvertisesPresetConfigWritableWhenConfigIsWritable(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, nil)

	verification := newSocketVerification(
		socket{},
		configuration.Server{},
		configuration.Common{SourceFile: configPath},
	)
	var accessConfig socketAccessConfiguration
	if err := json.Unmarshal(verification.configRspBody, &accessConfig); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if !accessConfig.PresetConfigWritable {
		t.Fatal("PresetConfigWritable = false, want true")
	}
}

func TestSocketVerificationTreatsBlankSharedKeyAsAnonymousUser(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, nil)
	verification := newSocketVerification(
		socket{commonCfg: configuration.Common{
			SourceFile: configPath,
			AdminKey:   "test-admin-key",
		}},
		configuration.Server{},
		configuration.Common{
			SourceFile: configPath,
			AdminKey:   "test-admin-key",
		},
	)
	request := httptest.NewRequest(http.MethodGet, "/shellport/socket/verify", nil)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := verification.Get(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	var accessConfig socketAccessConfiguration
	if err := json.Unmarshal(recorder.Body.Bytes(), &accessConfig); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if !accessConfig.PresetConfigWritable {
		t.Fatal("PresetConfigWritable = false, want true")
	}
}

func TestSocketVerificationRejectsAdminKeyForSocketBootstrap(t *testing.T) {
	commonCfg := configuration.Common{
		SharedKey: "test-shared-key",
		AdminKey:  "test-admin-key",
	}
	verification := newSocketVerification(
		socket{commonCfg: commonCfg},
		configuration.Server{},
		commonCfg,
	)
	request := httptest.NewRequest(http.MethodGet, "/shellport/socket/verify", nil)
	request.Header.Set(
		"X-Key",
		base64.StdEncoding.EncodeToString(
			verification.authKeyForSecret(request, commonCfg.AdminKey),
		),
	)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := verification.Get(&writer, request, log.Ditch{}); err == nil {
		t.Fatal("Get returned nil error, want admin key rejection")
	}
}

func TestSocketAccessConfigurationDoesNotExposePlaintextPasswords(t *testing.T) {
	accessConfig := newSocketAccessConfiguration([]configuration.Preset{
		{
			Title: "Atlantis",
			Type:  "SSH",
			Host:  "atlantis.home:22",
			Meta: map[string]string{
				configuration.PresetMetaPassword:          "mypassword",
				configuration.PresetMetaEncryptedPassword: "encrypted",
			},
		},
	}, "", "", true)

	if _, ok := accessConfig.Presets[0].Meta[configuration.PresetMetaPassword]; ok {
		t.Fatal("socket access configuration exposed plaintext Password")
	}
	if _, ok := accessConfig.Presets[0].Meta[configuration.PresetMetaEncryptedPassword]; ok {
		t.Fatal("socket access configuration exposed Encrypted Password")
	}
}
