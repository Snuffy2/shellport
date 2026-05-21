// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package application

import "fmt"

// Plate information contains static identity strings for the application.
const (
	// Name is the short application name.
	Name = "ShellPort"
	// FullName is the human-readable full application name.
	FullName = "ShellPort — browser-based remote shell access over SSH, Telnet, ET, and Mosh"
	// Author identifies the fork maintainer.
	Author = "Snuffy2"
	// URL is the canonical project URL.
	URL = "https://github.com/Snuffy2/shellport"
)

// banner is the startup message template printed to the screen on launch.
// Positional arguments: FullName, version, Author, URL.
const (
	banner = "\r\n %s %s\r\n\r\n Copyright (C) %s\r\n %s\r\n\r\n"
)

// version holds the current build version string, injected at link time.
// It defaults to "dev" when no version is provided by the build system.
var (
	version = "dev"
)

// Banner returns the startup/version banner shown by the application.
func Banner() string {
	return fmt.Sprintf(banner, FullName, version, Author, URL)
}
