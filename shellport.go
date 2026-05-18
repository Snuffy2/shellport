// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"

	"github.com/Snuffy2/shellport/application"
	"github.com/Snuffy2/shellport/application/commands"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/controller"
	"github.com/Snuffy2/shellport/application/log"
)

func shouldPrintVersion(args []string) bool {
	return len(args) == 2 && (args[1] == "-V" || args[1] == "--version")
}

func debugLoggingEnabled() bool {
	return len(configuration.GetEnv("SHELLPORT_DEBUG")) > 0
}

func main() {
	if shouldPrintVersion(os.Args) {
		if _, err := os.Stdout.WriteString(application.Banner()); err != nil {
			os.Exit(1)
		}
		return
	}

	configLoaders := make([]configuration.Loader, 0, 2)
	if cfgFile := configuration.GetEnv("SHELLPORT_CONFIG"); len(cfgFile) > 0 {
		configLoaders = append(configLoaders, configuration.CustomFile(cfgFile))
	} else {
		configLoaders = append(configLoaders, configuration.DefaultFile())
		configLoaders = append(configLoaders, configuration.Environ())
	}
	e := application.
		New(os.Stdout, log.NewDebugOrNonDebugWriter(
			debugLoggingEnabled(),
			application.Name,
			os.Stdout,
		)).
		Run(configuration.Redundant(configLoaders...),
			application.DefaultProccessSignallerBuilder,
			commands.New(),
			controller.Builder,
		)
	if e == nil {
		return
	}
	os.Exit(1)
}
