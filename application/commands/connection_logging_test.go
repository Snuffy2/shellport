// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Snuffy2/shellport/application/log"
)

func TestDebugConnectionAttemptLogsSanitizedConnectionDetails(t *testing.T) {
	var output bytes.Buffer
	logger := log.NewWriter("Test", &output)

	debugConnectionAttempt(
		logger,
		connectionDebugDetails{
			Protocol:   "SSH",
			User:       "alice",
			Address:    "example.com:22",
			Network:    "tcp",
			AuthMethod: "password",
			PresetID:   "production",
		},
	)

	got := output.String()
	for _, want := range []string{
		"Attempting SSH connection",
		"user=alice",
		"address=example.com:22",
		"network=tcp",
		"auth_method=password",
		"preset_id=production",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected log output %q to contain %q", got, want)
		}
	}

	for _, secret := range []string{"PRIVATE KEY", "password=", "shared_key", "SHELLPORT_SHAREDKEY"} {
		if strings.Contains(got, secret) {
			t.Fatalf("expected log output %q not to contain secret marker %q", got, secret)
		}
	}
}

func TestDebugConnectionAttemptOmitsEmptyOptionalDetails(t *testing.T) {
	var output bytes.Buffer
	logger := log.NewWriter("Test", &output)

	debugConnectionAttempt(
		logger,
		connectionDebugDetails{
			Protocol: "Telnet",
			Address:  "example.com:23",
			Network:  "tcp",
		},
	)

	got := output.String()
	for _, forbidden := range []string{"user=", "auth_method=", "preset_id="} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("expected log output %q to omit empty detail %q", got, forbidden)
		}
	}
}

func TestSSHAuthMethodDebugName(t *testing.T) {
	tests := []struct {
		method byte
		want   string
	}{
		{method: SSHAuthMethodNone, want: "none"},
		{method: SSHAuthMethodPassphrase, want: "password"},
		{method: SSHAuthMethodPrivateKey, want: "private_key"},
		{method: 0xff, want: "unknown(255)"},
	}

	for _, test := range tests {
		if got := sshAuthMethodDebugName(test.method); got != test.want {
			t.Fatalf("expected auth method %d to render as %q, got %q", test.method, test.want, got)
		}
	}
}
