// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import { describe, it } from "vitest";
import { Preset } from "./commands/presets.js";
import { buildPresetExecution } from "./home_preset_execution.js";

/**
 * Builds the merged preset shape used by the home view.
 *
 * @param {string} commandName Command display name.
 * @param {object} presetData Raw preset configuration.
 * @returns {{command: object, preset: Preset}} Merged preset entry.
 */
function mergedPreset(commandName, presetData) {
  return {
    command: {
      name() {
        return commandName;
      },
    },
    preset: new Preset(presetData),
  };
}

describe("preset execution helpers", () => {
  it("builds direct SSH execution for a complete credential preset", () => {
    const execution = buildPresetExecution(
      mergedPreset("SSH", {
        title: "Example SSH",
        type: "SSH",
        host: "example.com:22",
        tab_color: "#123",
        id: "preset-ssh",
        meta: {
          User: "alice",
          Authentication: "Password",
          Encoding: "utf-8",
          Password: "secret",
          Fingerprint: "SHA256:abc",
        },
      }),
    );

    assert.deepStrictEqual(execution, {
      config: {
        user: "alice",
        authentication: "Password",
        host: "example.com:22",
        charset: "utf-8",
        tabColor: "#123",
        fingerprint: "SHA256:abc",
        presetID: "preset-ssh",
      },
      session: {
        credential: "secret",
      },
      keptSessions: ["credential"],
    });
  });

  it("falls back to the wizard when SSH connection details are incomplete", () => {
    const execution = buildPresetExecution(
      mergedPreset("SSH", {
        title: "Example SSH",
        type: "SSH",
        host: "example.com:22",
        meta: {
          Authentication: "Password",
        },
      }),
    );

    assert.strictEqual(execution, null);
  });

  it("builds direct SSH execution for Atlantis-style private key presets", () => {
    const execution = buildPresetExecution(
      mergedPreset("SSH", {
        title: "Atlantis SSH",
        type: "SSH",
        host: "atlantis.home:22",
        meta: {
          User: "pi",
          Authentication: "Private Key",
          "Private Key": "PRIVATE KEY DATA",
        },
      }),
    );

    assert.deepStrictEqual(execution.config, {
      user: "pi",
      authentication: "Private Key",
      host: "atlantis.home:22",
      charset: "utf-8",
      tabColor: "",
      fingerprint: "",
      presetID: "",
    });
    assert.strictEqual(execution.session.credential, "PRIVATE KEY DATA");
    assert.deepStrictEqual(execution.keptSessions, ["credential"]);
  });

  it("builds direct SSH execution for Atlantis-style password presets without passwords", () => {
    const execution = buildPresetExecution(
      mergedPreset("SSH", {
        title: "Atlantis Password SSH",
        type: "SSH",
        host: "atlantis.home:22",
        meta: {
          User: "pi",
          Authentication: "Password",
        },
      }),
    );

    assert.deepStrictEqual(execution.config, {
      user: "pi",
      authentication: "Password",
      host: "atlantis.home:22",
      charset: "utf-8",
      tabColor: "",
      fingerprint: "",
      presetID: "",
    });
    assert.strictEqual(execution.session.credential, "");
    assert.deepStrictEqual(execution.keptSessions, []);
  });

  it("builds direct Mosh execution with the default mosh-server command", () => {
    const execution = buildPresetExecution(
      mergedPreset("Mosh", {
        title: "Example Mosh",
        type: "Mosh",
        host: "example.com:22",
        meta: {
          User: "alice",
          Authentication: "Private Key",
          "Private Key": "PRIVATE KEY DATA",
          Fingerprint: "SHA256:abc",
        },
      }),
    );

    assert.strictEqual(execution.config.moshServer, "mosh-server");
    assert.strictEqual(execution.session.credential, "PRIVATE KEY DATA");
    assert.deepStrictEqual(execution.keptSessions, ["credential"]);
  });

  it("builds direct ET execution with ET metadata and private-key credential", () => {
    const execution = buildPresetExecution(
      mergedPreset("ET", {
        title: "Example ET",
        type: "ET",
        host: "example.com:22",
        meta: {
          User: "alice",
          Authentication: "Private Key",
          "Private Key": "PRIVATE KEY DATA",
          Fingerprint: "SHA256:abc",
          "ET Server Port": "22022",
          "ET Command": "/usr/local/bin/et",
        },
      }),
    );

    assert.deepStrictEqual(execution.config, {
      user: "alice",
      authentication: "Private Key",
      host: "example.com:22",
      charset: "utf-8",
      tabColor: "",
      fingerprint: "SHA256:abc",
      presetID: "",
      etServerPort: "22022",
      etCommand: "/usr/local/bin/et",
    });
    assert.strictEqual(execution.session.credential, "PRIVATE KEY DATA");
    assert.deepStrictEqual(execution.keptSessions, ["credential"]);
  });

  it("does not direct-launch ET password presets", () => {
    const execution = buildPresetExecution(
      mergedPreset("ET", {
        title: "Example ET Password",
        type: "ET",
        host: "example.com:22",
        meta: {
          User: "alice",
          Authentication: "Password",
          Password: "secret",
        },
      }),
    );

    assert.strictEqual(execution, null);
  });

  it("builds direct Telnet execution for host presets", () => {
    const execution = buildPresetExecution(
      mergedPreset("Telnet", {
        title: "Example Telnet",
        type: "Telnet",
        host: "telnet.example.com:23",
        meta: {
          Encoding: "utf-8",
        },
      }),
    );

    assert.deepStrictEqual(execution, {
      config: {
        host: "telnet.example.com:23",
        charset: "utf-8",
        tabColor: "",
      },
      session: {},
      keptSessions: [],
    });
  });
});
