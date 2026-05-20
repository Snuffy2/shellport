// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"bytes"
	"io"
	"net"
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
		`user="alice"`,
		`address="example.com:22"`,
		`network="tcp"`,
		`auth_method="password"`,
		`preset_id="production"`,
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

func TestDebugConnectionFailedLogsSanitizedConnectionDetails(t *testing.T) {
	var output bytes.Buffer
	logger := log.NewWriter("Test", &output)

	debugConnectionFailed(
		logger,
		connectionDebugDetails{
			Protocol:   "Mosh",
			User:       "alice",
			Address:    "example.com:22",
			Network:    "tcp",
			AuthMethod: "private_key",
			PresetID:   "production",
		},
		errTestConnectionLogging,
	)

	got := output.String()
	for _, want := range []string{
		"[WRN]",
		"Mosh connection failed",
		`user="alice"`,
		`address="example.com:22"`,
		`network="tcp"`,
		`auth_method="private_key"`,
		`preset_id="production"`,
		"error=test connection logging error",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected log output %q to contain %q", got, want)
		}
	}

	assertNoSecretMarkers(t, got)
}

func TestDebugConnectionEstablishedLogsSanitizedConnectionDetails(t *testing.T) {
	var output bytes.Buffer
	logger := log.NewWriter("Test", &output)

	debugConnectionEstablished(
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
		"SSH connection established",
		`user="alice"`,
		`address="example.com:22"`,
		`network="tcp"`,
		`auth_method="password"`,
		`preset_id="production"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected log output %q to contain %q", got, want)
		}
	}

	assertNoSecretMarkers(t, got)
}

func TestDebugConnectionDisconnectedLogsCleanExit(t *testing.T) {
	var output bytes.Buffer
	logger := log.NewWriter("Test", &output)

	debugConnectionDisconnected(
		logger,
		connectionDebugDetails{
			Protocol: "Telnet",
			Address:  "example.com:23",
			Network:  "tcp",
		},
		"remote goroutine exited",
		nil,
	)

	got := output.String()
	for _, want := range []string{
		"Telnet connection disconnected",
		`address="example.com:23"`,
		`network="tcp"`,
		"reason=remote goroutine exited",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected log output %q to contain %q", got, want)
		}
	}

	for _, forbidden := range []string{"user=", "auth_method=", "preset_id=", "error="} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("expected log output %q to omit empty detail %q", got, forbidden)
		}
	}
}

func TestDebugConnectionDisconnectedLogsExpectedReadShutdownAtDebug(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{
			name: "EOF",
			err:  io.EOF,
		},
		{
			name: "closed pipe",
			err:  io.ErrClosedPipe,
		},
		{
			name: "closed network connection",
			err:  net.ErrClosed,
		},
		{
			name: "closed mosh session",
			err:  ErrMoshSessionClosed,
		},
		{
			name: "remote et process unavailable",
			err:  ErrETRemoteUnavailable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := log.NewWriter("Test", &output)

			debugConnectionDisconnected(
				logger,
				connectionDebugDetails{
					Protocol: "SSH",
					Address:  "example.com:22",
					Network:  "tcp",
				},
				"stdout stream ended",
				test.err,
			)

			got := output.String()
			for _, want := range []string{
				"[DBG]",
				"SSH connection disconnected",
				`address="example.com:22"`,
				`network="tcp"`,
				"reason=stdout stream ended",
				"error=" + test.err.Error(),
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("expected log output %q to contain %q", got, want)
				}
			}
			if strings.Contains(got, "[WRN]") {
				t.Fatalf("expected log output %q not to warn for expected shutdown", got)
			}
		})
	}
}

func TestDebugConnectionDisconnectedLogsErrorExit(t *testing.T) {
	var output bytes.Buffer
	logger := log.NewWriter("Test", &output)

	debugConnectionDisconnected(
		logger,
		connectionDebugDetails{
			Protocol:   "SSH",
			User:       "alice",
			Address:    "example.com:22",
			Network:    "tcp",
			AuthMethod: "password",
			PresetID:   "production",
		},
		"stdout stream ended",
		errTestConnectionLogging,
	)

	got := output.String()
	for _, want := range []string{
		"[WRN]",
		"SSH connection disconnected",
		`user="alice"`,
		`address="example.com:22"`,
		`network="tcp"`,
		`auth_method="password"`,
		`preset_id="production"`,
		"reason=stdout stream ended",
		"error=test connection logging error",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected log output %q to contain %q", got, want)
		}
	}

	assertNoSecretMarkers(t, got)
}

func TestConnectionDebugDetailsFieldsQuoteUntrustedValues(t *testing.T) {
	details := connectionDebugDetails{
		Protocol:   "SSH",
		User:       "alice\nforged=true",
		Address:    "example.com:22\rcontrol",
		Network:    "tcp\tproxy",
		AuthMethod: "password\nignored=true",
		PresetID:   "prod\x1b[31m",
	}

	got := details.fields()
	for _, want := range []string{
		`user="alice\nforged=true"`,
		`address="example.com:22\rcontrol"`,
		`network="tcp\tproxy"`,
		`auth_method="password\nignored=true"`,
		`preset_id="prod\x1b[31m"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected fields %q to contain %q", got, want)
		}
	}

	for _, forbidden := range []string{"alice\nforged=true", "example.com:22\rcontrol", "tcp\tproxy", "password\nignored=true"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("expected fields %q not to contain raw control sequence %q", got, forbidden)
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

type testConnectionLoggingError struct{}

func (testConnectionLoggingError) Error() string {
	return "test connection logging error"
}

var errTestConnectionLogging = testConnectionLoggingError{}

func assertNoSecretMarkers(t *testing.T, got string) {
	t.Helper()

	for _, secret := range []string{"PRIVATE KEY", "password=", "shared_key", "SHELLPORT_SHAREDKEY"} {
		if strings.Contains(got, secret) {
			t.Fatalf("expected log output %q not to contain secret marker %q", got, secret)
		}
	}
}
