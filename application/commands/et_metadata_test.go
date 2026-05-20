// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"errors"
	"testing"

	"github.com/Snuffy2/shellport/application/rw"
)

func TestParseETMetadataDefaults(t *testing.T) {
	limited := rw.NewLimitedReader(nil, 0)
	metadata, err := parseETMetadata(&limited, make([]byte, 1024))
	if err != nil {
		t.Fatalf("parseETMetadata() error = %v", err)
	}
	if metadata.ServerPort != etDefaultServerPort {
		t.Fatalf("ServerPort = %d, want %d", metadata.ServerPort, etDefaultServerPort)
	}
	if metadata.Command != etDefaultCommand {
		t.Fatalf("Command = %q, want %q", metadata.Command, etDefaultCommand)
	}
}

func TestParseETMetadataWireFormat(t *testing.T) {
	metadata, err := parseETMetadata(
		makeETMetadataReader(t, []string{"6022", "/usr/local/bin/et"}),
		make([]byte, 1024),
	)
	if err != nil {
		t.Fatalf("parseETMetadata() error = %v", err)
	}
	if metadata.ServerPort != 6022 {
		t.Fatalf("ServerPort = %d, want %d", metadata.ServerPort, 6022)
	}
	if metadata.Command != "/usr/local/bin/et" {
		t.Fatalf("Command = %q, want %q", metadata.Command, "/usr/local/bin/et")
	}
}

func TestParseETMetadataBlankPortIsInvalid(t *testing.T) {
	for _, port := range []string{"", "   "} {
		_, err := parseETMetadata(
			makeETMetadataReader(t, []string{port, "/usr/local/bin/et"}),
			make([]byte, 1024),
		)
		if err == nil {
			t.Fatal("expected parseETMetadata to reject blank port")
		}
		if !errors.Is(err, ErrETInvalidServerPort) {
			t.Fatalf("parseETMetadata() error = %v, want %v", err, ErrETInvalidServerPort)
		}
	}
}

func TestParseETMetadataBlankCommandIsInvalid(t *testing.T) {
	for _, command := range []string{"", " \n"} {
		_, err := parseETMetadata(
			makeETMetadataReader(t, []string{"2022", command}),
			make([]byte, 1024),
		)
		if err == nil {
			t.Fatal("expected parseETMetadata to reject blank command")
		}
		if !errors.Is(err, ErrETInvalidCommand) {
			t.Fatalf("parseETMetadata() error = %v, want %v", err, ErrETInvalidCommand)
		}
	}
}

func TestValidateETServerPort(t *testing.T) {
	for _, port := range []int{1, 2022, 65535} {
		if err := validateETServerPort(port); err != nil {
			t.Fatalf("validateETServerPort(%d) error = %v", port, err)
		}
	}
	for _, port := range []int{0, -1, 65536} {
		if err := validateETServerPort(port); !errors.Is(err, ErrETInvalidServerPort) {
			t.Fatalf("validateETServerPort(%d) error = %v, want ErrETInvalidServerPort", port, err)
		}
	}
}

func TestValidateETCommand(t *testing.T) {
	if err := validateETCommand("et"); err != nil {
		t.Fatalf("validateETCommand(et) error = %v", err)
	}
	if err := validateETCommand("/usr/local/bin/et"); err != nil {
		t.Fatalf("validateETCommand(path) error = %v", err)
	}
	for _, command := range []string{"", "et --flag", "et\nbad"} {
		if err := validateETCommand(command); !errors.Is(err, ErrETInvalidCommand) {
			t.Fatalf("validateETCommand(%q) error = %v, want ErrETInvalidCommand", command, err)
		}
	}
}

func makeETMetadataReader(t *testing.T, values []string) *rw.LimitedReader {
	t.Helper()

	payload := make([]byte, 0, 256)
	buf := make([]byte, 128)

	for _, value := range values {
		field := NewString([]byte(value))
		marshalLen, marshalErr := field.Marshal(buf)
		if marshalErr != nil {
			t.Fatalf("failed to marshal %q: %v", value, marshalErr)
		}
		payload = append(payload, buf[:marshalLen]...)
	}

	payloadCopy := append([]byte(nil), payload...)
	payloadLen := len(payloadCopy)
	reader := rw.NewFetchReader(func() ([]byte, error) {
		current := payloadCopy
		payloadCopy = nil

		return current, nil
	})
	limited := rw.NewLimitedReader(&reader, payloadLen)
	return &limited
}
