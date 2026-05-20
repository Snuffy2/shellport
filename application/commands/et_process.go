// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type etProcess interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Resize(rows uint16, cols uint16) error
	Close() error
}

type etPTYProcess struct {
	cmd       *exec.Cmd
	file      *os.File
	closeLock sync.Mutex
	closed    bool
}

const (
	etPTYCloseTermTimeout = 3 * time.Second
	etPTYCloseKillTimeout = 3 * time.Second
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

func startETPTY(ctx context.Context, metadata etMetadata, user string, sshAddress string, sshConfigPath string) (etProcess, error) {
	args := buildETClientArgs(metadata, user, sshAddress, sshConfigPath)
	cmd := exec.CommandContext(ctx, metadata.Command, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	file, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	return &etPTYProcess{cmd: cmd, file: file}, nil
}

func (p *etPTYProcess) Read(b []byte) (int, error) {
	return p.file.Read(b)
}

func (p *etPTYProcess) Write(b []byte) (int, error) {
	return p.file.Write(b)
}

func (p *etPTYProcess) Resize(rows uint16, cols uint16) error {
	return pty.Setsize(p.file, &pty.Winsize{Rows: rows, Cols: cols})
}

func (p *etPTYProcess) Close() error {
	p.closeLock.Lock()
	if p.closed {
		p.closeLock.Unlock()
		return nil
	}
	p.closed = true
	p.closeLock.Unlock()

	pid := 0
	if p.cmd != nil && p.cmd.Process != nil {
		pid = p.cmd.Process.Pid
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
	}
	if p.file != nil {
		_ = p.file.Close()
	}
	if p.cmd == nil {
		return nil
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- p.cmd.Wait()
	}()

	waitErr, termTimedOut := waitForError(waitCh, etPTYCloseTermTimeout)
	if !termTimedOut {
		if waitErr != nil && !isExpectedSignalExit(waitErr, syscall.SIGTERM) {
			return fmt.Errorf("et process group reaped with error: %w", waitErr)
		}
		return nil
	}

	closeErrs := []error{
		fmt.Errorf(
			"et process group did not exit after SIGTERM in %s: %w",
			etPTYCloseTermTimeout,
			contextTimeoutError(etPTYCloseTermTimeout),
		),
	}

	if pid > 0 {
		if killErr := syscall.Kill(-pid, syscall.SIGKILL); killErr != nil {
			closeErrs = append(
				closeErrs,
				fmt.Errorf("send SIGKILL to et process group %d failed: %w", pid, killErr),
			)
		}
	}

	reapedErr, killTimedOut := waitForError(waitCh, etPTYCloseKillTimeout)
	if !killTimedOut {
		if reapedErr != nil && !isExpectedSignalExit(reapedErr, syscall.SIGKILL) {
			closeErrs = append(
				closeErrs,
				fmt.Errorf("et process group reaped with error: %w", reapedErr),
			)
		}
		return errors.Join(closeErrs...)
	}

	closeErrs = append(
		closeErrs,
		fmt.Errorf(
			"et process group did not exit after SIGKILL in %s",
			etPTYCloseKillTimeout,
		),
	)
	return errors.Join(closeErrs...)
}

func isExpectedSignalExit(err error, signal syscall.Signal) bool {
	var execErr *exec.ExitError
	if !errors.As(err, &execErr) {
		return false
	}

	waitStatus, ok := execErr.Sys().(syscall.WaitStatus)
	if !ok {
		return false
	}

	return waitStatus.Signaled() && waitStatus.Signal() == signal
}

func waitForError(ch <-chan error, timeout time.Duration) (error, bool) {
	t := time.NewTimer(timeout)
	defer t.Stop()

	select {
	case waitErr := <-ch:
		return waitErr, false
	case <-t.C:
		return nil, true
	}
}

func contextTimeoutError(timeout time.Duration) error {
	return fmt.Errorf("%w (%s)", context.DeadlineExceeded, timeout)
}
