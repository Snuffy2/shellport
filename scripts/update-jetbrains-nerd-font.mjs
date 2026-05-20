// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { pipeline } from "node:stream/promises";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const nerdFontsRepo = "ryanoasis/nerd-fonts";
const fontArchiveName = "JetBrainsMono.tar.xz";
const latestReleaseURL = `https://api.github.com/repos/${nerdFontsRepo}/releases/latest`;
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
const textFontFiles = new Set(["OFL.txt", "README.md"]);

/**
 * Fetches a URL and returns its text body.
 *
 * @param {string} url Source URL.
 * @returns {Promise<string>} Response text.
 */
async function fetchText(url) {
  const response = await fetch(url, {
    headers: {
      Accept: "application/vnd.github+json, text/plain;q=0.9, */*;q=0.8",
      "User-Agent": "ShellPort font updater",
    },
  });
  if (!response.ok) {
    throw new Error(`failed to fetch ${url}: ${response.status}`);
  }

  return await response.text();
}

/**
 * Resolves the latest Nerd Fonts release tag.
 *
 * @returns {Promise<string>} Latest release tag, e.g. v3.4.0.
 */
async function resolveLatestReleaseTag() {
  const release = JSON.parse(await fetchText(latestReleaseURL));
  if (!release.tag_name) {
    throw new Error("latest Nerd Fonts release did not include tag_name");
  }

  return release.tag_name;
}

/**
 * Builds a GitHub release download URL.
 *
 * @param {string} releaseTag Nerd Fonts release tag.
 * @param {string} assetName Release asset name.
 * @returns {string} Download URL.
 */
function buildReleaseAssetURL(releaseTag, assetName) {
  return `https://github.com/${nerdFontsRepo}/releases/download/${releaseTag}/${assetName}`;
}

/**
 * Finds the expected SHA-256 checksum for the font archive in the release checksum file.
 *
 * @param {string} releaseTag Nerd Fonts release tag.
 * @returns {Promise<string>} Expected SHA-256 checksum.
 */
async function fetchExpectedArchiveSHA256(releaseTag) {
  const checksumText = await fetchText(
    buildReleaseAssetURL(releaseTag, "SHA-256.txt"),
  );
  const checksumLine = checksumText
    .split("\n")
    .find((line) => line.trim().endsWith(`  ${fontArchiveName}`));

  if (!checksumLine) {
    throw new Error(
      `SHA-256.txt for ${releaseTag} did not include ${fontArchiveName}`,
    );
  }

  return checksumLine.trim().split(/\s+/u)[0];
}

/**
 * Downloads a URL to a file.
 *
 * @param {string} url Source URL.
 * @param {string} targetPath File path to write.
 * @returns {Promise<void>}
 */
async function downloadFile(url, targetPath) {
  const response = await fetch(url, {
    headers: {
      "User-Agent": "ShellPort font updater",
    },
  });
  if (!response.ok) {
    throw new Error(`failed to download ${url}: ${response.status}`);
  }

  await pipeline(response.body, fs.createWriteStream(targetPath));
}

/**
 * Returns the SHA-256 checksum for a file.
 *
 * @param {string} filePath File path to hash.
 * @returns {string} Hex-encoded SHA-256.
 */
function fileSHA256(filePath) {
  const hash = createHash("sha256");
  hash.update(fs.readFileSync(filePath));

  return hash.digest("hex");
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
 * @param {string} releaseTag Nerd Fonts release tag.
 * @param {string} archiveSHA256 Verified source archive checksum.
 * @returns {void}
 */
function updateFontDirectory(extractDir, releaseTag, archiveSHA256) {
  fs.mkdirSync(fontTargetDir, { recursive: true });

  for (const fontFile of fontFiles) {
    const sourcePath = path.join(extractDir, fontFile);
    const targetPath = path.join(fontTargetDir, fontFile);

    fs.copyFileSync(sourcePath, targetPath);
    if (textFontFiles.has(fontFile)) {
      const content = fs.readFileSync(targetPath, "utf8");
      fs.writeFileSync(
        targetPath,
        content.replace(/[ \t]+$/gm, "").replace(/\n*$/u, "\n"),
      );
    }
    fs.chmodSync(targetPath, 0o644);
  }

  fs.writeFileSync(
    path.join(fontTargetDir, "manifest.json"),
    JSON.stringify(
      {
        source: nerdFontsRepo,
        releaseTag,
        archive: fontArchiveName,
        archiveSHA256,
      },
      null,
      2,
    ) + "\n",
  );
}

const tempDir = fs.mkdtempSync(
  path.join(os.tmpdir(), "shellport-jetbrains-nerd-font-"),
);

try {
  const releaseTag = await resolveLatestReleaseTag();
  const expectedArchiveSHA256 = await fetchExpectedArchiveSHA256(releaseTag);
  const archivePath = path.join(tempDir, fontArchiveName);

  await downloadFile(buildReleaseAssetURL(releaseTag, fontArchiveName), archivePath);
  const archiveSHA256 = fileSHA256(archivePath);
  if (archiveSHA256 !== expectedArchiveSHA256) {
    throw new Error(
      `${fontArchiveName} SHA-256 mismatch: expected ${expectedArchiveSHA256}, got ${archiveSHA256}`,
    );
  }
  extractFontFiles(archivePath, tempDir);
  updateFontDirectory(tempDir, releaseTag, archiveSHA256);
} finally {
  fs.rmSync(tempDir, { recursive: true, force: true });
}
