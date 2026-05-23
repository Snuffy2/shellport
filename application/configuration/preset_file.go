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

	"github.com/natefinch/atomic"
	"gopkg.in/yaml.v3"
)

// PersistPresetIDs writes generated preset IDs back to a YAML configuration file.
//
// It preserves the existing preset order and only updates ID fields. The caller
// must pass the same number of presets that were loaded from filePath.
func PersistPresetIDs(filePath string, presets []Preset) error {
	if filePath == "" {
		return nil
	}

	resolvedPath, err := resolveConfigFilePath(filePath)
	if err != nil {
		return err
	}
	doc, err := readCommonInputFileDocument(resolvedPath)
	if err != nil {
		return err
	}
	if len(doc.input.Presets) != len(presets) {
		return fmt.Errorf(
			"cannot persist preset IDs: file has %d presets, runtime has %d",
			len(doc.input.Presets),
			len(presets),
		)
	}
	for i := range doc.input.Presets {
		doc.input.Presets[i].ID = presets[i].ID
	}
	return writeCommonInputFileDocument(resolvedPath, doc)
}

// ReplaceFilePresets atomically updates the Presets list in a YAML config file.
func ReplaceFilePresets(filePath string, presets []Preset) error {
	return replaceFilePresets(filePath, presets, nil, nil, nil)
}

// ReplaceFilePresetsWithRuntime atomically updates a YAML config file using
// runtimePresets to distinguish deleted presets from raw entries the runtime
// did not understand.
func ReplaceFilePresetsWithRuntime(
	filePath string,
	presets []Preset,
	runtimePresets []Preset,
	clearPresetPasswordPresetIDs map[string]struct{},
) error {
	return ReplaceFilePresetsWithRuntimeSecrets(
		filePath,
		presets,
		runtimePresets,
		clearPresetPasswordPresetIDs,
		nil,
	)
}

func ReplaceFilePresetsWithRuntimeSecrets(
	filePath string,
	presets []Preset,
	runtimePresets []Preset,
	clearPresetPasswordPresetIDs map[string]struct{},
	clearPresetPrivateKeyPresetIDs map[string]struct{},
) error {
	return replaceFilePresets(
		filePath,
		presets,
		runtimePresets,
		clearPresetPasswordPresetIDs,
		clearPresetPrivateKeyPresetIDs,
	)
}

func replaceFilePresets(
	filePath string,
	presets []Preset,
	runtimePresets []Preset,
	clearPresetPasswordPresetIDs map[string]struct{},
	clearPresetPrivateKeyPresetIDs map[string]struct{},
) error {
	if filePath == "" {
		return fmt.Errorf("preset config updates require a file-backed configuration")
	}

	resolvedPath, err := resolveConfigFilePath(filePath)
	if err != nil {
		return err
	}
	doc, err := readCommonInputFileDocument(resolvedPath)
	if err != nil {
		return err
	}
	concrete := runtimePresets
	if concrete == nil {
		var concreteErr error
		concrete, concreteErr = doc.input.Presets.concretize()
		if concreteErr != nil {
			return concreteErr
		}
	}
	doc.input.Presets = mergePresetInputs(
		doc.input.Presets,
		concrete,
		presets,
		runtimePresets,
		clearPresetPasswordPresetIDs,
		clearPresetPrivateKeyPresetIDs,
	)
	return writeCommonInputFileDocument(resolvedPath, doc)
}

// PresetConfigWritable reports whether filePath points to a writable config file.
func PresetConfigWritable(filePath string) bool {
	if filePath == "" {
		return false
	}
	resolvedPath, resolveErr := resolveConfigFilePath(filePath)
	if resolveErr != nil {
		return false
	}
	f, err := os.OpenFile(resolvedPath, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	if closeErr := f.Close(); closeErr != nil {
		return false
	}
	tmp, createErr := os.CreateTemp(
		filepath.Dir(resolvedPath),
		filepath.Base(resolvedPath)+".writable.*.tmp",
	)
	if createErr != nil {
		return false
	}
	tmpName := tmp.Name()
	if closeErr := tmp.Close(); closeErr != nil {
		_ = os.Remove(tmpName)
		return false
	}
	return os.Remove(tmpName) == nil
}

func resolveConfigFilePath(filePath string) (string, error) {
	resolvedPath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return "", err
	}
	return resolvedPath, nil
}

// presetInputsFromPresets converts normalized presets back to file input shape.
func presetInputsFromPresets(presets []Preset) presetInputs {
	inputs := make(presetInputs, len(presets))
	for i, preset := range presets {
		inputs[i] = presetInputFromPreset(preset)
	}
	return inputs
}

func presetInputFromPreset(preset Preset) presetInput {
	return presetInput{
		ID:       preset.ID,
		Title:    preset.Title,
		Type:     preset.Type,
		Host:     preset.Host,
		TabColor: preset.TabColor,
		Meta:     metaInputFromPreset(preset.Meta),
	}
}

func mergePresetInputs(
	raw presetInputs,
	concrete []Preset,
	presets []Preset,
	runtimePresets []Preset,
	clearPresetPasswordPresetIDs map[string]struct{},
	clearPresetPrivateKeyPresetIDs map[string]struct{},
) presetInputs {
	rawByID := presetInputIndexByID(raw)
	concreteByID := presetMapByID(concrete)
	runtimeByID := presetMapByID(runtimePresets)
	merged := make(presetInputs, 0, safePresetInputCapacity(len(raw), len(presets)))
	touched := make(map[string]struct{}, len(presets))

	for _, preset := range presets {
		id := strings.TrimSpace(preset.ID)
		touched[id] = struct{}{}
		rawIndex, rawOK := rawByID[id]
		current, currentOK := concreteByID[id]
		if rawOK && currentOK {
			clearPresetPassword := false
			if _, ok := clearPresetPasswordPresetIDs[id]; ok {
				clearPresetPassword = true
			}
			clearPresetPrivateKey := false
			if _, ok := clearPresetPrivateKeyPresetIDs[id]; ok {
				clearPresetPrivateKey = true
			}
			merged = append(
				merged,
				mergePresetInput(
					raw[rawIndex],
					current,
					preset,
					clearPresetPassword,
					clearPresetPrivateKey,
				),
			)
			continue
		}
		merged = append(merged, presetInputFromPreset(preset))
	}

	for _, input := range raw {
		id := strings.TrimSpace(input.ID)
		if _, ok := touched[id]; ok {
			continue
		}
		if len(runtimeByID) > 0 {
			if _, ok := runtimeByID[id]; ok {
				continue
			}
		}
		merged = append(merged, input)
	}

	return merged
}

func safePresetInputCapacity(rawLen int, presetLen int) int {
	maxInt := int(^uint(0) >> 1)
	if rawLen > maxInt-presetLen {
		return 0
	}
	return rawLen + presetLen
}

func mergePresetInput(
	raw presetInput,
	current Preset,
	preset Preset,
	clearPresetPassword bool,
	clearPresetPrivateKey bool,
) presetInput {
	merged := raw
	merged.ID = preset.ID
	merged.Title = preserveRawString(raw.Title, current.Title, preset.Title)
	merged.Type = preserveRawString(raw.Type, current.Type, preset.Type)
	merged.Host = preserveRawString(raw.Host, current.Host, preset.Host)
	merged.TabColor = preserveRawString(raw.TabColor, current.TabColor, preset.TabColor)
	merged.Meta = mergePresetMeta(
		merged.Type,
		raw.Meta,
		current.Meta,
		preset.Meta,
		clearPresetPassword,
		clearPresetPrivateKey,
	)
	return merged
}

func preserveRawString(raw string, current string, next string) string {
	if next == current {
		return raw
	}
	return next
}

func mergePresetMeta(
	presetType string,
	raw Meta,
	current map[string]string,
	next map[string]string,
	clearPresetPassword bool,
	clearPresetPrivateKey bool,
) Meta {
	merged := Meta{}
	for key, value := range next {
		if rawValue, rawOK := raw[key]; rawOK &&
			shouldPreserveRawMetaReference(key, rawValue, value, current[key]) {
			merged[key] = rawValue
			continue
		}
		if currentValue, ok := current[key]; ok && value == currentValue {
			if rawValue, rawOK := raw[key]; rawOK {
				merged[key] = rawValue
				continue
			}
			merged[key] = String(value)
			continue
		}
		merged[key] = String(value)
	}
	for key, value := range raw {
		if _, ok := merged[key]; !ok {
			if isPresetPasswordMeta(key) &&
				(next["Authentication"] != "Password" || clearPresetPassword) {
				continue
			}
			if key == PresetMetaPrivateKey &&
				(next["Authentication"] != "Private Key" || clearPresetPrivateKey) {
				continue
			}
			if isKnownPresetMeta(key) && !presetMetaAllowedForType(presetType, key) {
				continue
			}
			merged[key] = value
		}
	}
	if _, ok := next[PresetMetaEncryptedPassword]; ok {
		delete(merged, PresetMetaPassword)
	}
	return merged
}

func shouldPreserveRawMetaReference(
	key string,
	rawValue String,
	nextValue string,
	currentValue string,
) bool {
	if key != "Private Key" {
		return false
	}
	if !strings.Contains(string(rawValue), "://") {
		return false
	}
	if strings.Contains(nextValue, "://") {
		return false
	}
	if nextValue == currentValue {
		return true
	}
	resolvedValue, err := rawValue.Parse()
	return err == nil && nextValue == resolvedValue
}

func isPresetPasswordMeta(key string) bool {
	return key == PresetMetaPassword || key == PresetMetaEncryptedPassword
}

func copyMeta(meta Meta) Meta {
	if meta == nil {
		return Meta{}
	}
	copied := make(Meta, len(meta))
	for key, value := range meta {
		copied[key] = value
	}
	return copied
}

func metaInputFromPreset(meta map[string]string) Meta {
	input := make(Meta, len(meta))
	for key, value := range meta {
		input[key] = String(value)
	}
	return input
}

func presetInputIndexByID(inputs presetInputs) map[string]int {
	byID := make(map[string]int, len(inputs))
	for i, input := range inputs {
		byID[strings.TrimSpace(input.ID)] = i
	}
	return byID
}

func presetMapByID(presets []Preset) map[string]Preset {
	byID := make(map[string]Preset, len(presets))
	for _, preset := range presets {
		byID[strings.TrimSpace(preset.ID)] = preset
	}
	return byID
}

type commonInputFileDocument struct {
	input      commonInput
	raw        map[string]any
	rawPresets []map[string]any
	presetsKey string
	syntax     *yaml.Node
	mode       os.FileMode
}

func readCommonInputFileDocument(filePath string) (commonInputFileDocument, error) {
	info, statErr := os.Stat(filePath)
	if statErr != nil {
		return commonInputFileDocument{}, statErr
	}

	data, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return commonInputFileDocument{}, readErr
	}
	raw, decodeErr := decodeYAMLMap(data)
	if decodeErr != nil {
		return commonInputFileDocument{}, decodeErr
	}
	syntax := yaml.Node{}
	if decodeErr := yaml.Unmarshal(data, &syntax); decodeErr != nil {
		return commonInputFileDocument{}, decodeErr
	}
	cfg, inputErr := commonInputFromYAMLMap(raw)
	if inputErr != nil {
		return commonInputFileDocument{}, inputErr
	}
	var rawPresets []map[string]any
	presetsKey, presets, ok := rawMappingValueCaseInsensitive(raw, "Presets")
	if ok {
		rawPresets, decodeErr = rawPresetMapsFromSyntax(&syntax, presetsKey, presets)
		if decodeErr != nil {
			return commonInputFileDocument{}, decodeErr
		}
	}
	return commonInputFileDocument{
		input:      cfg,
		raw:        raw,
		rawPresets: rawPresets,
		presetsKey: presetsKey,
		syntax:     &syntax,
		mode:       info.Mode(),
	}, nil
}

// readCommonInputFile decodes filePath and returns its file mode for rewrites.
func readCommonInputFile(filePath string) (commonInput, os.FileMode, error) {
	doc, err := readCommonInputFileDocument(filePath)
	if err != nil {
		return commonInput{}, 0, err
	}
	return doc.input, doc.mode, nil
}

func writeCommonInputFileDocument(
	filePath string,
	doc commonInputFileDocument,
) error {
	raw := doc.raw
	if raw == nil {
		raw = map[string]any{}
	}
	if _, ok := raw["AdminPassword"]; ok || doc.input.AdminPassword != "" {
		raw["AdminPassword"] = doc.input.AdminPassword
	}
	presets, marshalErr := marshalPresetInputsPreservingRaw(
		doc.input.Presets,
		doc.rawPresets,
	)
	if marshalErr != nil {
		return marshalErr
	}
	presetsKey := doc.presetsKey
	if presetsKey == "" {
		presetsKey = "Presets"
	}
	raw[presetsKey] = presets
	if doc.syntax != nil {
		updates := map[string]any{
			presetsKey: presets,
		}
		if adminPassword, ok := raw["AdminPassword"]; ok {
			updates["AdminPassword"] = adminPassword
		}
		return writeCommonInputFileSyntax(filePath, doc.syntax, updates, doc.mode)
	}
	return writeCommonInputFile(filePath, raw, doc.mode)
}

func writeCommonInputFileSyntax(
	filePath string,
	syntax *yaml.Node,
	updates map[string]any,
	mode os.FileMode,
) error {
	root := yamlMappingRoot(syntax)
	if root == nil {
		return writeCommonInputFile(filePath, updates, mode)
	}
	for key, value := range updates {
		node, err := yamlNodeFromValue(value)
		if err != nil {
			return err
		}
		setYAMLMappingValue(root, key, node)
	}
	return writeCommonInputFileNode(filePath, syntax, mode)
}

func yamlMappingRoot(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	return node
}

func yamlNodeFromValue(value any) (*yaml.Node, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	node := yaml.Node{}
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	if len(node.Content) == 1 {
		return node.Content[0], nil
	}
	return &node, nil
}

func setYAMLMappingValue(root *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if !strings.EqualFold(root.Content[i].Value, key) {
			continue
		}
		value.Anchor = root.Content[i+1].Anchor
		value.HeadComment = root.Content[i+1].HeadComment
		value.LineComment = root.Content[i+1].LineComment
		value.FootComment = root.Content[i+1].FootComment
		root.Content[i+1] = value
		return
	}
	root.Content = append(root.Content, &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: key,
	}, value)
}

func marshalPresetInputsPreservingRaw(
	inputs presetInputs,
	rawPresets []map[string]any,
) ([]map[string]any, error) {
	rawByID := make(map[string]map[string]any, len(rawPresets))
	for _, rawPreset := range rawPresets {
		id := rawPresetString(rawPreset, "ID")
		if id != "" {
			rawByID[id] = rawPreset
		}
	}

	presets := make([]map[string]any, len(inputs))
	for i, input := range inputs {
		id := strings.TrimSpace(input.ID)
		rawPreset := rawByID[id]
		if rawPreset == nil && i < len(rawPresets) {
			rawID := rawPresetString(rawPresets[i], "ID")
			if rawID == "" || rawID == id {
				rawPreset = rawPresets[i]
			}
		}
		preset, err := mergePresetInputRaw(input, rawPreset)
		if err != nil {
			return nil, err
		}
		presets[i] = preset
	}
	return presets, nil
}

func rawPresetString(
	rawPreset map[string]any,
	key string,
) string {
	if rawPreset == nil {
		return ""
	}
	if value, ok := rawPreset[key].(string); ok {
		return strings.TrimSpace(value)
	}
	if value, ok := rawPreset[key].(*yaml.Node); ok && value.Kind == yaml.ScalarNode {
		return strings.TrimSpace(value.Value)
	}
	return ""
}

func mergePresetInputRaw(
	input presetInput,
	rawPreset map[string]any,
) (map[string]any, error) {
	const presetInputFieldCount = 6
	merged := make(map[string]any, safePresetInputCapacity(len(rawPreset), presetInputFieldCount))
	for key, value := range rawPreset {
		merged[key] = value
	}
	merged["ID"] = input.ID
	merged["Title"] = input.Title
	merged["Type"] = input.Type
	merged["Host"] = input.Host
	merged["TabColor"] = input.TabColor
	merged["Meta"] = input.Meta
	return merged, nil
}

func rawPresetMaps(value any) ([]map[string]any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var rawPresets []map[string]any
	if err := json.Unmarshal(data, &rawPresets); err != nil {
		return nil, err
	}
	return rawPresets, nil
}

func rawPresetMapsFromSyntax(
	syntax *yaml.Node,
	key string,
	decoded any,
) ([]map[string]any, error) {
	decodedRawPresets, err := rawPresetMaps(decoded)
	if err != nil {
		return nil, err
	}
	presetsNode := yamlMappingValueNode(yamlMappingRoot(syntax), key)
	if presetsNode == nil || presetsNode.Kind != yaml.SequenceNode {
		return decodedRawPresets, nil
	}
	rawPresets := make([]map[string]any, 0, len(presetsNode.Content))
	for i, presetNode := range presetsNode.Content {
		if presetNode.Kind == yaml.AliasNode {
			presetNode = cloneYAMLNodeMaterializingAliases(presetNode)
		}
		if presetNode.Kind != yaml.MappingNode {
			return decodedRawPresets, nil
		}
		var decodedRawPreset map[string]any
		if i < len(decodedRawPresets) {
			decodedRawPreset = decodedRawPresets[i]
		}
		rawPresets = append(rawPresets, rawPresetMapFromYAMLNode(presetNode, decodedRawPreset))
	}
	return rawPresets, nil
}

func yamlMappingValueNode(root *yaml.Node, key string) *yaml.Node {
	if root == nil {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if strings.EqualFold(root.Content[i].Value, key) {
			return root.Content[i+1]
		}
	}
	return nil
}

func rawMappingValueCaseInsensitive(raw map[string]any, want string) (string, any, bool) {
	for key, value := range raw {
		if strings.EqualFold(key, want) {
			return key, value, true
		}
	}
	return "", nil, false
}

func rawPresetMapFromYAMLNode(node *yaml.Node, decoded map[string]any) map[string]any {
	rawPreset := make(map[string]any, safePresetInputCapacity(len(decoded), len(node.Content)/2))
	for key, value := range decoded {
		rawPreset[key] = value
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if key == "<<" {
			continue
		}
		rawPreset[key] = cloneYAMLNodeMaterializingAliases(node.Content[i+1])
	}
	return rawPreset
}

func cloneYAMLNodeMaterializingAliases(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode {
		return cloneYAMLNodeMaterializingAliases(node.Alias)
	}
	cloned := *node
	cloned.Alias = nil
	cloned.Anchor = ""
	if len(node.Content) > 0 {
		cloned.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			cloned.Content[i] = cloneYAMLNodeMaterializingAliases(child)
		}
	}
	return &cloned
}

// writeCommonInputFile atomically rewrites filePath with cfg encoded as YAML.
func writeCommonInputFile(
	filePath string,
	cfg map[string]any,
	mode os.FileMode,
) error {
	data, marshalErr := yaml.Marshal(cfg)
	if marshalErr != nil {
		return marshalErr
	}
	return writeCommonInputFileBytes(filePath, data, mode)
}

func writeCommonInputFileNode(
	filePath string,
	node *yaml.Node,
	mode os.FileMode,
) error {
	data, marshalErr := yaml.Marshal(node)
	if marshalErr != nil {
		return marshalErr
	}
	return writeCommonInputFileBytes(filePath, data, mode)
}

func writeCommonInputFileBytes(
	filePath string,
	data []byte,
	mode os.FileMode,
) error {
	tmp, createErr := os.CreateTemp(filepath.Dir(filePath), filepath.Base(filePath)+".*.tmp")
	if createErr != nil {
		return createErr
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, writeErr := tmp.Write(data); writeErr != nil {
		tmp.Close()
		return writeErr
	}
	if syncErr := tmp.Sync(); syncErr != nil {
		tmp.Close()
		return syncErr
	}
	if chmodErr := tmp.Chmod(mode); chmodErr != nil {
		tmp.Close()
		return chmodErr
	}
	if closeErr := tmp.Close(); closeErr != nil {
		return closeErr
	}
	return atomic.ReplaceFile(tmpName, filePath)
}
