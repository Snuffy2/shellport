// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "vitest";

import { Preset } from "./commands/presets.js";
import {
  authenticationOptionsForType,
  buildEditorState,
  buildPresetConfigFromEditorState,
  buildPresetConfigFromWizardFields,
  canManagePresets,
  cloneEditorState,
  clearHiddenPasswordIDs,
  encodingOptionsForType,
  privateKeyFileLabel,
  typeLocksEncoding,
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
  test("SSH and Telnet expose full encoding choices", () => {
    expect(encodingOptionsForType("SSH")).toContain("utf-8");
    expect(encodingOptionsForType("SSH")).toContain("shift-jis");
    expect(encodingOptionsForType("Telnet")).toEqual(
      encodingOptionsForType("SSH"),
    );
    expect(typeLocksEncoding("SSH")).toBe(false);
  });

  test("Mosh and ET lock encoding to utf-8", () => {
    for (const type of ["Mosh", "ET"]) {
      const state = buildEditorState(null, {
        type,
        meta: { Encoding: "Shift-JIS" },
      });

      expect(encodingOptionsForType(type)).toEqual(["utf-8"]);
      expect(typeLocksEncoding(type)).toBe(true);
      expect(state.meta.Encoding).toBe("utf-8");
    }
  });

  test("private key selector labels file references by filename", () => {
    expect(
      privateKeyFileLabel("file:///config/private_keys/atlantis.key"),
    ).toBe("atlantis.key");
    expect(privateKeyFileLabel("environment://SHELLPORT_TEST_KEY")).toBe(
      "environment://SHELLPORT_TEST_KEY",
    );
  });

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
    expect(state.privateKeyMode).toBe("paste");
  });

  test("defaults new key-capable presets to private key saving", () => {
    for (const type of ["SSH", "Mosh", "ET"]) {
      const state = buildEditorState(null, { type });

      expect(state.meta.Authentication).toBe("Private Key");
      expect(state.savePrivateKey).toBe(true);
      expect(state.privateKeyMode).toBe("existing");
    }
  });

  test("keeps existing preset authentication defaults unchanged", () => {
    const state = buildEditorState(
      new Preset({
        id: "preset-atlantis",
        title: "Atlantis",
        type: "SSH",
        host: "atlantis.home:22",
        meta: {
          Authentication: "Password",
          User: "pi",
        },
      }),
    );

    expect(state.meta.Authentication).toBe("Password");
    expect(state.savePrivateKey).toBe(false);
  });

  test("file-backed private keys default to existing key mode", () => {
    const state = buildEditorState(null, {
      type: "SSH",
      meta: {
        Authentication: "Private Key",
        "Private Key": "file:///config/private_keys/atlantis.key",
      },
    });

    expect(state.savePrivateKey).toBe(true);
    expect(state.privateKeyMode).toBe("existing");
    expect(state.privateKeyFile).toBe(
      "file:///config/private_keys/atlantis.key",
    );
  });

  test("redacted file-backed private keys default to saved existing key mode", () => {
    const state = buildEditorState(
      new Preset({
        id: "preset-atlantis",
        title: "Atlantis",
        type: "SSH",
        host: "atlantis.home:22",
        has_saved_private_key: true,
        private_key_file: "file:///config/private_keys/atlantis.key",
        private_key_filename: "atlantis.key",
        meta: {
          Authentication: "Private Key",
          User: "pi",
        },
      }),
    );

    expect(state.savePrivateKey).toBe(true);
    expect(state.privateKeyMode).toBe("existing");
    expect(state.privateKeyFile).toBe(
      "file:///config/private_keys/atlantis.key",
    );
    expect(state.privateKeyFilename).toBe("atlantis.key");
    expect(state.privateKey).toBe("");
  });

  test("redacted private key filenames show without a full key file reference", () => {
    const state = buildEditorState(
      new Preset({
        id: "preset-atlantis",
        title: "Atlantis",
        type: "SSH",
        host: "atlantis.home:22",
        has_saved_private_key: true,
        private_key_filename: "atlantis.key",
        meta: {
          Authentication: "Private Key",
          User: "pi",
        },
      }),
    );

    expect(state.savePrivateKey).toBe(true);
    expect(state.privateKeyMode).toBe("existing");
    expect(state.privateKeyFile).toBe("");
    expect(state.privateKeyFilename).toBe("atlantis.key");
    expect(state.privateKey).toBe("");
  });

  test("redacted inline private keys default to saved private key without exposing text", () => {
    const state = buildEditorState(
      new Preset({
        id: "preset-atlantis",
        title: "Atlantis",
        type: "SSH",
        host: "atlantis.home:22",
        has_saved_private_key: true,
        meta: {
          Authentication: "Private Key",
          User: "pi",
        },
      }),
    );

    expect(state.savePrivateKey).toBe(true);
    expect(state.privateKeyMode).toBe("existing");
    expect(state.privateKeyFile).toBe("");
    expect(state.privateKeyFilename).toBe("");
    expect(state.privateKey).toBe("");
  });

  test("cloneEditorState works when structuredClone is unavailable", () => {
    const originalStructuredClone = globalThis.structuredClone;
    try {
      globalThis.structuredClone = undefined;
      const state = {
        id: "preset-atlantis",
        title: "Atlantis",
        meta: { User: "pi" },
      };

      const cloned = cloneEditorState(state);
      cloned.meta.User = "root";

      expect(cloned).toEqual({
        id: "preset-atlantis",
        title: "Atlantis",
        meta: { User: "root" },
      });
      expect(state.meta.User).toBe("pi");
    } finally {
      globalThis.structuredClone = originalStructuredClone;
    }
  });

  test("cloneEditorState falls back when structuredClone rejects state", () => {
    const originalStructuredClone = globalThis.structuredClone;
    try {
      globalThis.structuredClone = () => {
        throw new DOMException("cannot clone", "DataCloneError");
      };
      const state = {
        id: "preset-atlantis",
        title: "Atlantis",
        meta: { User: "pi" },
      };

      const cloned = cloneEditorState(state);
      cloned.meta.User = "root";

      expect(cloned.meta.User).toBe("root");
      expect(state.meta.User).toBe("pi");
    } finally {
      globalThis.structuredClone = originalStructuredClone;
    }
  });

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

  test("buildPresetConfigFromEditorState omits blank replacement password", () => {
    const state = buildEditorState(
      new Preset({
        id: "preset-atlantis",
        title: "Atlantis",
        type: "SSH",
        host: "atlantis.home:22",
        has_saved_password: true,
        meta: {
          Authentication: "Password",
          User: "pi",
        },
      }),
    );

    const config = buildPresetConfigFromEditorState(state);

    expect(state.savePassword).toBe(true);
    expect(state.password).toBe("");
    expect(config.meta.Password).toBeUndefined();
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

  test("buildPresetConfigFromEditorState stores existing private key reference", () => {
    const state = buildEditorState(null, {
      type: "SSH",
      host: "atlantis.home:22",
      meta: {
        Authentication: "Private Key",
        "Private Key": "file:///config/private_keys/atlantis.key",
      },
    });

    const config = buildPresetConfigFromEditorState(state);

    expect(config.meta["Private Key"]).toBe(
      "file:///config/private_keys/atlantis.key",
    );
  });

  test("buildPresetConfigFromEditorState omits empty existing private key reference", () => {
    const state = buildEditorState(null, {
      type: "SSH",
      host: "atlantis.home:22",
      meta: {
        Authentication: "Private Key",
      },
    });
    state.privateKeyMode = "existing";
    state.privateKeyFile = "";
    state.savePrivateKey = true;

    const config = buildPresetConfigFromEditorState(state);

    expect(config.meta["Private Key"]).toBeUndefined();
  });

  test("buildPresetConfigFromEditorState drops cleared host and user metadata", () => {
    const state = buildEditorState(
      new Preset({
        id: "preset-atlantis",
        title: "Atlantis",
        type: "SSH",
        host: "atlantis.home:22",
        meta: {
          Host: "atlantis.home:22",
          User: "pi",
          Authentication: "None",
        },
      }),
    );
    state.host = "";
    state.meta.User = "";

    const config = buildPresetConfigFromEditorState(state);

    expect(config.host).toBe("");
    expect(config.meta.Host).toBeUndefined();
    expect(config.meta.User).toBeUndefined();
  });

  test("buildPresetConfigFromEditorState omits password when auth changes away from password", () => {
    const state = buildEditorState(
      new Preset({
        id: "preset-atlantis",
        title: "Atlantis",
        type: "SSH",
        host: "atlantis.home:22",
        has_saved_password: true,
        meta: {
          Authentication: "Password",
          User: "pi",
        },
      }),
    );
    state.meta.Authentication = "Private Key";

    const config = buildPresetConfigFromEditorState(state);

    expect(state.savePassword).toBe(true);
    expect(config.meta.Password).toBeUndefined();
  });

  test("ET presets use private key authentication only", () => {
    const state = buildEditorState(null, {
      type: "ET",
      host: "et.home:22",
      meta: {
        Authentication: "Password",
        Password: "secret",
        "ET Command": "et",
        "ET Server Port": "2022",
      },
    });
    state.savePassword = true;
    state.password = "secret";

    const config = buildPresetConfigFromEditorState(state);

    expect(authenticationOptionsForType("ET")).toEqual(["Private Key"]);
    expect(state.meta.Authentication).toBe("Private Key");
    expect(config.meta.Authentication).toBe("Private Key");
    expect(config.meta.Password).toBeUndefined();
  });

  test("clearHiddenPasswordIDs clears hidden password when auth changes away from password", () => {
    const state = buildEditorState(
      new Preset({
        id: "preset-atlantis",
        title: "Atlantis",
        type: "SSH",
        host: "atlantis.home:22",
        has_saved_password: true,
        meta: {
          Authentication: "Password",
          User: "pi",
        },
      }),
    );
    state.meta.Authentication = "Private Key";

    expect(state.savePassword).toBe(true);
    expect(clearHiddenPasswordIDs([state])).toEqual(["preset-atlantis"]);
  });

  test("buildPresetConfigFromEditorState omits private key when auth changes away from private key", () => {
    const state = buildEditorState(null, {
      type: "SSH",
      host: "atlantis.home:22",
      meta: {
        Authentication: "Private Key",
        "Private Key": "PRIVATE KEY DATA",
      },
    });
    state.meta.Authentication = "None";

    const config = buildPresetConfigFromEditorState(state);

    expect(state.savePrivateKey).toBe(true);
    expect(config.meta["Private Key"]).toBeUndefined();
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

  test("buildPresetConfigFromWizardFields maps Mosh connection fields", () => {
    const config = buildPresetConfigFromWizardFields("Mosh", {
      host: "columbia.home:6000",
      user: "ops",
      authentication: "Password",
      credential: "secret",
      encoding: "utf-8",
      "mosh server": "/usr/local/bin/mosh-server",
      "tab color": "#112233",
    });

    expect(config).toEqual({
      id: "",
      title: "columbia.home:6000",
      type: "Mosh",
      host: "columbia.home:6000",
      tab_color: "#112233",
      meta: {
        Host: "columbia.home:6000",
        User: "ops",
        Authentication: "Password",
        Password: "secret",
        Encoding: "utf-8",
        "Mosh Server": "/usr/local/bin/mosh-server",
      },
    });
  });

  test("buildPresetConfigFromWizardFields maps ET connection fields", () => {
    const config = buildPresetConfigFromWizardFields("ET", {
      host: "et.home:2022",
      user: "agent",
      authentication: "Private Key",
      credential: "PRIVATE KEY DATA",
      encoding: "utf-8",
      "et server port": "2022",
      "et command": "/usr/local/bin/et",
      "tab color": "#445566",
    });

    expect(config).toEqual({
      id: "",
      title: "et.home:2022",
      type: "ET",
      host: "et.home:2022",
      tab_color: "#445566",
      meta: {
        Host: "et.home:2022",
        User: "agent",
        Authentication: "Private Key",
        "Private Key": "PRIVATE KEY DATA",
        Encoding: "utf-8",
        "ET Server Port": "2022",
        "ET Command": "/usr/local/bin/et",
      },
    });
  });

  test("buildPresetConfigFromWizardFields maps Telnet connection fields", () => {
    const config = buildPresetConfigFromWizardFields("Telnet", {
      host: "legacy.home:23",
      encoding: "utf-8",
    });

    expect(config).toEqual({
      id: "",
      title: "legacy.home:23",
      type: "Telnet",
      host: "legacy.home:23",
      tab_color: "",
      meta: {
        Host: "legacy.home:23",
        Encoding: "utf-8",
      },
    });
  });

  test("buildPresetConfigFromWizardFields omits blank host metadata", () => {
    const config = buildPresetConfigFromWizardFields("Telnet", {
      host: "",
      encoding: "utf-8",
    });

    expect(config.host).toBe("");
    expect(config.title).toBe("New preset");
    expect(config.meta.Host).toBeUndefined();
    expect(config.meta.Encoding).toBe("utf-8");
  });
});
