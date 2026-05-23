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
	preserveYAMLConfigScalarText(normalized, &doc)
	return normalized, nil
}

var yamlCommonStringFieldNames = map[string]struct{}{
	"adminpassword":  {},
	"hostname":       {},
	"socks5":         {},
	"socks5password": {},
	"socks5user":     {},
	"userpassword":   {},
}

var yamlServerStringFieldNames = map[string]struct{}{
	"listeninterface":       {},
	"servermessage":         {},
	"servertitle":           {},
	"tlscertificatefile":    {},
	"tlscertificatekeyfile": {},
}

var yamlPresetStringFieldNames = map[string]struct{}{
	"host":     {},
	"id":       {},
	"tabcolor": {},
	"title":    {},
	"type":     {},
}

func isYAMLStringFieldName(key string, fields map[string]struct{}) bool {
	_, ok := fields[strings.ToLower(key)]
	return ok
}

func isYAMLMetaFieldName(key string) bool {
	return strings.EqualFold(key, "Meta")
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

func preserveYAMLConfigScalarText(value any, node *yaml.Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) > 0 {
			preserveYAMLConfigScalarText(value, node.Content[0])
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
			typed = yamlMergedMappingScalarText(
				node.Content[i+1],
				typed,
				yamlCommonStringFieldNames,
			)
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if key == "<<" {
				continue
			}
			child := node.Content[i+1]
			if strings.EqualFold(key, "Servers") {
				preserveYAMLServerScalarText(typed[key], child)
				continue
			}
			if strings.EqualFold(key, "Presets") {
				preserveYAMLPresetScalarText(typed[key], child)
				continue
			}
			if strings.EqualFold(key, "Hooks") {
				preserveYAMLHookScalarText(typed[key], child)
				continue
			}
			if isYAMLStringFieldName(key, yamlCommonStringFieldNames) {
				typed[key] = yamlStringFieldValue(child, typed[key])
			}
		}
	}
}

func preserveYAMLServerScalarText(value any, node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.AliasNode {
		preserveYAMLServerScalarText(value, node.Alias)
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		preserveYAMLMappingScalarText(value, node, yamlServerStringFieldNames)
	case yaml.SequenceNode:
		typed, ok := value.([]any)
		if !ok {
			return
		}
		for i, child := range node.Content {
			if i >= len(typed) {
				return
			}
			preserveYAMLServerScalarText(typed[i], child)
		}
	}
}

func preserveYAMLPresetScalarText(value any, node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.AliasNode {
		preserveYAMLPresetScalarText(value, node.Alias)
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		typed, ok := value.(map[string]any)
		if !ok {
			return
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value != "<<" {
				continue
			}
			typed = yamlMergedMappingScalarText(
				node.Content[i+1],
				typed,
				yamlPresetStringFieldNames,
			)
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if key == "<<" {
				continue
			}
			child := node.Content[i+1]
			if isYAMLMetaFieldName(key) {
				typed[key] = yamlMetaScalarText(child, typed[key])
				continue
			}
			if isYAMLStringFieldName(key, yamlPresetStringFieldNames) {
				typed[key] = yamlStringFieldValue(child, typed[key])
			}
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
			preserveYAMLPresetScalarText(typed[i], child)
		}
	}
}

func preserveYAMLHookScalarText(value any, node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.AliasNode {
		preserveYAMLHookScalarText(value, node.Alias)
		return
	}
	typed, ok := value.(map[string]any)
	if !ok || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "<<" {
			continue
		}
		key := node.Content[i].Value
		typed[key] = yamlHookCommandScalarText(node.Content[i+1], typed[key])
	}
}

func yamlHookCommandScalarText(node *yaml.Node, base any) any {
	if node == nil {
		return base
	}
	if node.Kind == yaml.AliasNode {
		return yamlHookCommandScalarText(node.Alias, base)
	}
	commands, ok := base.([]any)
	if !ok || node.Kind != yaml.SequenceNode {
		return base
	}
	for i, commandNode := range node.Content {
		if i >= len(commands) {
			return commands
		}
		command, ok := commands[i].([]any)
		if !ok {
			continue
		}
		if commandNode.Kind == yaml.AliasNode {
			commandNode = commandNode.Alias
		}
		if commandNode.Kind != yaml.SequenceNode {
			continue
		}
		for j, argNode := range commandNode.Content {
			if j >= len(command) {
				break
			}
			if argNode.Kind == yaml.AliasNode {
				argNode = argNode.Alias
			}
			if argNode.Kind == yaml.ScalarNode {
				command[j] = yamlScalarValue(argNode, command[j])
			}
		}
	}
	return commands
}

func yamlMergedMappingScalarText(
	node *yaml.Node,
	base map[string]any,
	fields map[string]struct{},
) map[string]any {
	if node == nil {
		return base
	}
	switch node.Kind {
	case yaml.AliasNode:
		return yamlMappingScalarText(node.Alias, base, fields)
	case yaml.MappingNode:
		return yamlMappingScalarText(node, base, fields)
	case yaml.SequenceNode:
		for i := len(node.Content) - 1; i >= 0; i-- {
			base = yamlMergedMappingScalarText(node.Content[i], base, fields)
		}
		return base
	default:
		return base
	}
}

func preserveYAMLMappingScalarText(
	value any,
	node *yaml.Node,
	fields map[string]struct{},
) {
	typed, ok := value.(map[string]any)
	if !ok {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != "<<" {
			continue
		}
		typed = yamlMergedMappingScalarText(node.Content[i+1], typed, fields)
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if key == "<<" {
			continue
		}
		if isYAMLStringFieldName(key, fields) {
			typed[key] = yamlStringFieldValue(node.Content[i+1], typed[key])
		}
	}
}

func yamlMappingScalarText(
	node *yaml.Node,
	base map[string]any,
	fields map[string]struct{},
) map[string]any {
	if node == nil || node.Kind != yaml.MappingNode {
		return base
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if key == "<<" {
			continue
		}
		child := node.Content[i+1]
		if isYAMLStringFieldName(key, fields) {
			base[key] = yamlStringFieldValue(child, base[key])
		}
	}
	return base
}

func yamlMetaScalarText(node *yaml.Node, base any) any {
	if node == nil {
		return base
	}
	switch node.Kind {
	case yaml.MappingNode:
		typed, ok := base.(map[string]any)
		if !ok {
			return base
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
		return yamlMetaScalarText(node.Alias, base)
	case yaml.SequenceNode:
		typed, ok := base.([]any)
		if !ok {
			return base
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
		return yamlScalarValue(node, base)
	default:
		return base
	}
}

func yamlStringFieldValue(node *yaml.Node, base any) any {
	if node == nil {
		return base
	}
	if node.Kind == yaml.AliasNode {
		return yamlStringFieldValue(node.Alias, base)
	}
	if node.Kind == yaml.ScalarNode {
		return yamlScalarValue(node, base)
	}
	return base
}

func yamlMetaMergedScalarText(node *yaml.Node, base map[string]any) map[string]any {
	if node == nil {
		return base
	}
	switch node.Kind {
	case yaml.AliasNode:
		merged, ok := yamlMetaScalarText(node.Alias, base).(map[string]any)
		if !ok {
			return base
		}
		return merged
	case yaml.MappingNode:
		merged, ok := yamlMetaScalarText(node, base).(map[string]any)
		if !ok {
			return base
		}
		return merged
	case yaml.SequenceNode:
		for i := len(node.Content) - 1; i >= 0; i-- {
			base = yamlMetaMergedScalarText(node.Content[i], base)
		}
		return base
	default:
		return base
	}
}

func yamlScalarValue(node *yaml.Node, base any) any {
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
				"configuration file was not specified and no default files "+
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

func defaultFileFromSearchList(defaultFiles []string) Loader {
	return func(log log.Logger) (string, Configuration, error) {
		for f := range defaultFiles {
			if fInfo, fErr := os.Stat(defaultFiles[f]); fErr != nil {
				continue
			} else if fInfo.IsDir() {
				continue
			} else {
				log.Info("Configuration file \"%s\" has been selected",
					defaultFiles[f])
				return loadFile(defaultFiles[f])
			}
		}
		return fileTypeName, Configuration{}, fmt.Errorf(
			"configuration file was not specified; also tried default files "+
				"\"%s\", but none of them was available",
			strings.Join(defaultFiles, "\", \""))
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
