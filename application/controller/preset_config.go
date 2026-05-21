// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
)

const (
	preserveHiddenPresetPasswordsHeader = "X-Preserve-Hidden-Preset-Passwords"
	clearHiddenPresetPasswordsHeader    = "X-Clear-Hidden-Preset-Passwords"
	presetFingerprintIDHeader           = "X-Preset-Fingerprint-ID"
	maxPresetConfigRequestBytes         = 256 * 1024
	maxPresetConfigPresets              = 512
	maxPresetConfigStringBytes          = 4096
	maxPresetFingerprintBytes           = 256
)

// presetConfig handles backend preset configuration reads and writes.
type presetConfig struct {
	baseController

	commonCfg configuration.Common
	commands  command.Commands
}

// presetConfigResponse is the JSON envelope returned by preset config APIs.
type presetConfigResponse struct {
	Presets []socketRemotePreset `json:"presets"`
}

// presetConfigRequest is the JSON envelope accepted by preset config APIs.
type presetConfigRequest struct {
	Presets []socketRemotePreset `json:"presets"`
}

// newPresetConfig builds the backend-only preset configuration controller.
func newPresetConfig(
	commonCfg configuration.Common,
	commands command.Commands,
) presetConfig {
	return presetConfig{
		commonCfg: commonCfg,
		commands:  commands,
	}
}

// Get returns the current live preset list.
func (p presetConfig) Get(
	w *ResponseWriter,
	r *http.Request,
	l log.Logger,
) error {
	if _, err := p.requireAuth(r); err != nil {
		return err
	}
	return p.writePresets(w, p.commonCfg.CurrentPresets())
}

// Put replaces the full preset list, adding missing IDs and persisting the file.
func (p presetConfig) Put(
	w *ResponseWriter,
	r *http.Request,
	l log.Logger,
) error {
	role, err := p.requireAuth(r)
	if err != nil {
		return err
	}
	targetPresetID := strings.TrimSpace(r.Header.Get(presetFingerprintIDHeader))
	if !p.commonCfg.PresetConfigWritable() {
		return NewError(
			http.StatusConflict,
			"Preset updates require a writable file-backed configuration",
		)
	}

	var request presetConfigRequest
	if err := json.NewDecoder(http.MaxBytesReader(
		w,
		r.Body,
		maxPresetConfigRequestBytes,
	)).Decode(&request); err != nil {
		return NewError(http.StatusBadRequest, err.Error())
	}

	p.lockPresetUpdates()
	defer p.unlockPresetUpdates()

	currentPresets := p.commonCfg.CurrentPresets()
	clearPresetIDs := parsePresetIDSet(r.Header.Get(clearHiddenPresetPasswordsHeader))
	fingerprintOnly := r.Header.Get(preserveHiddenPresetPasswordsHeader) == "yes" && targetPresetID != ""
	var presets []configuration.Preset
	clearPresetIDsForPersistence := clearPresetIDs
	if fingerprintOnly {
		if len(clearPresetIDs) > 0 {
			return NewError(
				http.StatusBadRequest,
				"cannot clear hidden passwords on fingerprint-only preset save",
			)
		}
		clearPresetIDsForPersistence = nil
		var err error
		if isCompactFingerprintRequest(request, targetPresetID) {
			presets, err = applyCompactFingerprintUpdate(
				currentPresets,
				targetPresetID,
				request.Presets[0].Meta["Fingerprint"],
			)
			if err != nil {
				return NewError(http.StatusBadRequest, err.Error())
			}
		} else {
			if err := validateFingerprintConfigRequest(request); err != nil {
				return NewError(http.StatusBadRequest, err.Error())
			}
			presets = presetConfigRequestPresets(request)
			presets = preserveHiddenPresetPasswordsExcept(
				presets,
				currentPresets,
				clearPresetIDs,
			)
		}
		presets, err = p.commands.Reconfigure(presets)
		if err != nil {
			return NewError(http.StatusBadRequest, err.Error())
		}
		if targetPresetID != "" {
			if err := validateFingerprintOnlyPresetUpdate(
				presets,
				currentPresets,
				targetPresetID,
			); err != nil {
				return NewError(http.StatusBadRequest, err.Error())
			}
		}
	} else if role < authRoleAdmin {
		return NewError(
			http.StatusForbidden,
			"Full preset updates require AdminKey authentication",
		)
	} else {
		if err := validatePresetConfigRequest(request); err != nil {
			return NewError(http.StatusBadRequest, err.Error())
		}
		presets = presetConfigRequestPresets(request)
		presets = preserveHiddenPresetPasswordsExcept(
			presets,
			currentPresets,
			clearPresetIDs,
		)
	}
	normalized, _, err := configuration.EnsurePresetIDs(presets)
	if err != nil {
		return NewError(http.StatusBadRequest, err.Error())
	}
	normalized, _, err = configuration.ApplyPresetSecrets(normalized)
	if err != nil {
		return NewError(http.StatusBadRequest, err.Error())
	}
	normalized, err = p.commands.Reconfigure(normalized)
	if err != nil {
		return NewError(http.StatusBadRequest, err.Error())
	}
	if err := configuration.ReplaceFilePresetsWithRuntime(
		p.commonCfg.SourceFile,
		normalized,
		currentPresets,
		clearPresetIDsForPersistence,
	); err != nil {
		return NewError(http.StatusInternalServerError, err.Error())
	}
	if p.commonCfg.PresetRepository != nil {
		p.commonCfg.PresetRepository.Replace(normalized)
	}
	return p.writePresets(w, normalized)
}

func (p presetConfig) lockPresetUpdates() {
	if p.commonCfg.PresetRepository != nil {
		p.commonCfg.PresetRepository.LockUpdates()
	}
}

func (p presetConfig) unlockPresetUpdates() {
	if p.commonCfg.PresetRepository != nil {
		p.commonCfg.PresetRepository.UnlockUpdates()
	}
}

func presetConfigRequestPresets(
	request presetConfigRequest,
) []configuration.Preset {
	presets := make([]configuration.Preset, len(request.Presets))
	for i, preset := range request.Presets {
		presets[i] = configuration.Preset{
			ID:       strings.TrimSpace(preset.ID),
			Title:    preset.Title,
			Type:     preset.Type,
			Host:     preset.Host,
			TabColor: preset.TabColor,
			Meta:     preset.Meta,
		}
	}
	return presets
}

func validatePresetConfigRequest(request presetConfigRequest) error {
	if len(request.Presets) > maxPresetConfigPresets {
		return fmt.Errorf(
			"preset count %d exceeds maximum %d",
			len(request.Presets),
			maxPresetConfigPresets,
		)
	}
	for _, preset := range request.Presets {
		if stringTooLong(preset.ID, configuration.MaxPresetIDLength) {
			return fmt.Errorf("preset ID exceeds maximum length")
		}
		if stringTooLong(preset.Title, maxPresetConfigStringBytes) ||
			stringTooLong(preset.Type, maxPresetConfigStringBytes) ||
			stringTooLong(preset.Host, maxPresetConfigStringBytes) ||
			stringTooLong(preset.TabColor, maxPresetConfigStringBytes) {
			return fmt.Errorf("preset fields exceed maximum length")
		}
		for key, value := range preset.Meta {
			if stringTooLong(key, maxPresetConfigStringBytes) ||
				stringTooLong(value, maxPresetConfigStringBytes) {
				return fmt.Errorf("preset metadata exceeds maximum length")
			}
		}
	}
	return nil
}

func validateFingerprintConfigRequest(request presetConfigRequest) error {
	if len(request.Presets) > maxPresetConfigPresets {
		return fmt.Errorf(
			"preset count %d exceeds maximum %d",
			len(request.Presets),
			maxPresetConfigPresets,
		)
	}
	for _, preset := range request.Presets {
		if stringTooLong(preset.ID, configuration.MaxPresetIDLength) {
			return fmt.Errorf("preset ID exceeds maximum length")
		}
	}
	return nil
}

func isCompactFingerprintRequest(
	request presetConfigRequest,
	targetPresetID string,
) bool {
	if len(request.Presets) != 1 || targetPresetID == "" {
		return false
	}
	preset := request.Presets[0]
	if strings.TrimSpace(preset.ID) != targetPresetID {
		return false
	}
	if preset.Title != "" ||
		preset.Type != "" ||
		preset.Host != "" ||
		preset.TabColor != "" {
		return false
	}
	_, hasFingerprint := preset.Meta["Fingerprint"]
	return hasFingerprint && len(preset.Meta) == 1
}

func applyCompactFingerprintUpdate(
	current []configuration.Preset,
	targetPresetID string,
	fingerprint string,
) ([]configuration.Preset, error) {
	presets := make([]configuration.Preset, len(current))
	found := false
	for i, preset := range current {
		presets[i] = copyPreset(preset)
		if preset.ID != targetPresetID {
			continue
		}
		if presets[i].Meta == nil {
			presets[i].Meta = map[string]string{}
		}
		presets[i].Meta["Fingerprint"] = fingerprint
		found = true
	}
	if !found {
		return nil, fmt.Errorf("fingerprint save cannot add preset %q", targetPresetID)
	}
	return presets, nil
}

func copyPreset(preset configuration.Preset) configuration.Preset {
	copied := preset
	copied.Meta = copyPresetMeta(preset.Meta)
	if preset.SecretMeta != nil {
		copied.SecretMeta = make(map[string]string, len(preset.SecretMeta))
		for key, value := range preset.SecretMeta {
			copied.SecretMeta[key] = value
		}
	}
	return copied
}

func stringTooLong(value string, maxBytes int) bool {
	return len(value) > maxBytes
}

func validateFingerprintOnlyPresetUpdate(
	presets []configuration.Preset,
	current []configuration.Preset,
	targetPresetID string,
) error {
	if targetPresetID == "" {
		return fmt.Errorf("fingerprint save requires a preset ID")
	}
	if len(presets) != len(current) {
		return fmt.Errorf("fingerprint save cannot add or remove presets")
	}
	currentByID := make(map[string]configuration.Preset, len(current))
	for _, preset := range current {
		currentByID[preset.ID] = preset
	}
	changedFingerprint := false
	for _, preset := range presets {
		currentPreset, ok := currentByID[preset.ID]
		if !ok {
			return fmt.Errorf("fingerprint save cannot add preset %q", preset.ID)
		}
		if preset.Title != currentPreset.Title ||
			preset.Type != currentPreset.Type ||
			preset.Host != currentPreset.Host ||
			preset.TabColor != currentPreset.TabColor {
			return fmt.Errorf("fingerprint save cannot change preset %q", preset.ID)
		}
		if !samePresetMetaExceptFingerprint(preset.Meta, currentPreset.Meta) {
			return fmt.Errorf(
				"fingerprint save cannot change non-fingerprint metadata for preset %q",
				preset.ID,
			)
		}
		if preset.Meta["Fingerprint"] == currentPreset.Meta["Fingerprint"] {
			continue
		}
		if preset.ID != targetPresetID {
			return fmt.Errorf("fingerprint save cannot change preset %q", preset.ID)
		}
		if err := validatePresetFingerprint(
			preset.Meta["Fingerprint"],
		); err != nil {
			return err
		}
		if changedFingerprint {
			return fmt.Errorf("fingerprint save can only change one preset")
		}
		changedFingerprint = true
	}
	if !changedFingerprint {
		return fmt.Errorf("fingerprint save did not change preset %q", targetPresetID)
	}
	return nil
}

func validatePresetFingerprint(fingerprint string) error {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return fmt.Errorf("fingerprint save cannot remove a fingerprint")
	}
	if len(fingerprint) > maxPresetFingerprintBytes {
		return fmt.Errorf("fingerprint exceeds maximum length")
	}
	if !strings.Contains(fingerprint, ":") {
		return fmt.Errorf("fingerprint has invalid format")
	}
	for _, r := range fingerprint {
		if r < 0x21 || r == 0x7f {
			return fmt.Errorf("fingerprint has invalid format")
		}
	}
	return nil
}

func samePresetMetaExceptFingerprint(
	next map[string]string,
	current map[string]string,
) bool {
	for key, value := range next {
		if key == "Fingerprint" {
			continue
		}
		if currentValue, ok := current[key]; !ok || currentValue != value {
			return false
		}
	}
	for key, value := range current {
		if key == "Fingerprint" {
			continue
		}
		if nextValue, ok := next[key]; !ok || nextValue != value {
			return false
		}
	}
	return true
}

func parsePresetIDSet(raw string) map[string]struct{} {
	idSet := make(map[string]struct{})
	for _, presetID := range strings.Split(raw, ",") {
		presetID = strings.TrimSpace(presetID)
		if presetID == "" {
			continue
		}
		idSet[presetID] = struct{}{}
	}
	return idSet
}

func preserveHiddenPresetPasswords(
	presets []configuration.Preset,
	current []configuration.Preset,
) []configuration.Preset {
	return preserveHiddenPresetPasswordsExcept(presets, current, nil)
}

func preserveHiddenPresetPasswordsExcept(
	presets []configuration.Preset,
	current []configuration.Preset,
	clearPresetIDs map[string]struct{},
) []configuration.Preset {
	currentByID := make(map[string]configuration.Preset, len(current))
	for _, preset := range current {
		currentByID[preset.ID] = preset
	}
	merged := make([]configuration.Preset, len(presets))
	for i, preset := range presets {
		merged[i] = preset
		currentPreset, ok := currentByID[preset.ID]
		if !ok {
			continue
		}
		if preset.Meta["Authentication"] != "Password" ||
			currentPreset.Meta["Authentication"] != "Password" {
			continue
		}
		if _, clear := clearPresetIDs[preset.ID]; clear {
			continue
		}
		if hasPresetPasswordMeta(preset.Meta) {
			continue
		}
		merged[i].Meta = copyPresetMeta(preset.Meta)
		copyHiddenPresetPassword(merged[i].Meta, currentPreset.Meta)
	}
	return merged
}

func hasPresetPasswordMeta(meta map[string]string) bool {
	if meta == nil {
		return false
	}
	if _, ok := meta[configuration.PresetMetaPassword]; ok {
		return true
	}
	if _, ok := meta[configuration.PresetMetaEncryptedPassword]; ok {
		return true
	}
	return false
}

func copyPresetMeta(meta map[string]string) map[string]string {
	copied := make(map[string]string, len(meta))
	for key, value := range meta {
		copied[key] = value
	}
	return copied
}

func copyHiddenPresetPassword(
	target map[string]string,
	source map[string]string,
) {
	if source == nil {
		return
	}
	if value, ok := source[configuration.PresetMetaPassword]; ok {
		target[configuration.PresetMetaPassword] = value
	}
	if value, ok := source[configuration.PresetMetaEncryptedPassword]; ok {
		target[configuration.PresetMetaEncryptedPassword] = value
	}
}

// requireAuth applies the same credential role matching used by socket verify.
func (p presetConfig) requireAuth(r *http.Request) (authRole, error) {
	role, err := requestAuthRoleForCommon(p.commonCfg, r, true)
	if err != nil {
		return authRoleNone, err
	}
	if role < authRoleUser {
		return authRoleNone, ErrSocketInvalidAuthKey
	}
	return role, nil
}

// writePresets serializes presets using the same preset shape sent to the UI.
func (p presetConfig) writePresets(
	w *ResponseWriter,
	presets []configuration.Preset,
) error {
	w.Header().Add("Cache-Control", "no-store")
	w.Header().Add("Pragma", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(presetConfigResponse{
		Presets: newSocketAccessConfiguration(presets, "", "", false).Presets,
	})
}
