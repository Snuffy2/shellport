// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

// Package application provides the top-level application lifecycle management
// for ShellPort, including startup, signal handling, configuration loading, and
// graceful shutdown with optional restart on SIGHUP.
package application

import (
	"fmt"
	"io"
	goLog "log"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"sync"
	"syscall"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
	"github.com/Snuffy2/shellport/application/server"
)

// ProccessSignaller is a channel used to send OS signals to the running
// application, triggering shutdown or restart behavior.
type ProccessSignaller chan os.Signal

// ProcessSignallerBuilder is a factory function that returns a new OS signal
// channel. Callers may substitute a custom builder for testing purposes.
type ProcessSignallerBuilder func() chan os.Signal

// DefaultProccessSignallerBuilder returns the default ProcessSignallerBuilder,
// which creates a buffered channel of size 1 for receiving OS signals.
func DefaultProccessSignallerBuilder() chan os.Signal {
	return make(chan os.Signal, 1)
}

var (
	// screenLineWipper is written to the screen output to clear the current
	// line before printing shutdown messages (e.g., to erase the ^C glyph).
	screenLineWipper = []byte("\r")
)

// Application holds the dependencies required to run the ShellPort application,
// including the output writer for banner/status messages and the structured
// logger used throughout the lifetime of the process.
type Application struct {
	// screen is the writer used for banner output and user-facing status lines.
	screen io.Writer
	// logger is the structured logger routed to the output writer.
	logger log.Logger
}

// New creates a new Application with the given screen writer and logger.
func New(screen io.Writer, logger log.Logger) Application {
	return Application{
		screen: screen,
		logger: logger,
	}
}

// run performs a single execution cycle: loads configuration, starts all
// configured servers, and blocks until an OS signal arrives. It returns
// (true, nil) when a SIGHUP is received, indicating that the caller should
// restart; (false, nil) for clean shutdown signals; and (false, err) when an
// error forces an early exit.
func (a Application) run(
	cLoader configuration.Loader,
	closeSigBuilder ProcessSignallerBuilder,
	commands command.Commands,
	handlerBuilder server.HandlerBuilderBuilder,
) (bool, error) {
	var err error

	loaderName, c, cErr := cLoader(a.logger.TitledContext("Configuration"))

	if cErr != nil {
		a.logger.Error("\"%s\" loader cannot load configuration: %s",
			loaderName, cErr)

		return false, cErr
	}

	c, err = normalizeStartupPresets(c, commands)
	if err != nil {
		a.logger.Error("Unable to normalize presets: %s", err)

		return false, err
	}

	// Verify all configuration
	err = c.Verify()

	if err != nil {
		a.logger.Error("Configuration was invalid: %s", err)

		return false, err
	}

	closeNotify := closeSigBuilder()
	closeNotifyDisableLock := sync.Mutex{}
	signal.Notify(closeNotify, os.Kill, os.Interrupt, syscall.SIGHUP)
	defer func() {
		closeNotifyDisableLock.Lock()
		defer closeNotifyDisableLock.Unlock()
		if closeNotify == nil {
			return
		}
		signal.Stop(closeNotify)
		close(closeNotify)
		closeNotify = nil
	}()

	servers := make([]*server.Serving, 0, len(c.Servers))
	s := server.New(a.logger)
	commonCfg := c.Common()
	warnIfPresetConfigNotWritable(commonCfg, a.logger)

	defer func() {
		for i := len(servers); i > 0; i-- {
			servers[i-1].Close()
		}
		s.Wait()
	}()

	for _, ss := range c.Servers {
		newServer := s.Serve(commonCfg, ss, func(e error) {
			closeNotifyDisableLock.Lock()
			defer closeNotifyDisableLock.Unlock()
			if closeNotify == nil {
				return
			}
			err = e
			signal.Stop(closeNotify)
			close(closeNotify)
			closeNotify = nil
		}, handlerBuilder(commands))
		servers = append(servers, newServer)
	}

	switch <-closeNotify {
	case syscall.SIGHUP:
		return true, nil
	case syscall.SIGTERM:
		fallthrough
	case os.Kill:
		fallthrough
	case os.Interrupt:
		a.screen.Write(screenLineWipper)
		return false, nil
	default:
		closeNotifyDisableLock.Lock()
		defer closeNotifyDisableLock.Unlock()
		return false, err
	}
}

// normalizeStartupPresets ensures loaded presets have stable IDs and secrets.
//
// File-backed configurations are rewritten when IDs are generated so future
// API edits can target stable identifiers. Duplicate IDs are returned as
// configuration errors.
func normalizeStartupPresets(
	c configuration.Configuration,
	commands command.Commands,
) (configuration.Configuration, error) {
	presets, changed, err := configuration.EnsurePresetIDs(c.Presets)
	if err != nil {
		return configuration.Configuration{}, err
	}
	c.Presets = presets
	if changed && configuration.PresetConfigWritable(c.SourceFile) {
		if err := configuration.PersistPresetIDs(c.SourceFile, presets); err != nil {
			return configuration.Configuration{}, err
		}
	}

	presets, err = commands.Reconfigure(c.Presets)
	if err != nil {
		return configuration.Configuration{}, err
	}
	reconfigureChanged := !reflect.DeepEqual(c.Presets, presets)
	runtimePresets := presets

	presets, secretsChanged, err := configuration.ApplyPresetSecrets(presets)
	if err != nil {
		return configuration.Configuration{}, err
	}
	c.Presets = presets
	if reconfigureChanged || secretsChanged {
		if !configuration.PresetConfigWritable(c.SourceFile) {
			if secretsChanged && c.SourceFile != "" {
				return configuration.Configuration{}, fmt.Errorf(
					"preset password migration requires a writable file-backed configuration",
				)
			}
			return c, nil
		}
		if err := configuration.ReplaceFilePresetsWithRuntime(
			c.SourceFile,
			presets,
			runtimePresets,
			nil,
		); err != nil {
			return configuration.Configuration{}, err
		}
	}
	return c, nil
}

// warnIfPresetConfigNotWritable logs when file-backed preset management cannot
// persist changes.
func warnIfPresetConfigNotWritable(
	commonCfg configuration.Common,
	logger log.Logger,
) {
	if commonCfg.SourceFile == "" || commonCfg.PresetConfigWritable() {
		return
	}
	logger.Warning(
		"Preset config file is not writable; UI preset create/edit/delete is disabled: %s",
		commonCfg.SourceFile,
	)
}

// Run executes the application loop. It prints the startup banner, redirects
// the standard library logger to the application logger, then repeatedly calls
// run until a non-restart signal is received or a fatal error occurs. It
// returns the first non-nil error encountered, or nil on clean exit.
func (a Application) Run(
	cLoader configuration.Loader,
	closeSigBuilder ProcessSignallerBuilder,
	commands command.Commands,
	handlerBuilder server.HandlerBuilderBuilder,
) error {
	fmt.Fprint(a.screen, Banner())
	goLog.SetOutput(a.logger)
	defer goLog.SetOutput(os.Stderr)
	a.logger.Info("Initializing")
	defer a.logger.Info("Closed")
	a.logger.Debug(
		"Runtime: %s. GOOS=%s, GOARCH=%s",
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
	)
	for {
		restart, runErr := a.run(
			cLoader, closeSigBuilder, commands, handlerBuilder)
		if runErr != nil {
			a.logger.Error("Unable to start due to error: %s", runErr)
			return runErr
		}
		if restart {
			a.logger.Info("Restarting")
			continue
		}
		return nil
	}
}
