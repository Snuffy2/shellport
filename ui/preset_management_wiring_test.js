// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, test } from "vitest";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

function readProjectFile(relativePath) {
  return readFileSync(path.join(repoRoot, relativePath), "utf8");
}

describe("preset management UI wiring", () => {
  test("known preset list has accessible icon edit button", () => {
    const source = readProjectFile("ui/widgets/connect_known.vue");
    const styles = readProjectFile("ui/widgets/connect_known.css");

    expect(source).toContain("canManagePresets");
    expect(source).toContain('aria-label="Edit preset"');
    expect(source).toContain('title="Edit preset"');
    expect(source).toContain('@click="editPreset(preset)"');
    expect(source).toContain('<span aria-hidden="true">&#9998;</span>');
    expect(source).toContain('"edit-preset"');
    expect(styles).toContain(".preset-edit-button > span");
    expect(styles).toContain("preset-row");
  });

  test("connect widget renders preset editor mode", () => {
    const source = readProjectFile("ui/widgets/connect.vue");

    expect(source).toContain("preset-editor");
    expect(source).toContain(':save-preset="presetSaveHandler"');
    expect(source).toContain(':delete-preset="presetDeleteHandler"');
  });

  test("preset editor uses color selector for tab color", () => {
    const source = readProjectFile("ui/widgets/preset_editor.vue");

    expect(source).toContain("Tab color");
    expect(source).toContain('type="color"');
  });

  test("home updates full preset list for save and delete", () => {
    const source = readProjectFile("ui/home.vue");

    expect(source).toContain("openPresetEditor(preset)");
    expect(source).toContain("savePresetFromEditor(payload)");
    expect(source).toContain("deletePresetFromEditor(payload)");
    expect(source).toContain("clearHiddenPasswordIDs");
  });

  test("home only passes save-as callback when preset management is available", () => {
    const source = readProjectFile("ui/home.vue");

    expect(source).toContain("const saveAsPreset = self.canManagePresets");
    expect(source).toContain(": null;");
    expect(source).toContain("saveAsPreset,");
  });

  test("connector action buttons have visible separation", () => {
    const source = readProjectFile("ui/widgets/connector.vue");
    const styles = readProjectFile("ui/widgets/connector.css");
    const presetEditorStyles = readProjectFile("ui/widgets/preset_editor.css");

    expect(source).toContain("connector-actions");
    expect(styles).toContain("#connector .connector-actions > button + button");
    expect(styles).toContain("margin-left: 8px");
    expect(presetEditorStyles).toContain("gap: 8px");
  });

  test("save as preset can open editor without required connection fields", () => {
    const connectorSource = readProjectFile("ui/widgets/connector.vue");

    expect(connectorSource).toContain("action.validate !== false");
    for (const commandPath of [
      "ui/commands/ssh.js",
      "ui/commands/mosh.js",
      "ui/commands/et.js",
      "ui/commands/telnet.js",
    ]) {
      const source = readProjectFile(commandPath);

      expect(source).toContain('text: "Save as preset"');
      expect(source).toContain("validate: false");
    }
  });
});
