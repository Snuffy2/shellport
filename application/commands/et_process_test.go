// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildETClientArgsUsesSSHOptionsAndServerPort(t *testing.T) {
	args := buildETClientArgs(etMetadata{Command: "et", ServerPort: 22022}, "alice", "example.com:22", "/tmp/ssh_config")
	want := []string{
		"--telemetry=false",
		"--port", "22022",
		"--ssh-option", "IdentitiesOnly=yes",
		"--ssh-option", "IdentityFile=/tmp/identity",
		"--ssh-option", "UserKnownHostsFile=/tmp/known_hosts",
		"--ssh-option", "StrictHostKeyChecking=yes",
		"--ssh-option", "BatchMode=yes",
		"--ssh-option", "Port=22",
		"--",
		"alice@example.com",
	}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestBuildETClientArgsSupportsIPv6Address(t *testing.T) {
	args := buildETClientArgs(etMetadata{Command: "et", ServerPort: 22022}, "alice", "[2001:db8::1]:22", "/tmp/ssh_config")
	want := []string{
		"--telemetry=false",
		"--port", "22022",
		"--ssh-option", "IdentitiesOnly=yes",
		"--ssh-option", "IdentityFile=/tmp/identity",
		"--ssh-option", "UserKnownHostsFile=/tmp/known_hosts",
		"--ssh-option", "StrictHostKeyChecking=yes",
		"--ssh-option", "BatchMode=yes",
		"--ssh-option", "Port=22",
		"--",
		"alice@2001:db8::1",
	}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestBuildETClientArgsTerminatesOptionsBeforeTarget(t *testing.T) {
	args := buildETClientArgs(etMetadata{Command: "et", ServerPort: 2022}, "-h", "example.com:22", "/tmp/ssh_config")
	if len(args) < 2 {
		t.Fatalf("args = %#v, want option terminator and target", args)
	}
	if args[len(args)-2] != "--" {
		t.Fatalf("args = %#v, want option terminator before target", args)
	}
	if args[len(args)-1] != "-h@example.com" {
		t.Fatalf("target = %q, want %q", args[len(args)-1], "-h@example.com")
	}
}

func TestWriteETSSHMaterialCreatesRestrictiveFiles(t *testing.T) {
	dir := t.TempDir()
	knownHostsLine := "example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeKeyMaterialForTestOnly"
	material, err := writeETSSHMaterial(dir, []byte("PRIVATE KEY\n"), knownHostsLine, "example.com:2222")
	if err != nil {
		t.Fatalf("writeETSSHMaterial() error = %v", err)
	}

	for _, path := range []string{material.IdentityPath, material.KnownHostsPath, material.ConfigPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s mode = %v, want no group/other permissions", path, info.Mode().Perm())
		}
	}

	knownHostsData, err := os.ReadFile(material.KnownHostsPath)
	if err != nil {
		t.Fatalf("read known hosts: %v", err)
	}
	if strings.TrimSpace(string(knownHostsData)) != knownHostsLine {
		t.Fatalf("known hosts line = %q, want %q", strings.TrimSpace(string(knownHostsData)), knownHostsLine)
	}

	config, err := os.ReadFile(material.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	configText := string(config)
	for _, expected := range []string{"IdentityFile " + material.IdentityPath, "UserKnownHostsFile " + material.KnownHostsPath, "BatchMode yes", "Port 2222"} {
		if !strings.Contains(configText, expected) {
			t.Fatalf("config missing %q:\n%s", expected, configText)
		}
	}
}

func TestCleanupETTempDirRemovesDirectory(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "file")
	if err := os.WriteFile(child, []byte("x"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := cleanupETTempDir(dir); err != nil {
		t.Fatalf("cleanupETTempDir() error = %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("stat dir error = %v, want not exist", err)
	}
}

func TestStartETPTYStartsProcessWithPTYSession(t *testing.T) {
	command, err := exec.LookPath("true")
	if err != nil {
		t.Skip("true command is unavailable")
	}

	process, err := startETPTY(
		context.Background(),
		etMetadata{Command: command, ServerPort: 2022},
		"alice",
		"example.com:22",
		filepath.Join(t.TempDir(), "ssh_config"),
	)
	if err != nil {
		t.Fatalf("startETPTY() error = %v", err)
	}
	if err := process.Close(); err != nil {
		t.Fatalf("process.Close() error = %v", err)
	}
}
