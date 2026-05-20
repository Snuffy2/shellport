// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type etSSHMaterial struct {
	IdentityPath   string
	KnownHostsPath string
	ConfigPath     string
}

func buildETClientArgs(metadata etMetadata, user string, sshAddress string, sshConfigPath string) []string {
	target := sshAddress
	if splitHost, _, err := net.SplitHostPort(sshAddress); err == nil {
		target = net.JoinHostPort(splitHost, strconv.Itoa(metadata.ServerPort))
		return []string{
			"-ssh-config",
			sshConfigPath,
			fmt.Sprintf("%s@%s", user, target),
		}
	}

	return []string{
		"-ssh-config",
		sshConfigPath,
		fmt.Sprintf("%s@%s:%d", user, target, metadata.ServerPort),
	}
}

func writeETSSHMaterial(dir string, privateKey []byte, knownHostsLine string, sshAddress string) (etSSHMaterial, error) {
	material := etSSHMaterial{
		IdentityPath:   filepath.Join(dir, "identity"),
		KnownHostsPath: filepath.Join(dir, "known_hosts"),
		ConfigPath:     filepath.Join(dir, "ssh_config"),
	}
	if err := os.WriteFile(material.IdentityPath, privateKey, 0o600); err != nil {
		return etSSHMaterial{}, err
	}
	trimmedKnownHostsLine := strings.TrimRight(knownHostsLine, "\r\n")
	if err := os.WriteFile(material.KnownHostsPath, []byte(trimmedKnownHostsLine+"\n"), 0o600); err != nil {
		return etSSHMaterial{}, err
	}

	configLines := []string{
		"Host *",
		"  IdentitiesOnly yes",
		"  IdentityFile " + material.IdentityPath,
		"  UserKnownHostsFile " + material.KnownHostsPath,
		"  StrictHostKeyChecking yes",
		"  BatchMode yes",
	}
	if _, splitPort, err := net.SplitHostPort(sshAddress); err == nil {
		configLines = append(configLines, "  Port "+splitPort)
	}
	config := strings.Join(configLines, "\n") + "\n"
	if err := os.WriteFile(material.ConfigPath, []byte(config), 0o600); err != nil {
		return etSSHMaterial{}, err
	}

	return material, nil
}

func cleanupETTempDir(dir string) error {
	if dir == "" {
		return nil
	}

	return os.RemoveAll(dir)
}
