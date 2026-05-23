// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

// Package configuration defines the data types and loader infrastructure used
// to supply runtime settings to the ShellPort application. It supports multiple
// configuration sources through the Loader function type and loader chain.
package configuration

import (
	"time"

	"github.com/Snuffy2/shellport/application/network"
)

// Common holds the configuration settings that are shared across all server
// instances within a single Configuration. It is derived from a Configuration
// via Configuration.Common() and passed to each server at startup.
type Common struct {
	// SourceFile is the YAML configuration file path when loaded from disk.
	SourceFile string
	// HostName restricts accepted HTTP Host headers when set.
	HostName string
	// UserPassword is the password required for client authentication;
	// an empty value disables authentication.
	UserPassword string
	// AdminPassword is the password that grants admin-level access.
	AdminPassword string
	// Dialer is the function used to open outbound network connections,
	// optionally via SOCKS5 or with access-control restrictions.
	Dialer network.Dial
	// DialTimeout is the maximum duration permitted for a single outbound dial.
	DialTimeout time.Duration
	// Socks5Configured reports whether Dialer routes traffic through SOCKS5.
	Socks5Configured bool
	// Presets is the list of pre-configured remote endpoints shown in the UI.
	Presets []Preset
	// PresetRepository stores the live preset list for runtime updates.
	PresetRepository *PresetRepository
	// Hooks contains the hook settings that govern lifecycle callbacks.
	Hooks HookSettings
	// OnlyAllowPresetRemotes restricts outbound connections to hosts listed in
	// Presets when true.
	OnlyAllowPresetRemotes bool
}

// CurrentPresets returns the current live preset list.
func (c Common) CurrentPresets() []Preset {
	if c.PresetRepository == nil {
		return c.Presets
	}
	return c.PresetRepository.List()
}

// PresetConfigWritable reports whether runtime preset changes can be persisted.
func (c Common) PresetConfigWritable() bool {
	return PresetConfigWritable(c.SourceFile)
}
