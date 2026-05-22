// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package configuration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/Snuffy2/shellport/application/log"
)

// fileTypeName is the loader name reported when configuration is loaded from a
// JSON file.
const (
	fileTypeName = "File"
)

// loadFile opens filePath, JSON-decodes it into a commonInput, and returns the
// resulting Configuration. It returns the fileTypeName string along with the
// configuration or the first error encountered.
func loadFile(filePath string) (string, Configuration, error) {
	f, fErr := os.Open(filePath)
	if fErr != nil {
		return fileTypeName, Configuration{}, fErr
	}
	defer f.Close()
	cfg := commonInput{}
	jDecoder := json.NewDecoder(f)
	raw := map[string]json.RawMessage{}
	if jDecodeErr := jDecoder.Decode(&raw); jDecodeErr != nil {
		return fileTypeName, Configuration{}, jDecodeErr
	}
	if err := rejectFilePresetSecretKey(raw); err != nil {
		return fileTypeName, Configuration{}, err
	}
	data, marshalErr := json.Marshal(raw)
	if marshalErr != nil {
		return fileTypeName, Configuration{}, marshalErr
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fileTypeName, Configuration{}, err
	}
	finalCfg, err := cfg.concretize()
	if adminKey := GetEnv("SHELLPORT_ADMIN_KEY"); adminKey != "" {
		finalCfg.AdminKey = adminKey
	}
	finalCfg.SourceFile = filePath
	return fileTypeName, finalCfg, err
}

func rejectFilePresetSecretKey(raw map[string]json.RawMessage) error {
	if _, ok := raw["PresetSecretKey"]; ok {
		return fmt.Errorf("%s must be set as an environment variable, not in JSON config", PresetSecretKeyEnv)
	}
	if _, ok := raw[PresetSecretKeyEnv]; ok {
		return fmt.Errorf("%s must be set as an environment variable, not in JSON config", PresetSecretKeyEnv)
	}
	return nil
}

// CustomFile creates a configuration file loader that loads configuration from
// the specified file path
func CustomFile(customPath string) Loader {
	return func(log log.Logger) (string, Configuration, error) {
		log.Info("Loading configuration from: %s", customPath)
		return loadFile(customPath)
	}
}

func defaultFileSearchList(homeDir string, executablePath string) []string {
	fallbackFileSearchList := make([]string, 0, 4)

	// /etc/shellport/shellport.conf.json
	fallbackFileSearchList = append(
		fallbackFileSearchList,
		filepath.Join("/", "etc", "shellport", "shellport.conf.json"),
	)

	// ~/.config/shellport.conf.json
	if homeDir != "" {
		fallbackFileSearchList = append(
			fallbackFileSearchList,
			filepath.Join(homeDir, ".config", "shellport.conf.json"))
	}

	// shellport.conf.json located at the same directory as ShellPort bin
	if executablePath != "" {
		fallbackFileSearchList = append(
			fallbackFileSearchList,
			filepath.Join(filepath.Dir(executablePath), "shellport.conf.json"))
	}

	return fallbackFileSearchList
}

// DefaultFile creates a configuration file loader that loads configuration from
// one of the default file path
func DefaultFile() Loader {
	return func(log log.Logger) (string, Configuration, error) {
		log.Info("Loading configuration from one of the default " +
			"configuration files ...")
		homeDir := ""
		if u, userErr := user.Current(); userErr == nil {
			homeDir = u.HomeDir
		}

		executablePath := ""
		if ex, exErr := os.Executable(); exErr == nil {
			executablePath = ex
		}

		fallbackFileSearchList := defaultFileSearchList(homeDir, executablePath)

		// Search given locations to select the config file
		for f := range fallbackFileSearchList {
			if fInfo, fErr := os.Stat(fallbackFileSearchList[f]); fErr != nil {
				continue
			} else if fInfo.IsDir() {
				continue
			} else {
				log.Info("Configuration file \"%s\" has been selected",
					fallbackFileSearchList[f])
				return loadFile(fallbackFileSearchList[f])
			}
		}
		return fileTypeName, Configuration{}, fmt.Errorf(
			"Configuration file was not specified. Also tried fallback files "+
				"\"%s\", but none of them was available",
			strings.Join(fallbackFileSearchList, "\", \""))
	}
}
