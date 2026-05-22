// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
)

func TestPresetPasswordCredentialMatchesHostUserAndPasswordAuth(t *testing.T) {
	credential, ok := presetPasswordCredential(
		command.Configuration{
			Presets: []configuration.Preset{
				{
					ID:   "preset-atlantis",
					Type: "SSH",
					Host: "atlantis.home:22",
					Meta: map[string]string{
						"Authentication": "Password",
						"User":           "pi",
					},
					SecretMeta: map[string]string{
						"Password": "mypassword",
					},
				},
			},
		},
		"SSH",
		"preset-atlantis",
		"pi",
		"atlantis.home:22",
	)

	if !ok {
		t.Fatal("presetPasswordCredential ok = false, want true")
	}
	if credential != "mypassword" {
		t.Fatalf("credential = %q, want mypassword", credential)
	}
}

func TestPresetPrivateKeyCredentialReadsFileReference(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "atlantis.key")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY DATA"), 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	credential, ok := presetPrivateKeyCredential(
		command.Configuration{
			Presets: []configuration.Preset{
				{
					ID:   "preset-atlantis",
					Type: "SSH",
					Host: "atlantis.home:22",
					Meta: map[string]string{
						"Authentication": "Private Key",
						"User":           "pi",
						"Private Key":    "file://" + keyPath,
					},
				},
			},
		},
		"SSH",
		"preset-atlantis",
		"pi",
		"atlantis.home:22",
	)

	if !ok {
		t.Fatal("presetPrivateKeyCredential ok = false, want true")
	}
	if credential != "PRIVATE KEY DATA" {
		t.Fatalf("credential = %q, want PRIVATE KEY DATA", credential)
	}
}

func TestPresetPrivateKeyCredentialPreservesEnvironmentReference(t *testing.T) {
	t.Setenv("SHELLPORT_TEST_PRIVATE_KEY", "ENV PRIVATE KEY DATA")

	credential, ok := presetPrivateKeyCredential(
		command.Configuration{
			Presets: []configuration.Preset{
				{
					ID:   "preset-atlantis",
					Type: "SSH",
					Host: "atlantis.home:22",
					Meta: map[string]string{
						"Authentication": "Private Key",
						"User":           "pi",
						"Private Key":    "environment://SHELLPORT_TEST_PRIVATE_KEY",
					},
				},
			},
		},
		"SSH",
		"preset-atlantis",
		"pi",
		"atlantis.home:22",
	)

	if !ok {
		t.Fatal("presetPrivateKeyCredential ok = false, want true")
	}
	if credential != "ENV PRIVATE KEY DATA" {
		t.Fatalf("credential = %q, want ENV PRIVATE KEY DATA", credential)
	}
}

func TestPresetPrivateKeyCredentialTreatsEmptyResolvedReferenceAsMissing(
	t *testing.T,
) {
	t.Setenv("SHELLPORT_TEST_PRIVATE_KEY", "")

	credential, ok := presetPrivateKeyCredential(
		command.Configuration{
			Presets: []configuration.Preset{
				{
					ID:   "preset-atlantis",
					Type: "SSH",
					Host: "atlantis.home:22",
					Meta: map[string]string{
						"Authentication": "Private Key",
						"User":           "pi",
						"Private Key":    "environment://SHELLPORT_TEST_PRIVATE_KEY",
					},
				},
			},
		},
		"SSH",
		"preset-atlantis",
		"pi",
		"atlantis.home:22",
	)

	if ok {
		t.Fatal("presetPrivateKeyCredential ok = true, want false")
	}
	if credential != "" {
		t.Fatalf("credential = %q, want empty", credential)
	}
}

func TestPresetPasswordCredentialUsesLivePresetRepository(t *testing.T) {
	repo := configuration.NewPresetRepository([]configuration.Preset{
		{
			ID:   "preset-atlantis",
			Type: "SSH",
			Host: "atlantis.home:22",
			Meta: map[string]string{
				"Authentication": "Password",
				"User":           "pi",
				"Password":       "oldpassword",
			},
		},
	})
	repo.Replace([]configuration.Preset{
		{
			ID:   "preset-atlantis",
			Type: "SSH",
			Host: "atlantis.home:22",
			Meta: map[string]string{
				"Authentication": "Password",
				"User":           "pi",
				"Password":       "newpassword",
			},
		},
	})

	credential, ok := presetPasswordCredential(
		command.Configuration{
			PresetRepository: repo,
			Presets: []configuration.Preset{
				{
					ID:   "preset-atlantis",
					Type: "SSH",
					Host: "atlantis.home:22",
					Meta: map[string]string{
						"Authentication": "Password",
						"User":           "pi",
						"Password":       "oldpassword",
					},
				},
			},
		},
		"SSH",
		"preset-atlantis",
		"pi",
		"atlantis.home:22",
	)

	if !ok {
		t.Fatal("presetPasswordCredential ok = false, want true")
	}
	if credential != "newpassword" {
		t.Fatalf("credential = %q, want newpassword", credential)
	}
}

func TestPresetPasswordCredentialMatchesCommandType(t *testing.T) {
	credential, ok := presetPasswordCredential(
		command.Configuration{
			Presets: []configuration.Preset{
				{
					ID:   "preset-mosh",
					Type: "Mosh",
					Host: "atlantis.home:22",
					Meta: map[string]string{
						"Authentication": "Password",
						"User":           "pi",
						"Password":       "moshpassword",
					},
				},
				{
					ID:   "preset-ssh",
					Type: "SSH",
					Host: "atlantis.home:22",
					Meta: map[string]string{
						"Authentication": "Password",
						"User":           "pi",
						"Password":       "sshpassword",
					},
				},
			},
		},
		"SSH",
		"preset-ssh",
		"pi",
		"atlantis.home:22",
	)

	if !ok {
		t.Fatal("presetPasswordCredential ok = false, want true")
	}
	if credential != "sshpassword" {
		t.Fatalf("credential = %q, want sshpassword", credential)
	}
}

func TestPresetPasswordCredentialRequiresPresetID(t *testing.T) {
	_, ok := presetPasswordCredential(
		command.Configuration{
			Presets: []configuration.Preset{
				{
					ID:   "preset-atlantis",
					Type: "SSH",
					Host: "atlantis.home:22",
					Meta: map[string]string{
						"Authentication": "Password",
						"User":           "pi",
						"Password":       "mypassword",
					},
				},
			},
		},
		"SSH",
		"",
		"pi",
		"atlantis.home:22",
	)

	if ok {
		t.Fatal("presetPasswordCredential ok = true without preset ID, want false")
	}
}

func TestPresetPasswordCredentialMatchesPresetID(t *testing.T) {
	credential, ok := presetPasswordCredential(
		command.Configuration{
			Presets: []configuration.Preset{
				{
					ID:   "preset-atlantis",
					Type: "SSH",
					Host: "shared.home:22",
					Meta: map[string]string{
						"Authentication": "Password",
						"User":           "pi",
						"Password":       "atlantis-password",
					},
				},
				{
					ID:   "preset-columbia",
					Type: "SSH",
					Host: "shared.home:22",
					Meta: map[string]string{
						"Authentication": "Password",
						"User":           "pi",
						"Password":       "columbia-password",
					},
				},
			},
		},
		"SSH",
		"preset-columbia",
		"pi",
		"shared.home:22",
	)

	if !ok {
		t.Fatal("presetPasswordCredential ok = false, want true")
	}
	if credential != "columbia-password" {
		t.Fatalf("credential = %q, want columbia-password", credential)
	}
}
