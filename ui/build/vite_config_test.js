// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, test } from "vitest";
import viteConfig from "../../vite.config.js";
import {
  resolveDevAssetRoute,
  resolveSourceURL,
  resolveVersion,
  renderDevShellHtml,
  rewriteDevShellScriptPaths,
} from "../../vite.config.js";

const testDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(testDir, "../..");
const viteConfigPath = path.join(repoRoot, "vite.config.js");
const indexHtmlPath = path.join(repoRoot, "ui", "index.html");
const errorHtmlPath = path.join(repoRoot, "ui", "error.html");
const connectVuePath = path.join(repoRoot, "ui", "widgets", "connect.vue");
const homeVuePath = path.join(repoRoot, "ui", "home.vue");
const legacyHelperNames = [
  ["passthrough", "AssetToken"],
  ["replace", "PublicAssetPaths"],
  ["restore", "PublicAssetTokens"],
  ["normalize", "HtmlOutputsPlugin"],
  ["restore", "DevHtmlAssetsPlugin"],
].map((parts) => parts.join(""));
const legacyPolyfillNames = [
  ["stream", "-browserify"],
  ["virtual:", "shellport-node-globals"],
  ["browser", "NodePolyfillsPlugin"],
].map((parts) => parts.join(""));

function readSource(filePath) {
  return fs.readFileSync(filePath, "utf8");
}

describe("vite config cleanup guards", () => {
  test("vite.config.js drops legacy helper names", () => {
    const viteConfig = readSource(viteConfigPath);

    for (const helperName of legacyHelperNames) {
      expect(viteConfig).not.toContain(helperName);
    }
  });

  test("vite.config.js keeps the simplified uiRoot and publicDir wiring", () => {
    const viteConfig = readSource(viteConfigPath);

    expect(viteConfig).toContain("uiRoot");
    expect(viteConfig).toContain("root: uiRoot");
    expect(viteConfig).toMatch(/^\s*publicDir,\s*$/m);
  });

  test("vite.config.js removes the node polyfill asset plumbing", () => {
    const viteConfig = readSource(viteConfigPath);

    for (const polyfillName of legacyPolyfillNames) {
      expect(viteConfig).not.toContain(polyfillName);
    }
  });

  test("source URL defaults to the repository and rejects invalid values", () => {
    expect(resolveSourceURL({})).toBe("https://github.com/Snuffy2/shellport");
    expect(
      resolveSourceURL({
        SHELLPORT_SOURCE_URL:
          "https://github.com/Snuffy2/shellport/archive/abc123.tar.gz",
      }),
    ).toBe("https://github.com/Snuffy2/shellport/archive/abc123.tar.gz");
    expect(() => resolveSourceURL({ SHELLPORT_SOURCE_URL: "" })).toThrow(
      "non-empty",
    );
    expect(() =>
      resolveSourceURL({ SHELLPORT_SOURCE_URL: "git://example.test/repo" }),
    ).toThrow("https:");
    expect(() =>
      resolveSourceURL({
        SHELLPORT_SOURCE_URL: "https://token@example.test/repo.tar.gz",
      }),
    ).toThrow("credentials");
  });

  test("vite config exposes the validated source URL to the frontend", () => {
    const config = viteConfig({ command: "build", mode: "test" });

    expect(config.define.__SHELLPORT_SOURCE_URL__).toBe(
      JSON.stringify(resolveSourceURL()),
    );
  });

  test("vite config exposes the resolved version to the frontend", () => {
    const config = viteConfig({ command: "build", mode: "test" });

    expect(resolveVersion({ SHELLPORT_VERSION: "v1.2.3" })).toBe("v1.2.3");
    expect(config.define.__SHELLPORT_VERSION__).toBe(
      JSON.stringify(resolveVersion()),
    );
  });

  test("version resolver falls back to a source-tree version file", () => {
    const missingGit = () => {
      throw new Error("git unavailable");
    };
    const readVersionFile = (filePath) => {
      expect(filePath).toBe("/tmp/shellport-version");

      return "c3a41fa\n";
    };

    expect(
      resolveVersion(
        {},
        {
          execFileSync: missingGit,
          readFileSync: readVersionFile,
          versionFilePath: "/tmp/shellport-version",
        },
      ),
    ).toBe("c3a41fa");
  });

  test("version resolver uses dev when no version source is available", () => {
    const missingSource = () => {
      throw new Error("missing");
    };

    expect(
      resolveVersion(
        {},
        { execFileSync: missingSource, readFileSync: missingSource },
      ),
    ).toBe("dev");
  });

  test("home screen binds the source link to the configured frontend value", () => {
    const connectVue = readSource(connectVuePath);
    const homeVue = readSource(homeVuePath);

    expect(homeVue).toContain("Version {{ version }}");
    expect(homeVue).toContain(':href="sourceURL"');
    expect(homeVue).toContain('rel="noopener noreferrer"');
    expect(homeVue).toContain("sourceURL: __SHELLPORT_SOURCE_URL__");
    expect(homeVue).toContain("version: __SHELLPORT_VERSION__");
    expect(connectVue).not.toContain("connect-warning");
    expect(connectVue).not.toContain(
      'href="https://github.com/Snuffy2/shellport"',
    );
  });

  test("home screen renders an optional server title and secondary message", () => {
    const appJs = readSource(path.join(repoRoot, "ui", "app.js"));
    const homeVue = readSource(homeVuePath);

    expect(appJs).toContain(':server-title="serverTitle"');
    expect(appJs).toContain('@title-change="updatePageTitle"');
    expect(appJs).toContain("updatePageTitle(newPageTitle)");
    expect(appJs).toContain("this.serverTitle = authData.server_title");
    expect(homeVue).toContain("<h1>{{ homeTitle }}</h1>");
    expect(homeVue).toContain('<p v-html="homeMessage"></p>');
    expect(homeVue).toContain("homeTitle()");
    expect(homeVue).toContain('"title-change"');
    expect(homeVue).toContain('this.$emit("title-change", this.homeTitle)');
    expect(homeVue).toContain("homeMessage()");
    expect(homeVue).toContain(
      '"ShellPort - browser-based remote shell access over SSH, Telnet, ET, and Mosh"',
    );
  });

  test("ui/index.html uses BASE_URL asset links and local scripts", () => {
    const indexHtml = readSource(indexHtmlPath);

    expect(indexHtml).not.toContain('href="/shellport/assets/');
    expect(indexHtml).not.toContain('src="/ui/');
    expect(indexHtml).toContain("%BASE_URL%shellport.svg");
    expect(indexHtml).toContain("%BASE_URL%site.webmanifest");
    expect(indexHtml).toContain("%BASE_URL%DEPENDENCIES.md");
    expect(indexHtml).toContain("%BASE_URL%README.md");
    expect(indexHtml).toContain("%BASE_URL%LICENSE.md");
    expect(indexHtml).toContain('src="node-globals.js"');
    expect(indexHtml).toContain('src="app.js"');
  });

  test("ui/error.html uses local stylesheet paths", () => {
    const errorHtml = readSource(errorHtmlPath);

    expect(errorHtml).not.toContain('href="/ui/');
    expect(errorHtml).toContain('href="app.css"');
    expect(errorHtml).toContain('href="common.css"');
    expect(errorHtml).toContain('href="landing.css"');
  });

  test("development asset routes include root documentation links", () => {
    const readmeAsset = resolveDevAssetRoute("/shellport/assets/README.md");
    const licenseAsset = resolveDevAssetRoute("/shellport/assets/LICENSE.md");
    const manifestAsset = resolveDevAssetRoute(
      "/shellport/assets/site.webmanifest",
    );
    const faviconAsset = resolveDevAssetRoute("/favicon.ico");

    expect(readmeAsset).toEqual({
      filePath: path.join(repoRoot, "README.md"),
      contentType: "text/markdown; charset=utf-8",
    });
    expect(licenseAsset).toEqual({
      filePath: path.join(repoRoot, "LICENSE.md"),
      contentType: "text/markdown; charset=utf-8",
    });
    expect(manifestAsset).toEqual({
      filePath: path.join(repoRoot, "ui", "public", "site.webmanifest"),
      contentType: "application/manifest+json",
    });
    expect(faviconAsset).toEqual({
      filePath: path.join(repoRoot, "ui", "public", "favicon.ico"),
      contentType: "image/x-icon",
    });
    expect(resolveDevAssetRoute("/shellport/assets/missing.md")).toBeNull();
  });

  test("development shell script paths resolve from Vite module root", () => {
    const html = [
      '<script type="module" src="/shellport/assets/@vite/client"></script>',
      '<script type="module" src="node-globals.js"></script>',
      '<script type="module" src="app.js"></script>',
    ].join("");

    expect(rewriteDevShellScriptPaths(html)).toBe(
      [
        '<script type="module" src="/shellport/assets/@vite/client"></script>',
        '<script type="module" src="/node-globals.js"></script>',
        '<script type="module" src="/app.js"></script>',
      ].join(""),
    );
  });

  test("development shell emits fixed public asset URLs after Vite transforms it", async () => {
    const sourceHtml = [
      '<link rel="icon" type="image/svg+xml" href="%BASE_URL%shellport.svg" />',
      '<link rel="manifest" href="%BASE_URL%site.webmanifest" />',
      '<script type="module" src="node-globals.js"></script>',
      '<script type="module" src="app.js"></script>',
    ].join("");
    const server = {
      transformIndexHtml: async (_url, html) =>
        html.replaceAll("%BASE_URL%", "/shellport/assets/shellport/assets/"),
    };

    const html = await renderDevShellHtml(server, "", sourceHtml);

    expect(html).toContain('href="/shellport/assets/shellport.svg"');
    expect(html).toContain('href="/shellport/assets/site.webmanifest"');
    expect(html).toContain('src="/node-globals.js"');
    expect(html).toContain('src="/app.js"');
    expect(html).not.toContain("/shellport/assets/shellport/assets/");
  });
});
