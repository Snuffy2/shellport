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
	defaultConfigFilePath = "/etc/shellport/shellport.conf.json"
	legacyConfigFilePath  = "/etc/shellport.conf.json"
	fileTypeName          = "File"
	defaultConfigContent  = `{
  "HostName": "",
  "SharedKey": "",
  "AdminKey": "",
  "DialTimeout": 5,
  "Socks5": "",
  "Socks5User": "",
  "Socks5Password": "",
  "Hooks": {
    "before_connecting": []
  },
  "HookTimeout": 30,
  "Servers": [
    {
      "ListenInterface": "0.0.0.0",
      "ListenPort": 8182,
      "InitialTimeout": 10,
      "ReadTimeout": 120,
      "WriteTimeout": 120,
      "HeartbeatTimeout": 15,
      "ReadDelay": 10,
      "WriteDelay": 10,
      "TLSCertificateFile": "",
      "TLSCertificateKeyFile": "",
      "ServerTitle": "",
      "ServerMessage": ""
    }
  ],
  "Presets": [],
  "OnlyAllowPresetRemotes": false
}
`
)

var environmentConfigNames = []string{
	"SHELLPORT_HOSTNAME",
	"SHELLPORT_SHAREDKEY",
	"SHELLPORT_DIALTIMEOUT",
	"SHELLPORT_SOCKS5",
	"SHELLPORT_SOCKS5_USER",
	"SHELLPORT_SOCKS5_PASSWORD",
	"SHELLPORT_HOOK_BEFORE_CONNECTING",
	"SHELLPORT_HOOKTIMEOUT",
	"SHELLPORT_LISTENINTERFACE",
	"SHELLPORT_LISTENPORT",
	"SHELLPORT_INITIALTIMEOUT",
	"SHELLPORT_READTIMEOUT",
	"SHELLPORT_WRITETIMEOUT",
	"SHELLPORT_HEARTBEATTIMEOUT",
	"SHELLPORT_READDELAY",
	"SHELLPORT_WRITEDELAY",
	"SHELLPORT_TLSCERTIFICATEFILE",
	"SHELLPORT_TLSCERTIFICATEKEYFILE",
	"SHELLPORT_SERVERTITLE",
	"SHELLPORT_SERVERMESSAGE",
	"SHELLPORT_PRESETS",
	"SHELLPORT_ONLYALLOWPRESETREMOTES",
}

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

func environmentConfigurationPresent() bool {
	for _, name := range environmentConfigNames {
		if strings.TrimSpace(GetEnv(name)) != "" {
			return true
		}
	}
	return false
}

// EnvironIfConfigured loads environment configuration only when a user supplied
// at least one environment-backed setting. Environment-only deployments should
// still win over auto-created file config, while empty environments should fall
// through to first-run file creation.
func EnvironIfConfigured() Loader {
	return func(log log.Logger) (string, Configuration, error) {
		if !environmentConfigurationPresent() {
			return environTypeName, Configuration{}, fmt.Errorf(
				"no ShellPort environment configuration was specified",
			)
		}
		return Environ()(log)
	}
}

func createDefaultConfigFile(filePath string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(defaultConfigContent); err != nil {
		return err
	}
	return nil
}

// AutoCreateDefaultFile creates and loads the default file-backed
// configuration when no configured default file or explicit environment config
// exists. A legacy default path blocks creation so an upgrade cannot silently
// replace a previously secured file-backed deployment with a blank generated
// config.
func AutoCreateDefaultFile(filePath string, legacyPath string) Loader {
	return func(log log.Logger) (string, Configuration, error) {
		if fileInfo, err := os.Stat(legacyPath); err == nil && !fileInfo.IsDir() {
			return fileTypeName, Configuration{}, fmt.Errorf(
				"legacy configuration file %q exists; move it to %q or set SHELLPORT_CONFIG",
				legacyPath,
				filePath,
			)
		}
		log.Info("No default configuration file was found; creating %s", filePath)
		if err := createDefaultConfigFile(filePath); err != nil {
			return fileTypeName, Configuration{}, fmt.Errorf(
				"configuration file was not specified and no fallback files "+
					"were available; also failed to create %q: %w",
				filePath,
				err,
			)
		}
		return loadFile(filePath)
	}
}

func defaultFileSearchList(homeDir string, executablePath string) []string {
	fallbackFileSearchList := make([]string, 0, 4)

	// /etc/shellport/shellport.conf.json
	fallbackFileSearchList = append(
		fallbackFileSearchList,
		defaultConfigFilePath,
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

func defaultFileFromSearchList(fallbackFileSearchList []string) Loader {
	return func(log log.Logger) (string, Configuration, error) {
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
			"configuration file was not specified; also tried fallback files "+
				"\"%s\", but none of them was available",
			strings.Join(fallbackFileSearchList, "\", \""))
	}
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
		return defaultFileFromSearchList(fallbackFileSearchList)(log)
	}
}
