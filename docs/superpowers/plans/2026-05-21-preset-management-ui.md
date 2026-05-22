# Preset Management UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add ShellPort UI support for creating, editing, and deleting config-file presets while enforcing `OnlyAllowPresetRemotes` and `AdminKey` policy.

**Architecture:** Keep the existing backend preset config API as the write path, add policy and hidden-secret metadata to the verification/config responses, and implement a compact Vue preset editor inside the existing connection overlay. Use pure JavaScript helper modules for preset editor state and API payload construction so behavior is testable without a browser component test harness.

**Tech Stack:** Go `net/http` controllers and tests, Vue 3 single-file components, plain CSS, Vitest for frontend units, existing `npm` scripts, and existing `go test` package tests.

---

## File Structure

- Modify `application/controller/socket_verify.go`: add preset management policy metadata and `has_saved_password` to preset JSON.
- Modify `application/controller/preset_config.go`: expose `has_saved_password` on config API responses and add a targeted header for clearing hidden saved passwords during full-list writes.
- Modify `application/controller/preset_config_test.go`: cover config API hidden-password metadata and targeted clearing.
- Create `application/controller/socket_verify_test.go`: cover verification policy metadata.
- Modify `ui/app.js`: parse preset policy, cache AdminKey in page memory, expose full-list preset save/delete functions to `home.vue`.
- Modify `ui/commands/presets.js`: parse and expose `has_saved_password`; preserve it in `toConfig` only as client metadata, not as backend `meta`.
- Create `ui/preset_management.js`: pure helpers for policy checks, editor state, payload construction, hidden password clear IDs, and save-as conversion from wizard fields.
- Create `ui/preset_management_test.js`: pure helper coverage for policy, secret defaults, fingerprint exclusion, and payload behavior.
- Create `ui/widgets/preset_editor.vue`: editor form for create/update/delete plus AdminKey and delete confirmation states.
- Create `ui/widgets/preset_editor.css`: scoped styles for the editor inside the connection overlay.
- Modify `ui/widgets/connect_known.vue`: right-aligned icon edit button with accessible label.
- Modify `ui/widgets/connect_known.css`: layout and icon-button styling.
- Modify `ui/widgets/connect.vue`: route edit events and render the preset editor mode inside the existing connect window.
- Modify `ui/home.vue`: own management state, open editor, save/delete presets, and pass save-as callback to command wizards.
- Modify `ui/commands/commands.js`: allow prompt action descriptors to opt out of disabling the current prompt after action completion when save-as opens the editor.
- Modify `ui/commands/ssh.js`, `ui/commands/mosh.js`, `ui/commands/et.js`, and `ui/commands/telnet.js`: add an optional save-as action on the first connection-details prompt.
- Add targeted frontend tests in existing command test files or `ui/preset_management_test.js` for save-as conversion and editor payloads.
- Update `CONFIGURATION.md` and `README.md`: replace the note that no AdminKey UI exists with the new behavior.

## Task 1: Backend Verification Metadata

**Files:**
- Modify: `application/controller/socket_verify.go`
- Create: `application/controller/socket_verify_test.go`

- [ ] **Step 1: Write failing tests for verification metadata**

Create `application/controller/socket_verify_test.go`:

```go
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"encoding/json"
	"testing"

	"github.com/Snuffy2/shellport/application/configuration"
)

func decodeAccessConfigForTest(t *testing.T, cfg socketAccessConfiguration) map[string]any {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal(buildAccessConfigRespondBody(cfg), &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	return decoded
}

func TestSocketAccessConfigurationIncludesPresetManagementPolicy(t *testing.T) {
	tests := []struct {
		name                string
		commonCfg           configuration.Common
		role                authRole
		wantWritable        bool
		wantCanManage       bool
		wantRequiresAdmin   bool
		wantBlockedByPreset bool
	}{
		{
			name: "non writable config",
			commonCfg: configuration.Common{
				SourceFile: "",
			},
			role:                authRoleAdmin,
			wantWritable:        false,
			wantCanManage:       false,
			wantRequiresAdmin:   false,
			wantBlockedByPreset: false,
		},
		{
			name: "blank admin key writes immediately",
			commonCfg: configuration.Common{
				SourceFile: "/tmp/shellport.conf.json",
				AdminKey:   "",
			},
			role:                authRoleAdmin,
			wantWritable:        true,
			wantCanManage:       true,
			wantRequiresAdmin:   false,
			wantBlockedByPreset: false,
		},
		{
			name: "admin key prompt required for user role",
			commonCfg: configuration.Common{
				SourceFile: "/tmp/shellport.conf.json",
				AdminKey:   "admin-secret",
			},
			role:                authRoleUser,
			wantWritable:        true,
			wantCanManage:       true,
			wantRequiresAdmin:   true,
			wantBlockedByPreset: false,
		},
		{
			name: "preset restriction blocks management",
			commonCfg: configuration.Common{
				SourceFile:              "/tmp/shellport.conf.json",
				AdminKey:                "",
				OnlyAllowPresetRemotes: true,
			},
			role:                authRoleAdmin,
			wantWritable:        true,
			wantCanManage:       false,
			wantRequiresAdmin:   false,
			wantBlockedByPreset: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := newPresetManagementPolicy(tt.commonCfg, tt.role)

			if policy.Writable != tt.wantWritable {
				t.Fatalf("Writable = %v, want %v", policy.Writable, tt.wantWritable)
			}
			if policy.CanManage != tt.wantCanManage {
				t.Fatalf("CanManage = %v, want %v", policy.CanManage, tt.wantCanManage)
			}
			if policy.RequiresAdminKey != tt.wantRequiresAdmin {
				t.Fatalf("RequiresAdminKey = %v, want %v", policy.RequiresAdminKey, tt.wantRequiresAdmin)
			}
			if policy.BlockedByPresetRestriction != tt.wantBlockedByPreset {
				t.Fatalf(
					"BlockedByPresetRestriction = %v, want %v",
					policy.BlockedByPresetRestriction,
					tt.wantBlockedByPreset,
				)
			}
		})
	}
}

func TestSocketAccessConfigurationMarksHiddenSavedPassword(t *testing.T) {
	cfg := newSocketAccessConfiguration(
		[]configuration.Preset{
			{
				ID:    "preset-password",
				Title: "Password",
				Type:  "SSH",
				Host:  "example.com:22",
				Meta: map[string]string{
					"Authentication": configuration.PresetMetaPassword,
					configuration.PresetMetaEncryptedPassword: "v1:aes-256-gcm:nonce:ciphertext",
				},
			},
		},
		"",
		"",
		newPresetManagementPolicy(configuration.Common{}, authRoleAdmin),
	)
	decoded := decodeAccessConfigForTest(t, cfg)
	presets := decoded["presets"].([]any)
	preset := presets[0].(map[string]any)
	meta := preset["meta"].(map[string]any)

	if preset["has_saved_password"] != true {
		t.Fatalf("has_saved_password = %v, want true", preset["has_saved_password"])
	}
	if _, ok := meta[configuration.PresetMetaPassword]; ok {
		t.Fatal("plaintext password leaked into socket preset metadata")
	}
	if _, ok := meta[configuration.PresetMetaEncryptedPassword]; ok {
		t.Fatal("encrypted password leaked into socket preset metadata")
	}
}
```

- [ ] **Step 2: Run the backend metadata tests and verify they fail**

Run:

```bash
go test ./application/controller -run 'TestSocketAccessConfiguration' -v
```

Expected: FAIL with errors for missing `newPresetManagementPolicy`, missing policy fields, or missing `has_saved_password`.

- [ ] **Step 3: Implement verification metadata**

In `application/controller/socket_verify.go`, add these types near `socketAccessConfiguration`:

```go
type presetManagementPolicy struct {
	Writable                   bool `json:"writable"`
	CanManage                  bool `json:"can_manage"`
	RequiresAdminKey           bool `json:"requires_admin_key"`
	BlockedByPresetRestriction bool `json:"blocked_by_preset_restriction"`
}
```

Extend `socketRemotePreset`:

```go
type socketRemotePreset struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Type             string            `json:"type"`
	Host             string            `json:"host"`
	TabColor         string            `json:"tab_color"`
	Meta             map[string]string `json:"meta"`
	HasSavedPassword bool              `json:"has_saved_password"`
}
```

Extend `socketAccessConfiguration`:

```go
type socketAccessConfiguration struct {
	Presets              []socketRemotePreset   `json:"presets"`
	ServerTitle          string                 `json:"server_title"`
	ServerMessage        string                 `json:"server_message"`
	PresetConfigWritable bool                   `json:"preset_config_writable"`
	PresetManagement     presetManagementPolicy `json:"preset_management"`
}
```

Add these helpers:

```go
func newPresetManagementPolicy(
	commonCfg configuration.Common,
	role authRole,
) presetManagementPolicy {
	writable := role >= authRoleUser && commonCfg.PresetConfigWritable()
	blocked := commonCfg.OnlyAllowPresetRemotes
	requiresAdminKey := writable && !blocked && commonCfg.AdminKey != "" && role < authRoleAdmin

	return presetManagementPolicy{
		Writable:                   writable,
		CanManage:                  writable && !blocked,
		RequiresAdminKey:           requiresAdminKey,
		BlockedByPresetRestriction: blocked,
	}
}

func presetHasSavedPassword(preset configuration.Preset) bool {
	if preset.Meta != nil {
		if value := preset.Meta[configuration.PresetMetaPassword]; value != "" {
			return true
		}
		if value := preset.Meta[configuration.PresetMetaEncryptedPassword]; value != "" {
			return true
		}
	}
	if preset.SecretMeta != nil {
		if value := preset.SecretMeta[configuration.PresetMetaPassword]; value != "" {
			return true
		}
	}
	return false
}
```

Change `newSocketAccessConfiguration` to accept `presetManagementPolicy` instead of `presetConfigWritable bool`, set `HasSavedPassword: presetHasSavedPassword(remotes[i])`, keep `PresetConfigWritable: policy.Writable`, and set `PresetManagement: policy`.

Update callers:

```go
newSocketAccessConfiguration(
	commCfg.Presets,
	srvCfg.ServerTitle,
	srvCfg.ServerMessage,
	newPresetManagementPolicy(commCfg, authRoleUser),
)
```

In `setServerConfigRespond`, pass the real role:

```go
newSocketAccessConfiguration(
	s.commonCfg.CurrentPresets(),
	s.serverCfg.ServerTitle,
	s.serverCfg.ServerMessage,
	newPresetManagementPolicy(s.commonCfg, role),
)
```

In `application/controller/preset_config.go`, update `writePresets` to use the new signature:

```go
		Presets: newSocketAccessConfiguration(
			presets,
			"",
			"",
			newPresetManagementPolicy(p.commonCfg, authRoleUser),
		).Presets,
```

- [ ] **Step 4: Run backend metadata tests and verify they pass**

Run:

```bash
go test ./application/controller -run 'TestSocketAccessConfiguration' -v
```

Expected: PASS.

- [ ] **Step 5: Commit backend metadata**

Run:

```bash
gofmt -w application/controller/socket_verify.go application/controller/socket_verify_test.go
go test ./application/controller -run 'TestSocketAccessConfiguration' -v
git add application/controller/socket_verify.go application/controller/socket_verify_test.go
git commit -m "feat: expose preset management metadata"
```

Expected: tests pass and commit succeeds.

## Task 2: Backend Hidden Password Clearing

**Files:**
- Modify: `application/controller/preset_config.go`
- Modify: `application/controller/preset_config_test.go`

- [ ] **Step 1: Write failing tests for hidden password clear and response metadata**

Append these tests to `application/controller/preset_config_test.go`:

```go
func TestPresetConfigGetMarksHiddenSavedPassword(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-atlantis",
			"Title": "Atlantis",
			"Type":  "SSH",
			"Host":  "atlantis.home",
			"Meta": map[string]any{
				"Authentication": "Password",
				"Password":       "hidden-password",
			},
		},
	})
	controller := newTestPresetConfig(t, configPath)
	request := httptest.NewRequest(http.MethodGet, "/shellport/config/presets", nil)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Get(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	var response presetConfigResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("json Decode returned error: %v", err)
	}
	if !response.Presets[0].HasSavedPassword {
		t.Fatal("HasSavedPassword = false, want true")
	}
	if _, ok := response.Presets[0].Meta["Password"]; ok {
		t.Fatal("response leaked password metadata")
	}
}

func TestPresetConfigPutCanClearOneHiddenPasswordWhilePreservingOthers(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "shellport.conf.json")
	writePresetAPIConfig(t, configPath, []map[string]any{
		{
			"ID":    "preset-clear",
			"Title": "Clear",
			"Type":  "SSH",
			"Host":  "clear.home",
			"Meta": map[string]any{
				"Authentication": "Password",
				"Password":       "clear-password",
			},
		},
		{
			"ID":    "preset-keep",
			"Title": "Keep",
			"Type":  "SSH",
			"Host":  "keep.home",
			"Meta": map[string]any{
				"Authentication": "Password",
				"Password":       "keep-password",
			},
		},
	})
	controller := newAuthenticatedTestPresetConfig(t, configPath)
	body := []byte(`{"presets":[{"id":"preset-clear","title":"Clear","type":"SSH","host":"clear.home","meta":{"Authentication":"Password"}},{"id":"preset-keep","title":"Keep","type":"SSH","host":"keep.home","meta":{"Authentication":"Password"}}]}`)
	request := httptest.NewRequest(http.MethodPut, "/shellport/config/presets", bytes.NewReader(body))
	request.Header.Set(preserveHiddenPresetPasswordsHeader, "yes")
	request.Header.Set(clearHiddenPresetPasswordsHeader, "preset-clear")
	authorizePresetConfigRequest(controller, request)
	recorder := httptest.NewRecorder()
	writer := newResponseWriter(recorder)

	if err := controller.Put(&writer, request, log.Ditch{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	_, reloaded, err := configuration.CustomFile(configPath)(log.Ditch{})
	if err != nil {
		t.Fatalf("CustomFile returned error: %v", err)
	}

	if _, ok := reloaded.Presets[0].Meta["Password"]; ok {
		t.Fatal("preset-clear still has Password metadata")
	}
	if reloaded.Presets[1].Meta["Password"] != "keep-password" {
		t.Fatalf("preset-keep password = %q, want keep-password", reloaded.Presets[1].Meta["Password"])
	}
}
```

- [ ] **Step 2: Run hidden password tests and verify they fail**

Run:

```bash
go test ./application/controller -run 'TestPresetConfig(GetMarksHiddenSavedPassword|PutCanClearOneHiddenPasswordWhilePreservingOthers)' -v
```

Expected: FAIL with missing `HasSavedPassword` or missing `clearHiddenPresetPasswordsHeader`.

- [ ] **Step 3: Add clear-password header support**

In `application/controller/preset_config.go`, extend constants:

```go
clearHiddenPresetPasswordsHeader = "X-Clear-Hidden-Preset-Passwords"
```

Add helpers near `preserveHiddenPresetPasswords`:

```go
func parsePresetIDSet(value string) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, part := range strings.Split(value, ",") {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		ids[id] = struct{}{}
	}
	return ids
}

func preserveHiddenPresetPasswordsExcept(
	presets []configuration.Preset,
	current []configuration.Preset,
	clearIDs map[string]struct{},
) []configuration.Preset {
	currentByID := make(map[string]configuration.Preset, len(current))
	for _, preset := range current {
		currentByID[preset.ID] = preset
	}
	merged := make([]configuration.Preset, len(presets))
	for i, preset := range presets {
		merged[i] = preset
		if _, clear := clearIDs[preset.ID]; clear {
			continue
		}
		currentPreset, ok := currentByID[preset.ID]
		if !ok {
			continue
		}
		if preset.Meta["Authentication"] != "Password" ||
			currentPreset.Meta["Authentication"] != "Password" {
			continue
		}
		if hasPresetPasswordMeta(preset.Meta) {
			continue
		}
		merged[i].Meta = copyPresetMeta(preset.Meta)
		copyHiddenPresetPassword(merged[i].Meta, currentPreset.Meta)
	}
	return merged
}
```

Replace both calls to `preserveHiddenPresetPasswords(presets, currentPresets)` with:

```go
presets = preserveHiddenPresetPasswordsExcept(
	presets,
	currentPresets,
	parsePresetIDSet(r.Header.Get(clearHiddenPresetPasswordsHeader)),
)
```

Keep `preserveHiddenPresetPasswords` only if existing tests call it directly; implement it as:

```go
func preserveHiddenPresetPasswords(
	presets []configuration.Preset,
	current []configuration.Preset,
) []configuration.Preset {
	return preserveHiddenPresetPasswordsExcept(presets, current, nil)
}
```

- [ ] **Step 4: Run hidden password tests and verify they pass**

Run:

```bash
gofmt -w application/controller/preset_config.go application/controller/preset_config_test.go
go test ./application/controller -run 'TestPresetConfig(GetMarksHiddenSavedPassword|PutCanClearOneHiddenPasswordWhilePreservingOthers)' -v
```

Expected: PASS.

- [ ] **Step 5: Commit hidden password clear support**

Run:

```bash
git add application/controller/preset_config.go application/controller/preset_config_test.go
git commit -m "feat: support clearing hidden preset passwords"
```

Expected: commit succeeds.

## Task 3: Frontend Preset Model And Helper Module

**Files:**
- Modify: `ui/commands/presets.js`
- Create: `ui/preset_management.js`
- Create: `ui/preset_management_test.js`

- [ ] **Step 1: Write failing frontend helper tests**

Create `ui/preset_management_test.js`:

```js
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "vitest";

import { Preset } from "./commands/presets.js";
import {
  buildEditorState,
  buildPresetConfigFromEditorState,
  buildPresetConfigFromWizardFields,
  canManagePresets,
  clearHiddenPasswordIDs,
  requiresAdminKey,
} from "./preset_management.js";

describe("preset management policy", () => {
  test("allows management when policy can_manage is true", () => {
    expect(canManagePresets({ can_manage: true })).toBe(true);
    expect(canManagePresets({ can_manage: false })).toBe(false);
    expect(canManagePresets(null)).toBe(false);
  });

  test("requires admin key only when policy says so", () => {
    expect(requiresAdminKey({ requires_admin_key: true })).toBe(true);
    expect(requiresAdminKey({ requires_admin_key: false })).toBe(false);
  });
});

describe("preset editor state", () => {
  test("defaults existing hidden password to keep checked", () => {
    const preset = new Preset({
      id: "preset-atlantis",
      title: "Atlantis",
      type: "SSH",
      host: "atlantis.home:22",
      has_saved_password: true,
      meta: {
        Authentication: "Password",
        User: "pi",
      },
    });

    const state = buildEditorState(preset);

    expect(state.savePassword).toBe(true);
    expect(state.hasSavedPassword).toBe(true);
    expect(state.password).toBe("");
  });

  test("defaults new password saving off and private key saving on when present", () => {
    const state = buildEditorState(null, {
      type: "SSH",
      meta: {
        Authentication: "Private Key",
        "Private Key": "PRIVATE KEY DATA",
      },
    });

    expect(state.savePassword).toBe(false);
    expect(state.savePrivateKey).toBe(true);
  });

  test("does not include fingerprint in editor state or payload", () => {
    const preset = new Preset({
      id: "preset-atlantis",
      title: "Atlantis",
      type: "SSH",
      host: "atlantis.home:22",
      meta: {
        Fingerprint: "SHA256:abc",
        User: "pi",
      },
    });

    const state = buildEditorState(preset);
    const config = buildPresetConfigFromEditorState(state);

    expect(state.meta.Fingerprint).toBeUndefined();
    expect(config.meta.Fingerprint).toBeUndefined();
  });

  test("clearHiddenPasswordIDs includes only unchecked hidden password presets", () => {
    const state = buildEditorState(
      new Preset({
        id: "preset-clear",
        title: "Clear",
        type: "SSH",
        host: "clear.home:22",
        has_saved_password: true,
        meta: {
          Authentication: "Password",
        },
      }),
    );
    state.savePassword = false;

    expect(clearHiddenPasswordIDs([state])).toEqual(["preset-clear"]);
  });

  test("buildPresetConfigFromWizardFields maps SSH connection fields", () => {
    const config = buildPresetConfigFromWizardFields("SSH", {
      host: "atlantis.home:22",
      user: "pi",
      authentication: "Private Key",
      credential: "PRIVATE KEY DATA",
      encoding: "utf-8",
      "tab color": "#123456",
    });

    expect(config).toEqual({
      id: "",
      title: "atlantis.home:22",
      type: "SSH",
      host: "atlantis.home:22",
      tab_color: "#123456",
      meta: {
        Host: "atlantis.home:22",
        User: "pi",
        Authentication: "Private Key",
        "Private Key": "PRIVATE KEY DATA",
        Encoding: "utf-8",
      },
    });
  });
});
```

- [ ] **Step 2: Run helper tests and verify they fail**

Run:

```bash
npx vitest run ui/preset_management_test.js
```

Expected: FAIL because `ui/preset_management.js` does not exist and `Preset.hasSavedPassword()` is missing.

- [ ] **Step 3: Extend `Preset` with hidden password metadata**

In `ui/commands/presets.js`, extend `presetItem`:

```js
const presetItem = {
  id: "",
  title: "",
  type: "",
  host: "",
  tab_color: "",
  meta: {},
  has_saved_password: false,
};
```

Add this method to `Preset`:

```js
  /**
   * Return whether the backend says this preset has a hidden saved password.
   *
   * @returns {boolean}
   */
  hasSavedPassword() {
    return this.preset.has_saved_password;
  }
```

Keep `toConfig()` returning only backend-editable fields:

```js
    return {
      id: this.preset.id,
      title: this.preset.title,
      type: this.preset.type,
      host: this.preset.host,
      tab_color: this.preset.tab_color,
      meta,
    };
```

- [ ] **Step 4: Implement pure preset management helpers**

Create `ui/preset_management.js`:

```js
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

const editableMetaKeys = new Set([
  "Authentication",
  "ET Command",
  "ET Server Port",
  "Encoding",
  "Host",
  "Mosh Server",
  "Private Key",
  "User",
]);

export function canManagePresets(policy) {
  return !!policy && policy.can_manage === true;
}

export function requiresAdminKey(policy) {
  return !!policy && policy.requires_admin_key === true;
}

function presetValue(preset, method, defaultValue) {
  return preset && typeof preset[method] === "function" ? preset[method]() : defaultValue;
}

function editableMetaFromPreset(preset) {
  const meta = {};
  if (!preset) {
    return meta;
  }
  for (const key of preset.metaKeys()) {
    if (!editableMetaKeys.has(key)) {
      continue;
    }
    meta[key] = preset.metaDefault(key, "");
  }
  return meta;
}

export function buildEditorState(preset, defaults = {}) {
  const meta = preset ? editableMetaFromPreset(preset) : { ...(defaults.meta || {}) };
  delete meta.Fingerprint;
  delete meta.Password;
  delete meta["Encrypted Password"];

  const privateKey = meta["Private Key"] || "";
  const hasSavedPassword =
    preset && typeof preset.hasSavedPassword === "function"
      ? preset.hasSavedPassword()
      : false;

  return {
    id: presetValue(preset, "id", defaults.id || ""),
    title: presetValue(preset, "title", defaults.title || defaults.host || "New preset"),
    type: presetValue(preset, "type", defaults.type || "SSH"),
    host: presetValue(preset, "host", defaults.host || meta.Host || ""),
    tabColor: presetValue(preset, "tabColor", defaults.tab_color || ""),
    meta,
    password: "",
    savePassword: hasSavedPassword,
    hasSavedPassword,
    privateKey,
    savePrivateKey: privateKey.length > 0,
    confirmDelete: false,
    error: "",
  };
}

export function buildPresetConfigFromEditorState(state) {
  const meta = { ...state.meta };
  delete meta.Fingerprint;
  delete meta.Password;
  delete meta["Encrypted Password"];

  if (state.savePassword && state.password.length > 0) {
    meta.Password = state.password;
  }
  if (state.savePrivateKey && state.privateKey.length > 0) {
    meta["Private Key"] = state.privateKey;
  } else {
    delete meta["Private Key"];
  }
  if (state.host.length > 0) {
    meta.Host = state.host;
  }

  return {
    id: state.id,
    title: state.title,
    type: state.type,
    host: state.host,
    tab_color: state.tabColor,
    meta,
  };
}

export function clearHiddenPasswordIDs(states) {
  return states
    .filter((state) => state.id.length > 0 && state.hasSavedPassword && !state.savePassword)
    .map((state) => state.id);
}

export function buildPresetConfigFromWizardFields(type, fields) {
  const host = fields.host || "";
  const authentication = fields.authentication || "";
  const credential = fields.credential || "";
  const meta = {
    Host: host,
  };

  if (fields.user) {
    meta.User = fields.user;
  }
  if (authentication) {
    meta.Authentication = authentication;
  }
  if (fields.encoding) {
    meta.Encoding = fields.encoding;
  }
  if (type === "Mosh" && fields["mosh server"]) {
    meta["Mosh Server"] = fields["mosh server"];
  }
  if (type === "ET") {
    if (fields["et server port"]) {
      meta["ET Server Port"] = fields["et server port"];
    }
    if (fields["et command"]) {
      meta["ET Command"] = fields["et command"];
    }
  }
  if (authentication === "Password") {
    meta.Password = credential;
  }
  if (authentication === "Private Key") {
    meta["Private Key"] = credential;
  }

  return {
    id: "",
    title: host || "New preset",
    type,
    host,
    tab_color: fields["tab color"] || "",
    meta,
  };
}
```

- [ ] **Step 5: Run helper tests and verify they pass**

Run:

```bash
npx vitest run ui/preset_management_test.js ui/commands/presets_test.js
```

Expected: PASS.

- [ ] **Step 6: Commit frontend model helpers**

Run:

```bash
git add ui/commands/presets.js ui/preset_management.js ui/preset_management_test.js
git commit -m "feat: add preset management helpers"
```

Expected: commit succeeds.

## Task 4: App Preset Write API And AdminKey Cache

**Files:**
- Modify: `ui/app.js`
- Create: `ui/app_preset_management_test.js`

- [ ] **Step 1: Write source-level tests for app wiring**

Create `ui/app_preset_management_test.js`:

```js
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, test } from "vitest";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

function readProjectFile(relativePath) {
  return readFileSync(path.join(repoRoot, relativePath), "utf8");
}

describe("app preset management wiring", () => {
  test("passes policy and write callbacks to home", () => {
    const source = readProjectFile("ui/app.js");

    expect(source).toContain(':preset-management-policy="presetData.management"');
    expect(source).toContain(':save-preset-config="savePresetConfig"');
    expect(source).toContain(':admin-key-required="presetAdminKeyRequired"');
  });

  test("keeps AdminKey in page memory only", () => {
    const source = readProjectFile("ui/app.js");

    expect(source).toContain("presetAdminPassphrase: \"\"");
    expect(source).toContain("this.presetAdminPassphrase = adminKey;");
    expect(source).not.toContain("localStorage");
    expect(source).not.toContain("sessionStorage");
  });

  test("full preset writes preserve hidden passwords and send clear IDs", () => {
    const source = readProjectFile("ui/app.js");

    expect(source).toContain('headers["X-Preserve-Hidden-Preset-Passwords"] = "yes";');
    expect(source).toContain('headers["X-Clear-Hidden-Preset-Passwords"] = clearPasswordIDs.join(",");');
  });
});
```

- [ ] **Step 2: Run app wiring tests and verify they fail**

Run:

```bash
npx vitest run ui/app_preset_management_test.js
```

Expected: FAIL because the new props and methods are not wired.

- [ ] **Step 3: Wire policy and write callbacks into the root template**

In `ui/app.js`, extend `mainTemplate` home props:

```js
  :preset-management-policy="presetData.management"
  :save-preset-config="savePresetConfig"
  :admin-key-required="presetAdminKeyRequired"
```

Extend `presetData` defaults in `data()`:

```js
        presetData: {
          presets: markRaw(new Presets([])),
          restricted: false,
          writable: false,
          management: {
            writable: false,
            can_manage: false,
            requires_admin_key: false,
            blocked_by_preset_restriction: false,
          },
        },
        presetAdminPassphrase: "",
```

In `executeHomeApp`, parse policy:

```js
        const presetManagement = authData.preset_management
          ? authData.preset_management
          : {
              writable: authData.preset_config_writable === true,
              can_manage:
                authData.preset_config_writable === true &&
                !authResult.onlyAllowPresetRemotes,
              requires_admin_key: false,
              blocked_by_preset_restriction:
                authResult.onlyAllowPresetRemotes === true,
            };

        this.presetData = {
          presets: markRaw(
            new Presets(authData.presets ? authData.presets : []),
          ),
          restricted: authResult.onlyAllowPresetRemotes,
          writable: authData.preset_config_writable === true,
          management: presetManagement,
        };
```

In `replacePresetData`, preserve `management`:

```js
        this.presetData = {
          presets: markRaw(new Presets(updatedPresets)),
          restricted: this.presetData.restricted,
          writable: this.presetData.writable,
          management: this.presetData.management,
        };
```

- [ ] **Step 4: Add AdminKey-aware header helpers and full-list save method**

In `methods`, add:

```js
      presetAdminKeyRequired() {
        return (
          this.presetData.management &&
          this.presetData.management.requires_admin_key === true &&
          this.presetAdminPassphrase.length <= 0
        );
      },
      async presetConfigHeadersForPassphrase(passphrase) {
        let headers = {
          "Content-Type": "application/json",
        };

        if (passphrase.length <= 0 || !this.key) {
          headers["X-Key"] = "";

          return headers;
        }

        const authKey = await this.getSocketAuthKey(passphrase);
        headers["X-Key"] = btoa(String.fromCharCode.apply(null, authKey));

        return headers;
      },
      async savePresetConfig(updatedPresets, options = {}) {
        if (!this.presetData.writable) {
          throw new Error("Preset config is not writable");
        }

        const adminKey =
          typeof options.adminKey === "string" && options.adminKey.length > 0
            ? options.adminKey
            : this.presetAdminPassphrase;
        const clearPasswordIDs = Array.isArray(options.clearPasswordIDs)
          ? options.clearPasswordIDs
          : [];
        const headers = await this.presetConfigHeadersForPassphrase(
          adminKey || this.presetConfigPassphrase,
        );
        headers["X-Preserve-Hidden-Preset-Passwords"] = "yes";
        if (clearPasswordIDs.length > 0) {
          headers["X-Clear-Hidden-Preset-Passwords"] =
            clearPasswordIDs.join(",");
        }

        const putResponse = await xhr.put(
          presetConfigInterface,
          headers,
          JSON.stringify({ presets: updatedPresets }),
        );
        if (putResponse.status !== 200) {
          throw new Error("Preset config write failed: " + putResponse.status);
        }

        if (typeof options.adminKey === "string" && options.adminKey.length > 0) {
          this.presetAdminPassphrase = options.adminKey;
        }

        const body = JSON.parse(putResponse.responseText);
        const responsePresets = body.presets ? body.presets : [];
        this.replacePresetData(responsePresets);

        return responsePresets;
      },
```

Update `presetConfigHeaders()` to delegate to `presetConfigHeadersForPassphrase(this.presetConfigPassphrase)`.

- [ ] **Step 5: Run app wiring tests**

Run:

```bash
npx vitest run ui/app_preset_management_test.js
```

Expected: PASS.

- [ ] **Step 6: Commit app preset API wiring**

Run:

```bash
git add ui/app.js ui/app_preset_management_test.js
git commit -m "feat: wire preset management API"
```

Expected: commit succeeds.

## Task 5: Preset Editor Component

**Files:**
- Create: `ui/widgets/preset_editor.vue`
- Create: `ui/widgets/preset_editor.css`
- Modify: `ui/preset_management_test.js`

- [ ] **Step 1: Add tests for editor payload helpers used by the component**

Append to `ui/preset_management_test.js`:

```js
test("buildPresetConfigFromEditorState keeps replacement password only when entered", () => {
  const state = buildEditorState(null, {
    title: "Atlantis",
    type: "SSH",
    host: "atlantis.home:22",
    meta: {
      Authentication: "Password",
      User: "pi",
    },
  });
  state.savePassword = true;
  state.password = "new-password";

  const config = buildPresetConfigFromEditorState(state);

  expect(config.meta.Password).toBe("new-password");
});

test("buildPresetConfigFromEditorState omits private key when unchecked", () => {
  const state = buildEditorState(null, {
    type: "SSH",
    host: "atlantis.home:22",
    meta: {
      Authentication: "Private Key",
      "Private Key": "PRIVATE KEY DATA",
    },
  });
  state.savePrivateKey = false;

  const config = buildPresetConfigFromEditorState(state);

  expect(config.meta["Private Key"]).toBeUndefined();
});
```

- [ ] **Step 2: Run editor helper tests**

Run:

```bash
npx vitest run ui/preset_management_test.js
```

Expected: PASS if Task 3 helpers already support these cases; FAIL if the helper needs adjustment. If it fails, update `buildPresetConfigFromEditorState` exactly as shown in Task 3 Step 4.

- [ ] **Step 3: Create the editor component**

Create `ui/widgets/preset_editor.vue`:

```vue
<!--
Copyright (C) 2026 Snuffy2
SPDX-License-Identifier: AGPL-3.0-only
-->

<template>
  <form id="preset-editor" class="form1" action="javascript:;" @submit.prevent="save">
    <h2>{{ mode === "create" ? "Save preset" : "Edit preset" }}</h2>

    <fieldset id="preset-editor-fields" :disabled="submitting">
      <label class="field">
        Preset name
        <input v-model="localState.title" type="text" autocomplete="off" />
      </label>

      <label class="field">
        Type
        <select v-model="localState.type">
          <option value="SSH">SSH</option>
          <option value="Mosh">Mosh</option>
          <option value="ET">ET</option>
          <option value="Telnet">Telnet</option>
        </select>
      </label>

      <label class="field">
        Host
        <input v-model="localState.host" type="text" autocomplete="off" />
      </label>

      <label class="field">
        Tab color
        <input v-model="localState.tabColor" type="text" autocomplete="off" placeholder="#1f8acb" />
      </label>

      <label v-if="usesUser" class="field">
        User
        <input v-model="localState.meta.User" type="text" autocomplete="off" />
      </label>

      <label v-if="usesAuthentication" class="field">
        Authentication
        <select v-model="localState.meta.Authentication">
          <option value="Password">Password</option>
          <option value="Private Key">Private Key</option>
          <option value="None">None</option>
        </select>
      </label>

      <label v-if="usesPassword" class="field horizontal">
        <input v-model="localState.savePassword" type="checkbox" />
        {{ localState.hasSavedPassword ? "Keep saved password" : "Save password" }}
      </label>

      <label v-if="usesPassword && localState.savePassword" class="field">
        {{ localState.hasSavedPassword ? "Replacement password" : "Password" }}
        <input v-model="localState.password" type="password" autocomplete="off" />
      </label>

      <label v-if="usesPrivateKey" class="field horizontal">
        <input v-model="localState.savePrivateKey" type="checkbox" />
        Save private key
      </label>

      <label v-if="usesPrivateKey && localState.savePrivateKey" class="field">
        Private Key
        <textarea v-model="localState.privateKey" autocomplete="off"></textarea>
      </label>

      <label class="field">
        Encoding
        <input v-model="localState.meta.Encoding" type="text" autocomplete="off" placeholder="utf-8" />
      </label>

      <label v-if="localState.type === 'Mosh'" class="field">
        Mosh Server
        <input v-model="localState.meta['Mosh Server']" type="text" autocomplete="off" placeholder="mosh-server" />
      </label>

      <label v-if="localState.type === 'ET'" class="field">
        ET Server Port
        <input v-model="localState.meta['ET Server Port']" type="text" autocomplete="off" placeholder="2022" />
      </label>

      <label v-if="localState.type === 'ET'" class="field">
        ET Command
        <input v-model="localState.meta['ET Command']" type="text" autocomplete="off" placeholder="et" />
      </label>

      <div v-if="error.length > 0" id="preset-editor-error">{{ error }}</div>

      <div v-if="confirmingDelete" id="preset-editor-confirm-delete">
        <p>Delete preset "{{ localState.title }}"?</p>
        <button type="button" @click="confirmDelete">Delete preset</button>
        <button type="button" class="secondary" @click="confirmingDelete = false">Cancel</button>
      </div>

      <div v-else-if="promptingAdminKey" id="preset-editor-admin-key">
        <label class="field">
          AdminKey
          <input v-model="adminKey" type="password" autocomplete="off" autofocus />
        </label>
        <button type="button" @click="submitAdminKey">Continue</button>
        <button type="button" class="secondary" @click="promptingAdminKey = false">Cancel</button>
      </div>

      <div v-else class="field preset-editor-actions">
        <button type="submit">{{ mode === "create" ? "Save preset" : "Save" }}</button>
        <button v-if="mode === 'edit'" type="button" class="secondary" @click="requestDelete">Delete</button>
        <button type="button" class="secondary" @click="$emit('cancel')">Cancel</button>
      </div>
    </fieldset>
  </form>
</template>

<script>
import "./preset_editor.css";

import {
  buildPresetConfigFromEditorState,
  requiresAdminKey,
} from "../preset_management.js";

export default {
  props: {
    mode: {
      type: String,
      default: "create",
    },
    state: {
      type: Object,
      required: true,
    },
    policy: {
      type: Object,
      default: () => null,
    },
    adminKeyCached: {
      type: Boolean,
      default: false,
    },
    savePreset: {
      type: Function,
      required: true,
    },
    deletePreset: {
      type: Function,
      required: true,
    },
  },
  emits: ["cancel"],
  data() {
    return {
      localState: structuredClone(this.state),
      submitting: false,
      error: "",
      promptingAdminKey: false,
      confirmingDelete: false,
      pendingAction: null,
      adminKey: "",
    };
  },
  computed: {
    usesAuthentication() {
      return ["SSH", "Mosh", "ET"].includes(this.localState.type);
    },
    usesUser() {
      return ["SSH", "Mosh", "ET"].includes(this.localState.type);
    },
    usesPassword() {
      return this.usesAuthentication && this.localState.meta.Authentication === "Password";
    },
    usesPrivateKey() {
      return this.usesAuthentication && this.localState.meta.Authentication === "Private Key";
    },
  },
  methods: {
    async runProtected(action) {
      this.error = "";
      if (requiresAdminKey(this.policy) && !this.adminKeyCached && this.adminKey.length <= 0) {
        this.pendingAction = action;
        this.promptingAdminKey = true;
        return;
      }
      this.submitting = true;
      try {
        await action(this.adminKey);
        this.adminKey = "";
      } catch (e) {
        this.error = String(e);
      } finally {
        this.submitting = false;
      }
    },
    save() {
      const config = buildPresetConfigFromEditorState(this.localState);
      return this.runProtected((adminKey) =>
        this.savePreset({
          config,
          state: this.localState,
          adminKey,
        }),
      );
    },
    requestDelete() {
      this.confirmingDelete = true;
    },
    confirmDelete() {
      this.confirmingDelete = false;
      return this.runProtected((adminKey) =>
        this.deletePreset({
          id: this.localState.id,
          state: this.localState,
          adminKey,
        }),
      );
    },
    submitAdminKey() {
      const action = this.pendingAction;
      this.pendingAction = null;
      this.promptingAdminKey = false;
      if (action === null) {
        return;
      }
      return this.runProtected(action);
    },
  },
};
</script>
```

- [ ] **Step 4: Add editor CSS**

Create `ui/widgets/preset_editor.css`:

```css
/*
 * Copyright (C) 2026 Snuffy2
 * SPDX-License-Identifier: AGPL-3.0-only
 */

#preset-editor {
  background: #3a3a3a;
  color: #ddd;
  padding: 0 20px 20px;
}

#preset-editor h2 {
  color: #ccc;
  font-size: 1.3em;
  margin: 0 0 14px;
}

#preset-editor-fields {
  border: 0;
  margin: 0;
  padding: 0;
}

#preset-editor .field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0 0 12px;
}

#preset-editor .field.horizontal {
  align-items: center;
  flex-direction: row;
}

#preset-editor input,
#preset-editor select,
#preset-editor textarea {
  background: #2d2d2d;
  border: 1px solid #666;
  border-radius: 3px;
  color: #fff;
  font: inherit;
  padding: 8px;
}

#preset-editor textarea {
  min-height: 120px;
  resize: vertical;
}

.preset-editor-actions {
  flex-direction: row;
}

#preset-editor button {
  background: #575757;
  border: 1px solid #777;
  border-radius: 3px;
  color: #fff;
  cursor: pointer;
  font: inherit;
  padding: 7px 12px;
}

#preset-editor button.secondary {
  background: #444;
}

#preset-editor-error {
  background: #733;
  color: #fff;
  margin: 0 0 12px;
  padding: 10px;
}
```

- [ ] **Step 5: Run focused frontend checks**

Run:

```bash
npx vitest run ui/preset_management_test.js
npx eslint ui/widgets/preset_editor.vue ui/widgets/preset_editor.css ui/preset_management.js
```

Expected: Vitest PASS. ESLint PASS for `.vue` and `.js`; if ESLint rejects the `.css` argument, rerun without CSS:

```bash
npx eslint ui/widgets/preset_editor.vue ui/preset_management.js
```

- [ ] **Step 6: Commit editor component**

Run:

```bash
git add ui/widgets/preset_editor.vue ui/widgets/preset_editor.css ui/preset_management_test.js
git commit -m "feat: add preset editor component"
```

Expected: commit succeeds.

## Task 6: Known Preset Edit And Delete Wiring

**Files:**
- Modify: `ui/widgets/connect_known.vue`
- Modify: `ui/widgets/connect_known.css`
- Modify: `ui/widgets/connect.vue`
- Modify: `ui/home.vue`
- Create: `ui/preset_management_wiring_test.js`

- [ ] **Step 1: Write source-level tests for known-list edit controls**

Create `ui/preset_management_wiring_test.js`:

```js
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, test } from "vitest";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

function readProjectFile(relativePath) {
  return readFileSync(path.join(repoRoot, relativePath), "utf8");
}

describe("preset management UI wiring", () => {
  test("known preset list has accessible icon edit button", () => {
    const source = readProjectFile("ui/widgets/connect_known.vue");

    expect(source).toContain("canManagePresets");
    expect(source).toContain('aria-label="Edit preset"');
    expect(source).toContain('title="Edit preset"');
    expect(source).toContain("@click.stop=\"editPreset(preset)\"");
    expect(source).toContain('"edit-preset"');
  });

  test("connect widget renders preset editor mode", () => {
    const source = readProjectFile("ui/widgets/connect.vue");

    expect(source).toContain("preset-editor");
    expect(source).toContain(":save-preset=\"presetSaveHandler\"");
    expect(source).toContain(":delete-preset=\"presetDeleteHandler\"");
  });

  test("home updates full preset list for save and delete", () => {
    const source = readProjectFile("ui/home.vue");

    expect(source).toContain("openPresetEditor(preset)");
    expect(source).toContain("savePresetFromEditor(payload)");
    expect(source).toContain("deletePresetFromEditor(payload)");
    expect(source).toContain("clearHiddenPasswordIDs");
  });
});
```

- [ ] **Step 2: Run wiring tests and verify they fail**

Run:

```bash
npx vitest run ui/preset_management_wiring_test.js
```

Expected: FAIL because edit wiring is missing.

- [ ] **Step 3: Add edit event to known preset list**

In `ui/widgets/connect_known.vue`, add prop:

```js
    canManagePresets: {
      type: Boolean,
      default: false,
    },
```

Extend `emits`:

```js
  emits: ["select-preset", "edit-preset", "refresh-presets"],
```

Add the button after `<h4>`:

```vue
              <button
                v-if="canManagePresets"
                type="button"
                class="preset-edit-button icon icon-pencil"
                aria-label="Edit preset"
                title="Edit preset"
                @click.stop="editPreset(preset)"
              ></button>
```

Add method:

```js
    editPreset(preset) {
      this.$emit("edit-preset", preset);
    },
```

If the icon font does not include `icon-pencil`, use text `✎` inside the button and keep the same accessible label.

- [ ] **Step 4: Add known-list edit CSS**

In `ui/widgets/connect_known.css`, replace the `h4` block with a flex-safe layout:

```css
#connect-known-list-presets li > .lst-wrap {
  align-items: center;
  cursor: pointer;
  display: flex;
  gap: 8px;
  border-radius: 0 3px 3px 3px;
  margin: 12px 10px 10px 0;
  padding: 10px;
}

#connect-known-list-presets li > .lst-wrap > h4 {
  flex: 1;
  font-size: 1.3em;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.preset-edit-button {
  align-items: center;
  background: #575757;
  border: 1px solid #777;
  border-radius: 3px;
  color: #fff;
  cursor: pointer;
  display: inline-flex;
  flex: 0 0 auto;
  height: 30px;
  justify-content: center;
  width: 30px;
}
```

- [ ] **Step 5: Route edit events through `connect.vue`**

In `ui/widgets/connect.vue`, import and register `PresetEditor`:

```js
import PresetEditor from "./preset_editor.vue";
```

```js
    "preset-editor": PresetEditor,
```

Add props:

```js
    canManagePresets: {
      type: Boolean,
      default: false,
    },
    presetEditor: {
      type: Object,
      default: () => null,
    },
    presetManagementPolicy: {
      type: Object,
      default: () => null,
    },
    presetSaveHandler: {
      type: Function,
      required: true,
    },
    presetDeleteHandler: {
      type: Function,
      required: true,
    },
    adminKeyCached: {
      type: Boolean,
      default: false,
    },
```

Pass `canManagePresets` into `connect-known` and route events:

```vue
        :can-manage-presets="canManagePresets"
        @edit-preset="editPreset"
```

Render the editor before `connect-switch`:

```vue
      <preset-editor
        v-if="presetEditor !== null && !inputting"
        :mode="presetEditor.mode"
        :state="presetEditor.state"
        :policy="presetManagementPolicy"
        :admin-key-cached="adminKeyCached"
        :save-preset="presetSaveHandler"
        :delete-preset="presetDeleteHandler"
        @cancel="cancelPresetEditor"
      ></preset-editor>
```

Hide normal tabs while editor is open:

```vue
      <connect-switch
        v-if="presetEditor === null && !inputting"
```

Add emits:

```js
  emits: [
    "display",
    "connector-select",
    "preset-select",
    "preset-edit",
    "preset-editor-cancel",
    "refresh-presets",
  ],
```

Add methods:

```js
    editPreset(preset) {
      if (this.inputting) {
        return;
      }
      this.$emit("preset-edit", preset);
    },
    cancelPresetEditor() {
      this.$emit("preset-editor-cancel");
    },
```

- [ ] **Step 6: Own editor state and save/delete in `home.vue`**

In `ui/home.vue`, import helpers:

```js
import {
  buildEditorState,
  canManagePresets,
  clearHiddenPasswordIDs,
} from "./preset_management.js";
```

Add props:

```js
    presetManagementPolicy: {
      type: Object,
      default: () => null,
    },
    savePresetConfig: {
      type: Function,
      default: () => null,
    },
    adminKeyRequired: {
      type: Function,
      default: () => true,
    },
```

Add data:

```js
      presetEditor: null,
```

Add computed:

```js
    canManagePresets() {
      return canManagePresets(this.presetManagementPolicy);
    },
    adminKeyCached() {
      return !this.adminKeyRequired();
    },
```

Pass props/events to `connect-widget`:

```vue
      :can-manage-presets="canManagePresets"
      :preset-editor="presetEditor"
      :preset-management-policy="presetManagementPolicy"
      :admin-key-cached="adminKeyCached"
      :preset-save-handler="savePresetFromEditor"
      :preset-delete-handler="deletePresetFromEditor"
      @preset-edit="openPresetEditor"
      @preset-editor-cancel="presetEditor = null"
```

Add methods:

```js
    rawPresetConfigs() {
      return this.presetData.toConfig();
    },
    openPresetEditor(preset) {
      if (!this.canManagePresets) {
        return;
      }
      this.presetEditor = {
        mode: "edit",
        presetID: preset.preset.id(),
        state: buildEditorState(preset.preset),
      };
    },
    async savePresetFromEditor(payload) {
      if (!this.savePresetConfig || this.presetEditor === null) {
        return;
      }
      const configs = this.rawPresetConfigs();
      const index = configs.findIndex((preset) => preset.id === payload.config.id);
      if (index >= 0) {
        configs[index] = payload.config;
      } else {
        configs.push(payload.config);
      }
      const clearIDs = clearHiddenPasswordIDs([payload.state]);
      const updatedPresets = await this.savePresetConfig(configs, {
        adminKey: payload.adminKey,
        clearPasswordIDs: clearIDs,
      });
      this.replacePresets(updatedPresets);
      this.presetEditor = null;
    },
    async deletePresetFromEditor(payload) {
      if (!this.savePresetConfig || this.presetEditor === null) {
        return;
      }
      const configs = this.rawPresetConfigs().filter(
        (preset) => preset.id !== payload.id,
      );
      const updatedPresets = await this.savePresetConfig(configs, {
        adminKey: payload.adminKey,
      });
      this.replacePresets(updatedPresets);
      this.presetEditor = null;
    },
```

- [ ] **Step 7: Run wiring tests**

Run:

```bash
npx vitest run ui/preset_management_wiring_test.js ui/preset_management_test.js
```

Expected: PASS.

- [ ] **Step 8: Commit known preset management wiring**

Run:

```bash
git add ui/widgets/connect_known.vue ui/widgets/connect_known.css ui/widgets/connect.vue ui/home.vue ui/preset_management_wiring_test.js
git commit -m "feat: wire preset edit and delete UI"
```

Expected: commit succeeds.

## Task 7: Save-As From New Remote Prompts

**Files:**
- Modify: `ui/commands/commands.js`
- Modify: `ui/commands/ssh.js`
- Modify: `ui/commands/mosh.js`
- Modify: `ui/commands/et.js`
- Modify: `ui/commands/telnet.js`
- Modify: `ui/home.vue`
- Modify: `ui/widgets/connector.vue`
- Modify: related command tests under `ui/commands/*_test.js`

- [ ] **Step 1: Write tests for prompt actions staying open**

Append to `ui/commands/commands_test.js`:

```js
  it("preserves secondary action options on prompt steps", () => {
    const step = command.prompt(
      "Title",
      "Message",
      "Connect",
      () => {},
      () => {},
      [],
      [
        {
          text: "Save as preset",
          keepOpen: true,
          respond() {},
        },
      ],
    );

    const next = new command.Next(step);

    expect(next.data().actions()[0].text).toBe("Save as preset");
    expect(next.data().actions()[0].keepOpen).toBe(true);
  });
```

If `Next` is not exported, export it from `ui/commands/commands.js`:

```js
export class Next {
```

- [ ] **Step 2: Run command action test**

Run:

```bash
npx vitest run ui/commands/commands_test.js
```

Expected: FAIL if `Next` is not exported or action metadata is not retained.

- [ ] **Step 3: Preserve prompt after save-as action**

In `ui/widgets/connector.vue`, update `runAction` finally block:

```js
      } finally {
        if (this.current === current && !this.disabled) {
          current.submitting = false;
        }
      }
```

This already keeps the prompt open. If the implementation disabled the prompt in a previous task, remove that disabling. The test locks action metadata; the existing `runAction` shape is compatible.

- [ ] **Step 4: Pass save-as callback from `home.vue` into new connection wizards**

In `connectNew(connector)`, define callback before `connector.wizard(...)`:

```js
        const saveAsPreset = (type, fields) => {
          this.openNewPresetEditor(type, fields);
        };
```

Pass it as a new final argument:

```js
              saveAsPreset,
```

Add method:

```js
    openNewPresetEditor(type, fields) {
      if (!this.canManagePresets) {
        return;
      }
      const config = buildPresetConfigFromWizardFields(type, fields);
      this.presetEditor = {
        mode: "create",
        presetID: "",
        state: buildEditorState(null, config),
      };
      this.connector.inputting = false;
      this.connector.acquired = false;
    },
```

Import `buildPresetConfigFromWizardFields` from `./preset_management.js`.

- [ ] **Step 5: Add save-as action to each first connection prompt**

For each command class, extend constructor parameters to accept `saveAsPreset = null`, store `this.saveAsPreset = saveAsPreset`, and add an action to the first prompt when it exists.

Use this helper shape inside each file:

```js
  saveAsPresetActions(type) {
    if (typeof this.saveAsPreset !== "function") {
      return [];
    }
    return [
      {
        text: "Save as preset",
        keepOpen: true,
        respond: (fields) => this.saveAsPreset(type, fields),
      },
    ];
  }
```

When building the initial `command.prompt(...)`, pass actions as the final argument:

```js
      this.saveAsPresetActions("SSH"),
```

Use the matching type string in each file:

```js
this.saveAsPresetActions("SSH")
this.saveAsPresetActions("Mosh")
this.saveAsPresetActions("ET")
this.saveAsPresetActions("Telnet")
```

Do not add the action to preset-based wizards where fields are readonly. Only add it for new remote prompts where `preset` is `null` or `presets.emptyPreset()`.

- [ ] **Step 6: Add tests for one command and one pure conversion per protocol**

In `ui/preset_management_test.js`, add:

```js
test("buildPresetConfigFromWizardFields maps Telnet fields", () => {
  const config = buildPresetConfigFromWizardFields("Telnet", {
    host: "router.home:23",
    encoding: "utf-8",
    "tab color": "#abcdef",
  });

  expect(config.type).toBe("Telnet");
  expect(config.host).toBe("router.home:23");
  expect(config.meta).toEqual({
    Host: "router.home:23",
    Encoding: "utf-8",
  });
});

test("buildPresetConfigFromWizardFields maps Mosh fields", () => {
  const config = buildPresetConfigFromWizardFields("Mosh", {
    host: "columbia.home:22",
    user: "pi",
    authentication: "Password",
    credential: "pw",
    encoding: "utf-8",
    "mosh server": "mosh-server",
  });

  expect(config.type).toBe("Mosh");
  expect(config.meta.Password).toBe("pw");
  expect(config.meta["Mosh Server"]).toBe("mosh-server");
});

test("buildPresetConfigFromWizardFields maps ET fields", () => {
  const config = buildPresetConfigFromWizardFields("ET", {
    host: "et.home:22",
    user: "pi",
    authentication: "Private Key",
    credential: "KEY",
    encoding: "utf-8",
    "et server port": "2022",
    "et command": "et",
  });

  expect(config.type).toBe("ET");
  expect(config.meta["Private Key"]).toBe("KEY");
  expect(config.meta["ET Server Port"]).toBe("2022");
  expect(config.meta["ET Command"]).toBe("et");
});
```

- [ ] **Step 7: Run save-as tests**

Run:

```bash
npx vitest run ui/commands/commands_test.js ui/preset_management_test.js ui/commands/ssh_test.js ui/commands/mosh_test.js ui/commands/et_test.js ui/commands/telnet_test.js
```

Expected: PASS.

- [ ] **Step 8: Commit save-as flow**

Run:

```bash
git add ui/commands/commands.js ui/commands/commands_test.js ui/commands/ssh.js ui/commands/mosh.js ui/commands/et.js ui/commands/telnet.js ui/home.vue ui/widgets/connector.vue ui/preset_management_test.js ui/commands/ssh_test.js ui/commands/mosh_test.js ui/commands/et_test.js ui/commands/telnet_test.js
git commit -m "feat: add save-as preset flow"
```

Expected: commit succeeds.

## Task 8: Documentation And Validation

**Files:**
- Modify: `README.md`
- Modify: `CONFIGURATION.md`

- [ ] **Step 1: Update README preset management note**

In `README.md`, replace the existing sentence that says there is not yet a separate UI prompt for `AdminKey` with:

```md
Full preset add/edit/remove API writes require admin access. The UI prompts for
`AdminKey` before the first protected preset create, edit, or delete action and
remembers a successful AdminKey only for the current browser page session. If
`AdminKey` is blank, anyone with UI access can manage presets. If both
`SharedKey` and `AdminKey` are blank, everyone gets admin access without
authentication. Fingerprint saves remain available only from the connection-time
fingerprint prompt.
```

- [ ] **Step 2: Update CONFIGURATION preset management section**

In `CONFIGURATION.md`, update the `Preset Management API` notes to state:

```md
The UI supports preset create, edit, and delete when the active configuration is
file-backed and `OnlyAllowPresetRemotes` is false. If `AdminKey` is configured,
the UI prompts for it on the first protected write and caches it in browser
memory until the page reloads. If `AdminKey` is blank, authenticated users are
admin users for preset management. If both `SharedKey` and `AdminKey` are blank,
anonymous visitors can manage presets.

The preset editor never displays hidden saved passwords. It receives a boolean
that a saved password exists and can keep or clear that password on save.
Fingerprint editing is intentionally not part of the preset editor; users can
save fingerprints from the connection-time fingerprint prompt.
```

- [ ] **Step 3: Run targeted backend and frontend tests**

Run:

```bash
go test ./application/controller -v
npx vitest run ui/preset_management_test.js ui/app_preset_management_test.js ui/preset_management_wiring_test.js ui/commands/commands_test.js ui/commands/ssh_test.js ui/commands/mosh_test.js ui/commands/et_test.js ui/commands/telnet_test.js
```

Expected: PASS.

- [ ] **Step 4: Run broader validation**

Run:

```bash
npm run lint
npm run testonly
```

Expected: PASS. `npm run testonly` runs Vitest and `go test ./... -race -timeout 30s`.

- [ ] **Step 5: Run generation if frontend wiring or static assets need build verification**

Run:

```bash
npm run generate
```

Expected: PASS and generated `.tmp/` output remains untracked unless the user explicitly asks to commit generated output.

- [ ] **Step 6: Commit documentation and final validation fixes**

Run:

```bash
git add README.md CONFIGURATION.md
git status --short
git commit -m "docs: describe preset management UI"
```

Expected: only intended docs and any required test/source fixes are staged, and commit succeeds.

## Self-Review Notes

Spec coverage:
- Permission model is covered by Tasks 1, 4, 6, and 8.
- Backend metadata and hidden password boolean are covered by Tasks 1 and 2.
- Preset list edit icon is covered by Task 6.
- New remote save-as is covered by Task 7.
- Preset editor, delete confirmation, and AdminKey prompt are covered by Tasks 5 and 6.
- Secret defaults and fingerprint exclusion are covered by Tasks 3 and 5.
- Error handling is covered by Tasks 4, 5, and 6.
- Testing and docs are covered by all tasks and Task 8.

Implementation constraints:
- Do not alter the connection-time fingerprint save path except to keep it working with the new preset metadata.
- Do not display plaintext or encrypted saved passwords sent from the backend; backend responses expose only `has_saved_password`.
- Use `trash`, not `rm`, for any cleanup.
- Do not commit generated `.tmp/` assets.
