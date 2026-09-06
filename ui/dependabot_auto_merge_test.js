// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";

import { afterEach, describe, expect, test } from "vitest";

import { authorizeDependabotUpdate } from "../.github/scripts/dependabot-auto-merge.mjs";

const [dependabotSha, currentBaseSha, headSha] = ["1", "2", "3"].map(
  (character) => character.repeat(40),
);
const temporaryDirectories = [];

function eventFor(headRef, action = "opened") {
  return {
    action,
    repository: {
      default_branch: "main",
      fork: false,
      full_name: "Snuffy2/shellport",
    },
    pull_request: {
      base: { ref: "main", sha: currentBaseSha },
      head: {
        ref: headRef,
        repo: { full_name: "Snuffy2/shellport" },
        sha: headSha,
      },
      user: { login: "dependabot[bot]" },
    },
  };
}

function dependabotCommit(sha = headSha) {
  return {
    author: { login: "dependabot[bot]" },
    commit: { verification: { verified: true } },
    parents: [],
    sha,
  };
}

function updateCommit(sha, previous, base) {
  return {
    author: { login: "maintainer" },
    commit: { verification: { verified: true } },
    committer: { login: "web-flow" },
    parents: [{ sha: previous }, { sha: base }],
    sha,
  };
}

function trustedBase(...paths) {
  const directory = mkdtempSync(join(tmpdir(), "dependabot-authorizer-"));
  temporaryDirectories.push(directory);
  for (const path of paths) {
    const file = join(directory, path);
    mkdirSync(dirname(file), { recursive: true });
    writeFileSync(file, "trusted\n");
  }
  return directory;
}

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0))
    rmSync(directory, { force: true, recursive: true });
});

describe("Dependabot auto-merge authorization", () => {
  test("accepts a verified npm manifest and lockfile update from an npm base", () => {
    expect(
      authorizeDependabotUpdate({
        actor: "dependabot[bot]",
        changedFiles: ["package.json", "package-lock.json"],
        commits: [dependabotCommit()],
        event: eventFor("dependabot/npm_and_yarn/example-1.0.0"),
        trustedBaseDirectory: trustedBase("package.json", "package-lock.json"),
      }),
    ).toBe("npm");
  });

  test("accepts a verified npm lockfile-only update from an npm base", () => {
    expect(
      authorizeDependabotUpdate({
        actor: "dependabot[bot]",
        changedFiles: ["package-lock.json"],
        commits: [dependabotCommit()],
        event: eventFor("dependabot/npm_and_yarn/example-1.0.0"),
        trustedBaseDirectory: trustedBase("package.json", "package-lock.json"),
      }),
    ).toBe("npm");
  });

  test("rejects npm updates that omit the lockfile or include generated output", () => {
    const event = eventFor("dependabot/npm_and_yarn/example-1.0.0");
    for (const changedFiles of [
      ["package.json", "package-lock.json", "dist/index.js"],
      ["package.json"],
    ]) {
      expect(() =>
        authorizeDependabotUpdate({
          actor: "dependabot[bot]",
          changedFiles,
          commits: [dependabotCommit()],
          event,
          trustedBaseDirectory: trustedBase(
            "package.json",
            "package-lock.json",
          ),
        }),
      ).toThrow();
    }
  });

  test("rejects an update whose ecosystem is absent from the trusted base", () => {
    expect(() =>
      authorizeDependabotUpdate({
        actor: "dependabot[bot]",
        changedFiles: ["uv.lock"],
        commits: [dependabotCommit()],
        event: eventFor("dependabot/uv/example-1.0.0"),
        trustedBaseDirectory: trustedBase("package.json", "package-lock.json"),
      }),
    ).toThrow();
  });

  test("accepts existing trusted workflow and action manifests", () => {
    const event = eventFor("dependabot/github_actions/actions/checkout-7");
    const trustedBaseDirectory = trustedBase(
      ".github/workflows/ci.yml",
      "actions/release/action.yaml",
    );
    for (const changedFiles of [
      [".github/workflows/ci.yml"],
      ["actions/release/action.yaml"],
    ])
      expect(
        authorizeDependabotUpdate({
          actor: "dependabot[bot]",
          changedFiles,
          commits: [dependabotCommit()],
          event,
          trustedBaseDirectory,
        }),
      ).toBe("github-actions");
  });

  test("rejects absent and path-escaping Actions files", () => {
    const event = eventFor("dependabot/github_actions/example/action");
    for (const changedFiles of [["action.yml"], ["../action.yml"]])
      expect(() =>
        authorizeDependabotUpdate({
          actor: "dependabot[bot]",
          changedFiles,
          commits: [dependabotCommit()],
          event,
          trustedBaseDirectory: trustedBase(".github/workflows/ci.yml"),
        }),
      ).toThrow();
  });

  test("accepts a verified web-flow Update branch chain", () => {
    const firstUpdateSha = "4".repeat(40);
    expect(
      authorizeDependabotUpdate({
        actor: "maintainer",
        changedFiles: ["package-lock.json"],
        commits: [
          dependabotCommit(dependabotSha),
          updateCommit(firstUpdateSha, dependabotSha, "5".repeat(40)),
          updateCommit(headSha, firstUpdateSha, currentBaseSha),
        ],
        event: eventFor("dependabot/npm_and_yarn/example-1.0.0", "synchronize"),
        trustedBaseDirectory: trustedBase("package.json", "package-lock.json"),
      }),
    ).toBe("npm");
  });

  test("rejects a non-web-flow Update branch chain", () => {
    const commit = updateCommit(headSha, dependabotSha, currentBaseSha);
    commit.committer.login = "maintainer";
    expect(() =>
      authorizeDependabotUpdate({
        actor: "maintainer",
        changedFiles: ["package-lock.json"],
        commits: [dependabotCommit(dependabotSha), commit],
        event: eventFor("dependabot/npm_and_yarn/example-1.0.0", "synchronize"),
        trustedBaseDirectory: trustedBase("package.json", "package-lock.json"),
      }),
    ).toThrow();
  });
});
