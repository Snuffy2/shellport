// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package configuration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Snuffy2/shellport/application/log"
	"gopkg.in/yaml.v3"
)

// fileTypeName is the loader name reported when configuration is loaded from a
// YAML file.
const (
	// DefaultConfigFilePath is the default Docker config file path.
	DefaultConfigFilePath = "/config/shellport.conf.yml"
	fileTypeName          = "File"
	defaultConfigContent  = `HostName: ""
UserPassword: ""
AdminPassword: ""
DialTimeout: 5
Socks5: ""
Socks5User: ""
Socks5Password: ""
Hooks:
  before_connecting: []
HookTimeout: 30
Servers:
  - ListenInterface: 0.0.0.0
    ListenPort: 8182
    InitialTimeout: 10
    ReadTimeout: 120
    WriteTimeout: 120
    HeartbeatTimeout: 15
    ReadDelay: 10
    WriteDelay: 10
    TLSCertificateFile: ""
    TLSCertificateKeyFile: ""
    ServerTitle: ""
    ServerMessage: ""
Presets: []
OnlyAllowPresetRemotes: false
`
)

// loadFile opens filePath, YAML-decodes it into a commonInput, and returns the
// resulting Configuration. It returns the fileTypeName string along with the
// configuration or the first error encountered.
func loadFile(filePath string) (string, Configuration, error) {
	data, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return fileTypeName, Configuration{}, readErr
	}
	raw, err := decodeYAMLMap(data)
	if err != nil {
		return fileTypeName, Configuration{}, err
	}
	if err := rejectFilePresetSecretKey(raw); err != nil {
		return fileTypeName, Configuration{}, err
	}
	cfg, err := commonInputFromYAMLMap(raw)
	if err != nil {
		return fileTypeName, Configuration{}, err
	}
	finalCfg, err := cfg.concretize()
	finalCfg.SourceFile = filePath
	return fileTypeName, finalCfg, err
}

func decodeYAMLMap(data []byte) (map[string]any, error) {
	raw := map[string]any{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	normalized := normalizeYAMLMap(raw)
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	preserveYAMLMetaScalarText(normalized, &doc)
	return normalized, nil
}

var yamlStringFieldNames = map[string]struct{}{
	"AdminPassword":         {},
	"Host":                  {},
	"HostName":              {},
	"ID":                    {},
	"ListenInterface":       {},
	"ServerMessage":         {},
	"ServerTitle":           {},
	"Socks5":                {},
	"Socks5Password":        {},
	"Socks5User":            {},
	"TLSCertificateFile":    {},
	"TLSCertificateKeyFile": {},
	"TabColor":              {},
	"Title":                 {},
	"Type":                  {},
	"UserPassword":          {},
}

func commonInputFromYAMLMap(raw map[string]any) (commonInput, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return commonInput{}, err
	}
	cfg := commonInput{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return commonInput{}, err
	}
	return cfg, nil
}

func normalizeYAMLMap(raw map[string]any) map[string]any {
	normalized := make(map[string]any, len(raw))
	for key, value := range raw {
		if key == "Meta" {
			normalized[key] = normalizeYAMLMeta(value)
			continue
		}
		normalized[key] = normalizeYAMLValue(value)
	}
	return normalized
}

func normalizeYAMLMeta(value any) any {
	typed, ok := value.(map[string]any)
	if !ok {
		return normalizeYAMLValue(value)
	}
	normalized := make(map[string]any, len(typed))
	for key, metaValue := range typed {
		normalized[key] = normalizeYAMLMetaValue(metaValue)
	}
	return normalized
}

func normalizeYAMLMetaValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case map[string]any, []any:
		return normalizeYAMLValue(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func preserveYAMLMetaScalarText(value any, node *yaml.Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) > 0 {
			preserveYAMLMetaScalarText(value, node.Content[0])
		}
	case yaml.MappingNode:
		typed, ok := value.(map[string]any)
		if !ok {
			return
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value != "<<" {
				continue
			}
			typed = yamlMergedMappingScalarText(node.Content[i+1], typed)
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if key == "<<" {
				continue
			}
			child := node.Content[i+1]
			if key == "Meta" {
				typed[key] = yamlMetaScalarText(child, typed[key])
				continue
			}
			if _, ok := yamlStringFieldNames[key]; ok {
				typed[key] = yamlStringFieldValue(child, typed[key])
				continue
			}
			preserveYAMLMetaScalarText(typed[key], child)
		}
	case yaml.SequenceNode:
		typed, ok := value.([]any)
		if !ok {
			return
		}
		for i, child := range node.Content {
			if i >= len(typed) {
				return
			}
			preserveYAMLMetaScalarText(typed[i], child)
		}
	}
}

func yamlMergedMappingScalarText(node *yaml.Node, fallback map[string]any) map[string]any {
	if node == nil {
		return fallback
	}
	switch node.Kind {
	case yaml.AliasNode:
		return yamlMappingScalarText(node.Alias, fallback)
	case yaml.MappingNode:
		return yamlMappingScalarText(node, fallback)
	case yaml.SequenceNode:
		for i := len(node.Content) - 1; i >= 0; i-- {
			fallback = yamlMergedMappingScalarText(node.Content[i], fallback)
		}
		return fallback
	default:
		return fallback
	}
}

func yamlMappingScalarText(node *yaml.Node, fallback map[string]any) map[string]any {
	if node == nil || node.Kind != yaml.MappingNode {
		return fallback
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if key == "<<" {
			continue
		}
		child := node.Content[i+1]
		if key == "Meta" {
			fallback[key] = yamlMetaScalarText(child, fallback[key])
			continue
		}
		if _, ok := yamlStringFieldNames[key]; ok {
			fallback[key] = yamlStringFieldValue(child, fallback[key])
		}
	}
	return fallback
}

func yamlMetaScalarText(node *yaml.Node, fallback any) any {
	if node == nil {
		return fallback
	}
	switch node.Kind {
	case yaml.MappingNode:
		typed, ok := fallback.(map[string]any)
		if !ok {
			return fallback
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value != "<<" {
				continue
			}
			typed = yamlMetaMergedScalarText(node.Content[i+1], typed)
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if key == "<<" {
				continue
			}
			valueNode := node.Content[i+1]
			if valueNode.Kind == yaml.ScalarNode {
				typed[key] = yamlScalarValue(valueNode, typed[key])
				continue
			}
			typed[key] = yamlMetaScalarText(valueNode, typed[key])
		}
		return typed
	case yaml.AliasNode:
		return yamlMetaScalarText(node.Alias, fallback)
	case yaml.SequenceNode:
		typed, ok := fallback.([]any)
		if !ok {
			return fallback
		}
		for i, child := range node.Content {
			if i >= len(typed) {
				return typed
			}
			if child.Kind == yaml.ScalarNode {
				typed[i] = yamlScalarValue(child, typed[i])
				continue
			}
			typed[i] = yamlMetaScalarText(child, typed[i])
		}
		return typed
	case yaml.ScalarNode:
		return yamlScalarValue(node, fallback)
	default:
		return fallback
	}
}

func yamlStringFieldValue(node *yaml.Node, fallback any) any {
	if node == nil {
		return fallback
	}
	if node.Kind == yaml.AliasNode {
		return yamlStringFieldValue(node.Alias, fallback)
	}
	if node.Kind == yaml.ScalarNode {
		return yamlScalarValue(node, fallback)
	}
	return fallback
}

func yamlMetaMergedScalarText(node *yaml.Node, fallback map[string]any) map[string]any {
	if node == nil {
		return fallback
	}
	switch node.Kind {
	case yaml.AliasNode:
		merged, ok := yamlMetaScalarText(node.Alias, fallback).(map[string]any)
		if !ok {
			return fallback
		}
		return merged
	case yaml.MappingNode:
		merged, ok := yamlMetaScalarText(node, fallback).(map[string]any)
		if !ok {
			return fallback
		}
		return merged
	case yaml.SequenceNode:
		for i := len(node.Content) - 1; i >= 0; i-- {
			fallback = yamlMetaMergedScalarText(node.Content[i], fallback)
		}
		return fallback
	default:
		return fallback
	}
}

func yamlScalarValue(node *yaml.Node, fallback any) any {
	if node.Tag == "!!null" {
		return ""
	}
	return node.Value
}

func normalizeYAMLValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return normalizeYAMLMap(typed)
	case []any:
		normalized := make([]any, len(typed))
		for i, item := range typed {
			normalized[i] = normalizeYAMLValue(item)
		}
		return normalized
	default:
		return value
	}
}

func rejectFilePresetSecretKey(raw map[string]any) error {
	if _, ok := raw["PresetSecretKey"]; ok {
		return fmt.Errorf("%s must be set as an environment variable, not in YAML config", PresetSecretKeyEnv)
	}
	if _, ok := raw[PresetSecretKeyEnv]; ok {
		return fmt.Errorf("%s must be set as an environment variable, not in YAML config", PresetSecretKeyEnv)
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
// configuration when no configured default file exists.
func AutoCreateDefaultFile(filePath string) Loader {
	return func(log log.Logger) (string, Configuration, error) {
		log.Info("No default configuration file was found; creating %s", filePath)
		if err := createDefaultConfigFile(filePath); err != nil {
			if os.IsExist(err) {
				return loadFile(filePath)
			}
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

func defaultFileSearchList() []string {
	return []string{DefaultConfigFilePath}
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
		log.Info("Loading configuration from the default configuration file")
		return defaultFileFromSearchList(defaultFileSearchList())(log)
	}
}
