// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import "github.com/Snuffy2/shellport/application/command"

// New creates and returns the fully populated command.Commands array with
// Telnet at index 0, SSH at index 1, Mosh at index 2, and ET at index 3,
// ready to be passed to a Commander.
func New() command.Commands {
	return command.Commands{
		command.Register("Telnet", newTelnet, parseTelnetConfig),
		command.Register("SSH", newSSH, parseSSHConfig),
		command.Register("Mosh", newMosh, parseMoshConfig),
		command.Register("ET", newET, parseETConfig),
	}
}
