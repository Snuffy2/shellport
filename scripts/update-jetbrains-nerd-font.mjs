// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { pipeline } from "node:stream/promises";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const fontArchiveURL =
  process.env.SHELLPORT_JETBRAINS_NERD_FONT_URL ||
  "https://github.com/ryanoasis/nerd-fonts/releases/latest/download/JetBrainsMono.tar.xz";
const fontTargetDir = path.join(
  repoRoot,
  "ui",
  "fonts",
  "JetBrainsMonoNerdFont",
);
const fontFiles = [
  "JetBrainsMonoNerdFontMono-Regular.ttf",
  "JetBrainsMonoNerdFontMono-Bold.ttf",
  "OFL.txt",
  "README.md",
];

/**
 * Downloads a URL to a file.
 *
 * @param {string} url Source URL.
 * @param {string} targetPath File path to write.
 * @returns {Promise<void>}
 */
async function downloadFile(url, targetPath) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`failed to download ${url}: ${response.status}`);
  }

  await pipeline(response.body, fs.createWriteStream(targetPath));
}

/**
 * Runs tar to extract the selected font files from the Nerd Fonts archive.
 *
 * @param {string} archivePath Downloaded tar.xz archive path.
 * @param {string} extractDir Temporary extraction directory.
 * @returns {void}
 */
function extractFontFiles(archivePath, extractDir) {
  const result = spawnSync(
    "tar",
    ["-xJf", archivePath, "-C", extractDir, ...fontFiles],
    {
      stdio: "inherit",
    },
  );

  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    throw new Error(`tar exited with status ${result.status}`);
  }
}

/**
 * Copies the selected extracted files into the checked-in font directory.
 *
 * @param {string} extractDir Temporary extraction directory.
 * @returns {void}
 */
function updateFontDirectory(extractDir) {
  fs.mkdirSync(fontTargetDir, { recursive: true });

  for (const fontFile of fontFiles) {
    const sourcePath = path.join(extractDir, fontFile);
    const targetPath = path.join(fontTargetDir, fontFile);

    fs.copyFileSync(sourcePath, targetPath);
    fs.chmodSync(targetPath, 0o644);
  }
}

const tempDir = fs.mkdtempSync(
  path.join(os.tmpdir(), "shellport-jetbrains-nerd-font-"),
);

try {
  const archivePath = path.join(tempDir, "JetBrainsMono.tar.xz");

  await downloadFile(fontArchiveURL, archivePath);
  extractFontFiles(archivePath, tempDir);
  updateFontDirectory(tempDir);
} finally {
  fs.rmSync(tempDir, { recursive: true, force: true });
}
