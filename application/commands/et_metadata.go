// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/Snuffy2/shellport/application/rw"
)

const (
	etDefaultCommand    = "et"
	etDefaultServerPort = 2022
	etMaxCommandLen     = 512
)

var (
	ErrETInvalidServerPort = errors.New("invalid ET server port")
	ErrETInvalidCommand    = errors.New("invalid ET command")
)

type etMetadata struct {
	ServerPort int
	Command    string
}

func defaultETMetadata() etMetadata {
	return etMetadata{
		ServerPort: etDefaultServerPort,
		Command:    etDefaultCommand,
	}
}

func parseETMetadata(r *rw.LimitedReader, b []byte) (etMetadata, error) {
	metadata := defaultETMetadata()
	if r == nil || r.Completed() {
		return metadata, nil
	}

	portString, err := ParseString(r.Read, b)
	if err != nil {
		return etMetadata{}, err
	}
	portText := strings.TrimSpace(string(portString.Data()))
	if portText == "" {
		return etMetadata{}, ErrETInvalidServerPort
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return etMetadata{}, ErrETInvalidServerPort
	}
	if err := validateETServerPort(port); err != nil {
		return etMetadata{}, err
	}
	metadata.ServerPort = port

	if r.Completed() {
		return metadata, nil
	}

	commandString, err := ParseString(r.Read, b)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return etMetadata{}, ErrETInvalidCommand
		}
		return etMetadata{}, err
	}
	commandText := strings.TrimSpace(string(commandString.Data()))
	if commandText == "" {
		return etMetadata{}, ErrETInvalidCommand
	}
	if err := validateETCommand(commandText); err != nil {
		return etMetadata{}, err
	}
	metadata.Command = commandText

	return metadata, nil
}

func validateETServerPort(port int) error {
	if port < 1 || port > 65535 {
		return ErrETInvalidServerPort
	}
	return nil
}

func validateETCommand(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return ErrETInvalidCommand
	}
	if len(command) > etMaxCommandLen {
		return ErrETInvalidCommand
	}
	if strings.ContainsAny(command, " \t\r\n") {
		return ErrETInvalidCommand
	}
	return nil
}
