// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
)

// socketVerification is the controller for the "/shellport/socket/verify"
// endpoint. It handles client authentication via a time-windowed HMAC token
// and returns server configuration (heartbeat interval, timeout, and preset
// remote list) as JSON to authenticated clients.
type socketVerification struct {
	socket

	// heartbeat is the server's configured heartbeat timeout in seconds,
	// pre-formatted as a string for inclusion in the X-Heartbeat response header.
	heartbeat string
	// timeout is the server's configured read timeout in seconds,
	// pre-formatted as a string for inclusion in the X-Timeout response header.
	timeout string
	// configRspBody is the pre-serialized JSON body containing the access
	// configuration (presets and server message) sent to authenticated clients.
	configRspBody []byte
}

// socketRemotePreset is the JSON-serializable representation of a single
// preset remote connection. It is derived from configuration.Preset and
// transmitted to the client as part of the socket access configuration.
type socketRemotePreset struct {
	ID                 string            `json:"id"`
	Title              string            `json:"title"`
	Type               string            `json:"type"`
	Host               string            `json:"host"`
	TabColor           string            `json:"tab_color"`
	Meta               map[string]string `json:"meta"`
	HasSavedPassword   bool              `json:"has_saved_password"`
	HasSavedPrivateKey bool              `json:"has_saved_private_key"`
	PrivateKeyFile     string            `json:"private_key_file"`
	PrivateKeyFilename string            `json:"private_key_filename"`
}

// socketAccessConfiguration is the top-level JSON envelope sent to the client
// after successful authentication on the verification endpoint. It carries the
// list of preset remote connections, server title, and HTML-escaped server
// message. ServerTitle is plain text; the client renders it with Vue text
// interpolation rather than v-html.
type socketAccessConfiguration struct {
	Presets              []socketRemotePreset   `json:"presets"`
	ServerTitle          string                 `json:"server_title"`
	ServerMessage        string                 `json:"server_message"`
	PresetConfigWritable bool                   `json:"preset_config_writable"`
	PresetManagement     presetManagementPolicy `json:"preset_management"`
	PrivateKeyFiles      []string               `json:"private_key_files"`
}

type presetManagementPolicy struct {
	Writable                   bool `json:"writable"`
	CanManage                  bool `json:"can_manage"`
	RequiresAdminKey           bool `json:"requires_admin_key"`
	BlockedByPresetRestriction bool `json:"blocked_by_preset_restriction"`
}

type authRole int

const (
	authRoleNone authRole = iota
	authRoleUser
	authRoleAdmin
)

// newSocketAccessConfiguration builds a socketAccessConfiguration from the
// given slice of configured presets, a server title, and a server message. The
// server message is HTML-escaped and then Markdown-link-converted before being
// embedded in the response.
func newSocketAccessConfiguration(
	remotes []configuration.Preset,
	serverTitle string,
	serverMessage string,
	presetConfigWritable bool,
	presetManagement ...presetManagementPolicy,
) socketAccessConfiguration {
	policy := presetManagementPolicy{
		Writable: presetConfigWritable,
	}
	if len(presetManagement) > 0 {
		policy = presetManagement[0]
	}

	presets := make([]socketRemotePreset, len(remotes))
	for i := range presets {
		presets[i] = socketRemotePreset{
			Title:              remotes[i].Title,
			ID:                 remotes[i].ID,
			Type:               remotes[i].Type,
			Host:               remotes[i].Host,
			TabColor:           remotes[i].TabColor,
			Meta:               sanitizeSocketPresetMeta(remotes[i].Meta),
			HasSavedPassword:   presetHasSavedPassword(remotes[i]),
			HasSavedPrivateKey: presetHasSavedPrivateKey(remotes[i]),
			PrivateKeyFile:     presetPrivateKeyFile(remotes[i], policy),
			PrivateKeyFilename: presetPrivateKeyFilename(remotes[i]),
		}
	}
	return socketAccessConfiguration{
		Presets:              presets,
		ServerTitle:          serverTitle,
		ServerMessage:        parseServerMessage(html.EscapeString(serverMessage)),
		PresetConfigWritable: policy.Writable,
		PresetManagement:     policy,
		PrivateKeyFiles:      []string{},
	}
}

func sanitizeSocketPresetMeta(meta map[string]string) map[string]string {
	sanitized := make(map[string]string, len(meta))
	for key, value := range meta {
		if key == configuration.PresetMetaPassword ||
			key == configuration.PresetMetaEncryptedPassword ||
			key == configuration.PresetMetaPrivateKey {
			continue
		}
		sanitized[key] = value
	}
	return sanitized
}

func newPresetManagementPolicy(
	commonCfg configuration.Common,
	role authRole,
) presetManagementPolicy {
	writable := role >= authRoleUser && commonCfg.PresetConfigWritable()
	blockedByPresetRestriction := commonCfg.OnlyAllowPresetRemotes
	requiresAdminKey := writable &&
		!blockedByPresetRestriction &&
		commonCfg.AdminKey != "" &&
		role < authRoleAdmin

	return presetManagementPolicy{
		Writable:                   writable,
		CanManage:                  writable && !blockedByPresetRestriction,
		RequiresAdminKey:           requiresAdminKey,
		BlockedByPresetRestriction: blockedByPresetRestriction,
	}
}

func presetHasSavedPassword(preset configuration.Preset) bool {
	if preset.Meta != nil {
		if _, hasPassword := preset.Meta[configuration.PresetMetaPassword]; hasPassword {
			return true
		}
		if _, hasEncrypted := preset.Meta[configuration.PresetMetaEncryptedPassword]; hasEncrypted {
			return true
		}
	}
	if preset.SecretMeta != nil {
		if _, hasSecretPassword := preset.SecretMeta[configuration.PresetMetaPassword]; hasSecretPassword {
			return true
		}
	}
	return false
}

func presetHasSavedPrivateKey(preset configuration.Preset) bool {
	return preset.Meta[configuration.PresetMetaPrivateKey] != ""
}

func presetPrivateKeyFile(preset configuration.Preset, policy presetManagementPolicy) string {
	if !policy.CanManage || policy.RequiresAdminKey {
		return ""
	}
	privateKey := preset.Meta[configuration.PresetMetaPrivateKey]
	if privateKeyFilePath(privateKey) != "" {
		return privateKey
	}
	return ""
}

func presetPrivateKeyFilename(preset configuration.Preset) string {
	keyPath := privateKeyFilePath(preset.Meta[configuration.PresetMetaPrivateKey])
	if keyPath == "" {
		return ""
	}
	parts := strings.FieldsFunc(keyPath, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func privateKeyFilePath(privateKey string) string {
	schemeIndex := strings.Index(privateKey, "://")
	if schemeIndex < 0 || !strings.EqualFold(privateKey[:schemeIndex], "file") {
		return ""
	}
	return privateKey[schemeIndex+3:]
}

// buildAccessConfigRespondBody serializes accessCfg to JSON. It panics if
// marshaling fails, which should never occur for this well-typed struct.
func buildAccessConfigRespondBody(accessCfg socketAccessConfiguration) []byte {
	mData, mErr := json.Marshal(accessCfg)
	if mErr != nil {
		panic(fmt.Errorf("unable to marshal remote data: %s", mErr))
	}
	return mData
}

// newSocketVerification constructs a socketVerification controller that wraps
// s and pre-computes the heartbeat interval, read timeout, and the JSON access
// configuration body from srvCfg and commCfg. The configuration body is built
// once at startup to avoid repeated serialization on every request.
func newSocketVerification(
	s socket,
	srvCfg configuration.Server,
	commCfg configuration.Common,
) socketVerification {
	return socketVerification{
		socket: s,
		heartbeat: strconv.FormatFloat(
			srvCfg.HeartbeatTimeout.Seconds(), 'g', 2, 64),
		timeout: strconv.FormatFloat(
			srvCfg.ReadTimeout.Seconds(), 'g', 2, 64),
		configRspBody: buildAccessConfigRespondBody(
			socketAccessConfigurationWithPrivateKeyFiles(
				newSocketAccessConfiguration(
					commCfg.Presets,
					srvCfg.ServerTitle,
					srvCfg.ServerMessage,
					false,
					newPresetManagementPolicy(commCfg, authRoleUser),
				),
				commCfg,
			),
		),
	}
}

// authKeyForSecret derives the expected 32-byte authentication token for this
// request using a truncated Unix timestamp (100-second window) combined with
// the configured secret.
func (s socketVerification) authKeyForSecret(
	r *http.Request,
	secret string,
) []byte {
	return authKeyForSecret(secret)
}

func authKeyForSecret(secret string) []byte {
	timeMixer := strconv.FormatInt(time.Now().Unix()/100, 10)
	return hashCombineSocketKeys(
		timeMixer,
		secret,
	)[:32]
}

func (s socketVerification) anonymousAuthRole() authRole {
	return anonymousAuthRole(s.commonCfg)
}

func anonymousAuthRole(commonCfg configuration.Common) authRole {
	if commonCfg.SharedKey == "" {
		if commonCfg.AdminKey == "" {
			return authRoleAdmin
		}
		return authRoleUser
	}
	return authRoleNone
}

func requestAuthRoleForCommon(
	commonCfg configuration.Common,
	r *http.Request,
	allowAdminKey bool,
) (authRole, error) {
	key := r.Header.Get("X-Key")
	if len(key) <= 0 {
		return anonymousAuthRole(commonCfg), nil
	}
	if len(key) > 64 {
		return authRoleNone, ErrSocketInvalidAuthKey
	}
	time.Sleep(500 * time.Millisecond)
	decodedKey, decodedKeyErr := base64.StdEncoding.DecodeString(key)
	if decodedKeyErr != nil {
		return authRoleNone, NewError(http.StatusBadRequest, decodedKeyErr.Error())
	}
	if allowAdminKey &&
		commonCfg.AdminKey != "" &&
		hmac.Equal(authKeyForSecret(commonCfg.AdminKey), decodedKey) {
		return authRoleAdmin, nil
	}
	if commonCfg.SharedKey != "" &&
		hmac.Equal(authKeyForSecret(commonCfg.SharedKey), decodedKey) {
		if commonCfg.AdminKey == "" {
			return authRoleAdmin, nil
		}
		return authRoleUser, nil
	}
	return authRoleNone, ErrSocketAuthFailed
}

func (s socketVerification) requestAuthRole(r *http.Request) (authRole, error) {
	return requestAuthRoleForCommon(s.commonCfg, r, false)
}

// setServerConfigRespond appends the X-Heartbeat, X-Timeout, and (when
// applicable) X-OnlyAllowPresetRemotes headers to hd, sets the Content-Type,
// and writes the pre-serialized JSON configuration body to w.
func (s socketVerification) setServerConfigRespond(
	hd *http.Header, w http.ResponseWriter, role authRole) {
	hd.Add("X-Heartbeat", s.heartbeat)
	hd.Add("X-Timeout", s.timeout)
	if s.commonCfg.OnlyAllowPresetRemotes {
		hd.Add("X-OnlyAllowPresetRemotes", "yes")
	}
	hd.Set("Content-Type", "application/json; charset=utf-8")
	w.Write(buildAccessConfigRespondBody(
		socketAccessConfigurationWithPrivateKeyFiles(
			newSocketAccessConfiguration(
				s.commonCfg.CurrentPresets(),
				s.serverCfg.ServerTitle,
				s.serverCfg.ServerMessage,
				false,
				newPresetManagementPolicy(s.commonCfg, role),
			),
			s.commonCfg,
		),
	))
}

func socketAccessConfigurationWithPrivateKeyFiles(
	accessConfig socketAccessConfiguration,
	commonCfg configuration.Common,
) socketAccessConfiguration {
	if !accessConfig.PresetManagement.CanManage ||
		accessConfig.PresetManagement.RequiresAdminKey {
		accessConfig.PrivateKeyFiles = []string{}
		return accessConfig
	}
	files, err := configuration.ListPresetPrivateKeyFiles(commonCfg.SourceFile)
	if err != nil {
		accessConfig.PrivateKeyFiles = []string{}
		return accessConfig
	}
	accessConfig.PrivateKeyFiles = files
	return accessConfig
}

// Get handles HTTP GET requests for the socket verification endpoint. When no
// X-Key header is present and no shared key is configured, it returns the
// server configuration immediately. When a shared key is configured and no
// X-Key header is present, it returns ErrSocketInvalidAuthKey. When an X-Key
// header is present, it base64-decodes the value, applies a 500ms delay to
// slow brute-force attempts, and compares the decoded bytes against the
// time-windowed HMAC; it returns ErrSocketAuthFailed on mismatch or the server
// configuration on success.
func (s socketVerification) Get(
	w *ResponseWriter, r *http.Request, l log.Logger) error {
	hd := w.Header()
	hd.Add("Cache-Control", "no-store")
	hd.Add("Pragma", "no-store")
	key := r.Header.Get("X-Key")
	if len(key) <= 0 {
		hd.Add("X-Key", base64.StdEncoding.EncodeToString(s.mixerKey(r)))
		role := s.anonymousAuthRole()
		if role >= authRoleUser {
			s.setServerConfigRespond(&hd, w, role)
			return nil
		}
		return ErrSocketInvalidAuthKey
	}
	role, err := s.requestAuthRole(r)
	if err != nil {
		return err
	}
	hd.Add("X-Key", base64.StdEncoding.EncodeToString(s.mixerKey(r)))
	s.setServerConfigRespond(&hd, w, role)
	return nil
}
