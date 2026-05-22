// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { charsetPresets } from "./commands/common.js";

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

function typeUsesAuthentication(type) {
  return ["SSH", "Mosh", "ET"].includes(type);
}

export function authenticationOptionsForType(type) {
  if (type === "ET") {
    return ["Private Key"];
  }

  if (typeUsesAuthentication(type)) {
    return ["Password", "Private Key", "None"];
  }

  return [];
}

function normalizeAuthenticationForType(type, authentication) {
  const options = authenticationOptionsForType(type);
  if (options.length === 0) {
    return "";
  }

  if (options.includes(authentication)) {
    return authentication;
  }

  return options[0];
}

export function encodingOptionsForType(type) {
  if (type === "Mosh" || type === "ET") {
    return ["utf-8"];
  }
  return charsetPresets;
}

export function typeLocksEncoding(type) {
  return encodingOptionsForType(type).length === 1;
}

function normalizeEncodingForType(type, encoding) {
  const options = encodingOptionsForType(type);
  if (options.includes(encoding)) {
    return encoding;
  }
  return "utf-8";
}

function privateKeyModeForValue(value) {
  const scheme = uriScheme(value);
  if (scheme === "file" || scheme === "environment") {
    return "existing";
  }
  if (value.length > 0) {
    return "paste";
  }
  return "existing";
}

export function privateKeyFileLabel(value) {
  if (uriScheme(value) !== "file") {
    return value;
  }
  const path = value.slice(value.indexOf("://") + 3);
  const parts = path.split(/[\\/]/).filter((part) => part.length > 0);
  return parts.length > 0 ? parts[parts.length - 1] : value;
}

function uriScheme(value) {
  const schemeIndex = value.indexOf("://");
  return schemeIndex < 0 ? "" : value.slice(0, schemeIndex).toLowerCase();
}

export function canManagePresets(policy) {
  return !!policy && policy.can_manage === true;
}

export function requiresAdminPassword(policy) {
  return !!policy && policy.requires_admin_password === true;
}

function presetValue(preset, method, defaultValue) {
  return preset && typeof preset[method] === "function"
    ? preset[method]()
    : defaultValue;
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
  const isNewPreset = !preset;
  const meta = preset
    ? editableMetaFromPreset(preset)
    : {
        ...(defaults.meta || {}),
      };

  const type = presetValue(preset, "type", defaults.type || "SSH");

  delete meta.Fingerprint;
  delete meta.Password;
  delete meta["Encrypted Password"];
  const defaultAuthentication =
    isNewPreset && typeUsesAuthentication(type) ? "Private Key" : "";
  meta.Authentication = normalizeAuthenticationForType(
    type,
    meta.Authentication || defaultAuthentication,
  );
  meta.Encoding = normalizeEncodingForType(type, meta.Encoding || "utf-8");

  const privateKey = meta["Private Key"] || "";
  const hasSavedPassword =
    preset && typeof preset.hasSavedPassword === "function"
      ? preset.hasSavedPassword()
      : false;
  const hasSavedPrivateKey =
    preset && typeof preset.hasSavedPrivateKey === "function"
      ? preset.hasSavedPrivateKey()
      : false;
  const savedPrivateKeyFile =
    preset && typeof preset.privateKeyFile === "function"
      ? preset.privateKeyFile()
      : "";
  const savedPrivateKeyFilename =
    preset && typeof preset.privateKeyFilename === "function"
      ? preset.privateKeyFilename()
      : "";
  const privateKeyFile =
    uriScheme(privateKey) === "file" || uriScheme(privateKey) === "environment"
      ? privateKey
      : savedPrivateKeyFile;
  const privateKeyFilename =
    privateKeyFile.length > 0
      ? privateKeyFileLabel(privateKeyFile)
      : savedPrivateKeyFilename;

  return {
    id: presetValue(preset, "id", defaults.id || ""),
    title: presetValue(
      preset,
      "title",
      defaults.title || defaults.host || "New preset",
    ),
    type,
    host: presetValue(preset, "host", defaults.host || meta.Host || ""),
    tabColor: presetValue(preset, "tabColor", defaults.tab_color || ""),
    meta,
    password: "",
    savePassword: hasSavedPassword,
    hasSavedPassword,
    privateKey,
    savePrivateKey:
      privateKey.length > 0 ||
      hasSavedPrivateKey ||
      (isNewPreset && meta.Authentication === "Private Key"),
    hasSavedPrivateKey,
    privateKeyMode: privateKeyModeForValue(privateKey),
    privateKeyFile,
    privateKeyFilename,
    confirmDelete: false,
    error: "",
  };
}

export function cloneEditorState(state) {
  if (typeof structuredClone === "function") {
    try {
      return structuredClone(state);
    } catch (_e) {
      // Fall through to JSON cloning for browser runtimes that expose
      // structuredClone but reject this plain editor payload.
    }
  }

  return JSON.parse(JSON.stringify(state));
}

export function buildPresetConfigFromEditorState(state) {
  const meta = { ...state.meta };
  const usesAuthentication = typeUsesAuthentication(state.type);
  meta.Encoding = normalizeEncodingForType(
    state.type,
    meta.Encoding || "utf-8",
  );
  const authentication = normalizeAuthenticationForType(
    state.type,
    usesAuthentication ? meta.Authentication || "" : "",
  );

  delete meta.Fingerprint;
  delete meta.Password;
  delete meta["Encrypted Password"];
  if (meta.User === "") {
    delete meta.User;
  }
  if (authentication.length > 0) {
    meta.Authentication = authentication;
  } else {
    delete meta.Authentication;
  }

  if (
    usesAuthentication &&
    authentication === "Password" &&
    state.savePassword &&
    state.password.length > 0
  ) {
    meta.Password = state.password;
  }
  if (
    usesAuthentication &&
    authentication === "Private Key" &&
    state.savePrivateKey
  ) {
    if (state.privateKeyMode === "existing") {
      if (state.privateKeyFile.length > 0) {
        meta["Private Key"] = state.privateKeyFile;
      } else {
        delete meta["Private Key"];
      }
    } else if (state.privateKey.length > 0) {
      meta["Private Key"] = state.privateKey;
    } else {
      delete meta["Private Key"];
    }
  } else {
    delete meta["Private Key"];
  }
  if (state.host.length > 0) {
    meta.Host = state.host;
  } else {
    delete meta.Host;
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
    .filter((state) => {
      if (state.id.length <= 0 || !state.hasSavedPassword) {
        return false;
      }

      const usesPassword =
        typeUsesAuthentication(state.type) &&
        state.meta.Authentication === "Password";

      return !usesPassword || !state.savePassword;
    })
    .map((state) => state.id);
}

export function clearHiddenPrivateKeyIDs(states) {
  return states
    .filter((state) => {
      if (state.id.length <= 0 || !state.hasSavedPrivateKey) {
        return false;
      }

      const usesPrivateKey =
        typeUsesAuthentication(state.type) &&
        state.meta.Authentication === "Private Key";

      return !usesPrivateKey || !state.savePrivateKey;
    })
    .map((state) => state.id);
}

export function buildPresetConfigFromWizardFields(type, fields) {
  const normalized = {};
  for (const [key, value] of Object.entries(fields)) {
    normalized[key.toLowerCase()] = value;
  }

  const host = normalized.host || "";
  const authentication = normalized.authentication || "";
  const credential = normalized.credential || "";
  const meta = {};

  if (host) {
    meta.Host = host;
  }
  if (normalized.user) {
    meta.User = normalized.user;
  }
  if (authentication) {
    meta.Authentication = authentication;
  }
  if (normalized.encoding) {
    meta.Encoding = normalized.encoding;
  }
  if (type === "Mosh" && normalized["mosh server"]) {
    meta["Mosh Server"] = normalized["mosh server"];
  }
  if (type === "ET") {
    if (normalized["et server port"]) {
      meta["ET Server Port"] = normalized["et server port"];
    }
    if (normalized["et command"]) {
      meta["ET Command"] = normalized["et command"];
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
    tab_color: normalized["tab color"] || "",
    meta,
  };
}
