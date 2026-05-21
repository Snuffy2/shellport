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

export function canManagePresets(policy) {
  return !!policy && policy.can_manage === true;
}

export function requiresAdminKey(policy) {
  return !!policy && policy.requires_admin_key === true;
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
  const meta = preset
    ? editableMetaFromPreset(preset)
    : {
        ...(defaults.meta || {}),
      };

  delete meta.Fingerprint;
  delete meta.Password;
  delete meta["Encrypted Password"];
  meta.Authentication = normalizeAuthenticationForType(
    presetValue(preset, "type", defaults.type || "SSH"),
    meta.Authentication || "",
  );

  const privateKey = meta["Private Key"] || "";
  const hasSavedPassword =
    preset && typeof preset.hasSavedPassword === "function"
      ? preset.hasSavedPassword()
      : false;

  return {
    id: presetValue(preset, "id", defaults.id || ""),
    title: presetValue(
      preset,
      "title",
      defaults.title || defaults.host || "New preset",
    ),
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
  const usesAuthentication = typeUsesAuthentication(state.type);
  const authentication = normalizeAuthenticationForType(
    state.type,
    usesAuthentication ? meta.Authentication || "" : "",
  );

  delete meta.Fingerprint;
  delete meta.Password;
  delete meta["Encrypted Password"];
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
    state.savePrivateKey &&
    state.privateKey.length > 0
  ) {
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

export function buildPresetConfigFromWizardFields(type, fields) {
  const normalized = {};
  for (const [key, value] of Object.entries(fields)) {
    normalized[key.toLowerCase()] = value;
  }

  const host = normalized.host || "";
  const authentication = normalized.authentication || "";
  const credential = normalized.credential || "";
  const meta = {
    Host: host,
  };

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
