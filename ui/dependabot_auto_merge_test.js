// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterEach, describe, expect, test } from "vitest";

import { authorizeDependabotUpdate } from "../.github/scripts/dependabot-auto-merge.mjs";

const hashes = ["1", "2", "3"].map((character) => character.repeat(40));
const temporaryDirectories = [];

function eventFor(headRef) {
  return {
    action: "opened",
    repository: {
      default_branch: "main",
      fork: false,
      full_name: "Snuffy2/shellport",
    },
    pull_request: {
      base: { ref: "main", sha: hashes[1] },
      head: {
        ref: headRef,
        repo: { full_name: "Snuffy2/shellport" },
        sha: hashes[2],
      },
      user: { login: "dependabot[bot]" },
    },
  };
}

function dependabotCommit(sha = hashes[2]) {
  return {
    author: { login: "dependabot[bot]" },
    commit: { verification: { verified: true } },
    parents: [],
    sha,
  };
}

function trustedBase(path) {
  const directory = mkdtempSync(join(tmpdir(), "dependabot-authorizer-"));
  temporaryDirectories.push(directory);
  const file = join(directory, path);
  mkdirSync(join(file, ".."), { recursive: true });
  writeFileSync(file, "trusted\n");
  return directory;
}

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0))
    rmSync(directory, { force: true, recursive: true });
});

describe("Dependabot auto-merge authorization", () => {
  test("accepts a verified npm manifest and lockfile update", () => {
    expect(
      authorizeDependabotUpdate({
        actor: "dependabot[bot]",
        changedFiles: ["package.json", "package-lock.json"],
        commits: [dependabotCommit()],
        event: eventFor("dependabot/npm_and_yarn/example-1.0.0"),
      }),
    ).toBe("npm");
  });

  test("rejects npm updates that include generated output or omit the manifest", () => {
    const event = eventFor("dependabot/npm_and_yarn/example-1.0.0");
    for (const changedFiles of [
      ["package.json", "package-lock.json", "dist/index.js"],
      ["package-lock.json"],
    ]) {
      expect(() =>
        authorizeDependabotUpdate({
          actor: "dependabot[bot]",
          changedFiles,
          commits: [dependabotCommit()],
          event,
        }),
      ).toThrow();
    }
  });

  test("accepts an Actions update only when its target exists on the trusted base", () => {
    const event = eventFor("dependabot/github_actions/actions/checkout-7");
    expect(
      authorizeDependabotUpdate({
        actor: "dependabot[bot]",
        changedFiles: [".github/workflows/ci.yml"],
        commits: [dependabotCommit()],
        event,
        trustedBaseDirectory: trustedBase(".github/workflows/ci.yml"),
      }),
    ).toBe("github-actions");
  });

  test("rejects Actions files that are absent from the trusted base", () => {
    expect(() =>
      authorizeDependabotUpdate({
        actor: "dependabot[bot]",
        changedFiles: ["action.yml"],
        commits: [dependabotCommit()],
        event: eventFor("dependabot/github_actions/example/action"),
        trustedBaseDirectory: trustedBase(".github/workflows/ci.yml"),
      }),
    ).toThrow();
  });
});
