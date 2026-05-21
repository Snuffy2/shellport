// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/rw"
)

func parseOptionalPresetID(r *rw.LimitedReader, b []byte) (string, error) {
	if r.Completed() {
		return "", nil
	}
	presetID, err := ParseString(r.Read, b)
	if err != nil {
		return "", err
	}
	return string(presetID.Data()), nil
}

func presetPasswordCredential(
	cfg command.Configuration,
	presetType string,
	presetID string,
	user string,
	host string,
) (string, bool) {
	if presetID == "" {
		return "", false
	}
	presets := cfg.Presets
	if cfg.PresetRepository != nil {
		presets = cfg.PresetRepository.List()
	}
	for _, preset := range presets {
		if preset.ID != presetID {
			continue
		}
		if preset.Type != presetType {
			continue
		}
		if preset.Host != host {
			continue
		}
		if preset.Meta["User"] != user {
			continue
		}
		if preset.Meta["Authentication"] != "Password" {
			continue
		}
		if preset.SecretMeta != nil {
			if credential := preset.SecretMeta[configuration.PresetMetaPassword]; credential != "" {
				return credential, true
			}
		}
		if credential := preset.Meta[configuration.PresetMetaPassword]; credential != "" {
			return credential, true
		}
	}
	return "", false
}

func presetPrivateKeyCredential(
	cfg command.Configuration,
	presetType string,
	presetID string,
	user string,
	host string,
) (string, bool) {
	if presetID == "" {
		return "", false
	}
	presets := cfg.Presets
	if cfg.PresetRepository != nil {
		presets = cfg.PresetRepository.List()
	}
	for _, preset := range presets {
		if preset.ID != presetID {
			continue
		}
		if preset.Type != presetType {
			continue
		}
		if preset.Host != host {
			continue
		}
		if preset.Meta["User"] != user {
			continue
		}
		if preset.Meta["Authentication"] != "Private Key" {
			continue
		}
		credential := preset.Meta[configuration.PresetMetaPrivateKey]
		if credential == "" {
			return "", false
		}
		parsed, err := configuration.String(credential).Parse()
		if err != nil {
			return "", false
		}
		return parsed, true
	}
	return "", false
}
