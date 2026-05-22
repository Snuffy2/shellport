// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package configuration

import (
	"errors"
	"fmt"
	"time"

	"github.com/Snuffy2/shellport/application/network"
)

// Configuration is the top-level application configuration produced by a
// Loader. It aggregates global settings, server definitions, preset endpoints,
// hook rules, and optional SOCKS5 proxy credentials. Use Verify to validate
// the values before using the configuration.
type Configuration struct {
	SourceFile             string
	HostName               string
	UserPassword           string
	AdminPassword          string
	DialTimeout            time.Duration
	Socks5                 string
	Socks5User             string
	Socks5Password         string
	Hooks                  Hooks
	HookTimeout            time.Duration
	Servers                []Server
	Presets                []Preset
	OnlyAllowPresetRemotes bool
}

// Verify verifies current setting
func (c Configuration) Verify() error {
	if err := c.Hooks.verify(); err != nil {
		return fmt.Errorf("invalid Hook settings: %s", err)
	}
	if len(c.Servers) <= 0 {
		return errors.New("must specify at least one server")
	}
	for i, c := range c.Servers {
		if vErr := c.verify(); vErr == nil {
			continue
		} else {
			return fmt.Errorf("invalid setting for server %d: %s", i, vErr)
		}
	}
	return nil
}

// Dialer constructs and returns the network.Dial function for this
// configuration. When a SOCKS5 proxy is specified it wraps a plain TCP dialer.
// When OnlyAllowPresetRemotes is true it additionally wraps the dialer in an
// access-control layer that restricts connections to preset hosts.
func (c Configuration) Dialer() network.Dial {
	d := network.TCPDial()
	if len(c.Socks5) > 0 {
		d = network.BuildSocks5Dial(c.Socks5, c.Socks5User, c.Socks5Password, d)
	}
	if c.OnlyAllowPresetRemotes {
		d = network.AccessControlDial(
			NewPresetRepository(c.Presets),
			d,
		)
	}
	return d
}

// hookSettings converts the configuration's Hooks and HookTimeout fields into
// a HookSettings value suitable for passing to the command layer.
func (c Configuration) hookSettings() HookSettings {
	return HookSettings{
		Timeout: c.HookTimeout,
		Hooks:   c.Hooks,
	}
}

// Common derives the Common settings struct from the Configuration, building
// the Dialer and HookSettings in the process.
func (c Configuration) Common() Common {
	presetRepository := NewPresetRepository(c.Presets)
	return Common{
		SourceFile:             c.SourceFile,
		HostName:               c.HostName,
		UserPassword:           c.UserPassword,
		AdminPassword:          c.AdminPassword,
		Dialer:                 c.dialerWithPresetRepository(presetRepository),
		DialTimeout:            c.DialTimeout,
		Socks5Configured:       len(c.Socks5) > 0,
		Presets:                c.Presets,
		PresetRepository:       presetRepository,
		Hooks:                  c.hookSettings(),
		OnlyAllowPresetRemotes: c.OnlyAllowPresetRemotes,
	}
}

// dialerWithPresetRepository constructs a Dial function using presetRepository
// as the live allow-list when preset-only dialing is enabled.
func (c Configuration) dialerWithPresetRepository(
	presetRepository *PresetRepository,
) network.Dial {
	d := network.TCPDial()
	if len(c.Socks5) > 0 {
		d = network.BuildSocks5Dial(c.Socks5, c.Socks5User, c.Socks5Password, d)
	}
	if c.OnlyAllowPresetRemotes {
		d = network.AccessControlDial(presetRepository, d)
	}
	return d
}

// DecideDialTimeout returns the effective dial timeout clamped to the given
// maximum. If DialTimeout is zero or negative it falls through to max.
func (c Common) DecideDialTimeout(max time.Duration) time.Duration {
	return clampRange(c.DialTimeout, max, 0)
}
