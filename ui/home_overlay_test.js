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

describe("home overlays", () => {
  test("closing the final tab hides the tab-list overlay", () => {
    const source = readProjectFile("ui/home.vue");

    expect(source).toContain("this.tab.tabs.splice(index, 1);");
    expect(source).toContain("if (this.tab.tabs.length === 0)");
    expect(source).toContain("this.windows.tabs = false;");
  });

  test("tab-list overlay watches tab count, not array identity", () => {
    const source = readProjectFile("ui/widgets/tab_window.vue");

    expect(source).toContain('"tabs.length"');
    expect(source).toContain("handler(newLength)");
    expect(source).toContain("if (!newDisplay || this.tabs.length > 0)");
    expect(source).toContain('this.$emit("display", false);');
  });
});
