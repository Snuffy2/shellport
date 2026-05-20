// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * @file Helpers for deciding when preset connections can skip the wizard UI.
 */

/**
 * Returns the preset credential value for the selected auth method.
 *
 * @param {object} presetData Preset accessor object.
 * @param {string} authentication Selected authentication method.
 * @returns {string|null} Credential string, empty when the selected method
 *   needs a later credential prompt, or null when the auth method is invalid.
 */
export function presetCredential(presetData, authentication) {
  switch (authentication) {
    case "Password":
      return presetData.metaDefault("Password", "");

    case "Private Key":
      return presetData.metaDefault("Private Key", "");

    case "None":
      return "";

    default:
      return null;
  }
}

function validETServerPort(portText) {
  if (!/^[0-9]+$/.test(portText)) {
    return false;
  }

  const port = Number.parseInt(portText, 10);
  return port >= 1 && port <= 65535;
}

/**
 * Builds a non-interactive execution payload for presets with complete
 * connection details.
 *
 * Returns null when a preset is missing fields that would force the first
 * connection-details prompt. Missing fingerprints and credentials stay in the
 * execution payload so the command flow can ask only those later prompts.
 *
 * @param {{ command: object, preset: object }} preset Merged preset entry.
 * @returns {{config: object, session: object, keptSessions: Array<string>}|null}
 *   Execution data for `command.execute`, or null to use the interactive wizard.
 */
export function buildPresetExecution(preset) {
  const presetData = preset.preset;
  const commandName = preset.command.name();
  const host = presetData.metaDefault("Host", presetData.host());
  const charset = presetData.metaDefault("Encoding", "utf-8");
  const tabColor = presetData.tabColor();

  if (host.length <= 0) {
    return null;
  }

  if (commandName === "Telnet") {
    return {
      config: {
        host,
        charset,
        tabColor,
      },
      session: {},
      keptSessions: [],
    };
  }

  if (commandName !== "SSH" && commandName !== "Mosh" && commandName !== "ET") {
    return null;
  }

  const user = presetData.metaDefault("User", "");
  const authentication = presetData.metaDefault("Authentication", "");
  const fingerprint = presetData.metaDefault("Fingerprint", "");
  const credential = presetCredential(presetData, authentication);

  if (user.length <= 0 || authentication.length <= 0 || credential === null) {
    return null;
  }

  const config = {
    user,
    authentication,
    host,
    charset,
    tabColor,
    fingerprint,
    presetID: presetData.id(),
  };

  if (commandName === "Mosh") {
    config.moshServer = presetData.metaDefault("Mosh Server", "mosh-server");
  }

  if (commandName === "ET") {
    if (authentication !== "Private Key") {
      return null;
    }

    const etServerPort = presetData.metaDefault("ET Server Port", "2022");
    if (!validETServerPort(etServerPort)) {
      return null;
    }

    config.etServerPort = etServerPort;
    config.etCommand = "et";
  }

  return {
    config,
    session: {
      credential,
    },
    keptSessions: credential.length > 0 ? ["credential"] : [],
  };
}
