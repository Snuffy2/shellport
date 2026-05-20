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

describe("JetBrainsMono Nerd Font updater", function () {
  test("is wired into dependency maintenance", function () {
    const packageConfig = JSON.parse(readProjectFile("package.json"));
    const workflowSource = readProjectFile(
      ".github/workflows/renovate_regen.yml",
    );
    const updaterSource = readProjectFile(
      "scripts/update-jetbrains-nerd-font.mjs",
    );

    expect(packageConfig.scripts["update:fonts"]).toBe(
      "node scripts/update-jetbrains-nerd-font.mjs",
    );
    expect(workflowSource).toContain("npm run update:fonts");
    expect(workflowSource).toContain("ui/fonts/JetBrainsMonoNerdFont");
    expect(updaterSource).toContain(
      "https://api.github.com/repos/${nerdFontsRepo}/releases/latest",
    );
    expect(updaterSource).toContain("SHA-256.txt");
    expect(updaterSource).toContain("archiveSHA256");
    expect(updaterSource).toContain("JetBrainsMonoNerdFontMono-Regular.ttf");
    expect(updaterSource).toContain("JetBrainsMonoNerdFontMono-Bold.ttf");
  });
});
