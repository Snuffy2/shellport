// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";

import { afterEach, describe, expect, test } from "vitest";

import { authorizeDependabotUpdate } from "../.github/scripts/dependabot-auto-merge.mjs";

const [dependabotSha, firstBaseSha, firstUpdateSha, currentBaseSha, headSha] = [
  "1",
  "2",
  "3",
  "4",
  "5",
].map((character) => character.repeat(40));
const temporaryDirectories = [];

function eventFor(headRef, action = "reopened") {
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

function dependabotCommit(
  sha = headSha,
  verified = true,
  committer = "web-flow",
) {
  return {
    author: { login: "dependabot[bot]" },
    commit: { verification: { verified } },
    committer: { login: committer },
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

function ancestryProof(parentSha, status = "ahead") {
  return {
    ahead_by: status === "identical" ? 0 : 1,
    base_commit: parentSha,
    base_sha: currentBaseSha,
    behind_by: 0,
    head_commit: currentBaseSha,
    merge_base_commit: parentSha,
    parent_sha: parentSha,
    status,
  };
}

function updateChain() {
  return [
    dependabotCommit(dependabotSha),
    updateCommit(firstUpdateSha, dependabotSha, firstBaseSha),
    updateCommit(headSha, firstUpdateSha, currentBaseSha),
  ];
}

function updateChainProofs() {
  return [
    ancestryProof(firstBaseSha),
    ancestryProof(currentBaseSha, "identical"),
  ];
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

function authorize({
  actor = "dependabot[bot]",
  ancestryProofs = [],
  changedFiles = ["package-lock.json"],
  commits = [dependabotCommit()],
  event = eventFor("dependabot/npm_and_yarn/example-1.0.0"),
  trustedBaseDirectory = trustedBase("package.json", "package-lock.json"),
} = {}) {
  return authorizeDependabotUpdate({
    actor,
    ancestryProofs,
    changedFiles,
    commits,
    event,
    trustedBaseDirectory,
  });
}

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0))
    rmSync(directory, { force: true, recursive: true });
});

describe("Dependabot auto-merge authorization", () => {
  test("authorizes reopened direct updates from verified exact history", () => {
    expect(authorize()).toBe("npm");
    expect(
      authorize({ changedFiles: ["package.json", "package-lock.json"] }),
    ).toBe("npm");
  });

  test("authorizes reopened GitHub Update branch chains", () => {
    expect(
      authorize({
        ancestryProofs: updateChainProofs(),
        commits: updateChain(),
      }),
    ).toBe("npm");
  });

  test("rejects direct updates and chain roots without GitHub web-flow identity", () => {
    for (const committer of [undefined, "maintainer"])
      expect(() => {
        const direct = dependabotCommit(headSha, true, committer);
        if (committer === undefined) delete direct.committer;
        authorize({ commits: [direct] });
      }).toThrow();

    for (const committer of [undefined, "maintainer"])
      expect(() => {
        const chain = updateChain();
        if (committer === undefined) delete chain[0].committer;
        else chain[0].committer.login = committer;
        authorize({ ancestryProofs: updateChainProofs(), commits: chain });
      }).toThrow();
  });

  test("rejects an Update branch merge not committed by GitHub web flow", () => {
    const chain = updateChain();
    chain[1].committer.login = "maintainer";
    expect(() =>
      authorize({ ancestryProofs: updateChainProofs(), commits: chain }),
    ).toThrow();
  });

  test("does not use the triggering actor or action as authorization inputs", () => {
    for (const [actor, action] of [
      ["dependabot[bot]", "opened"],
      ["maintainer", "synchronize"],
      ["any-user", "reopened"],
    ])
      expect(
        authorize({
          actor,
          event: eventFor("dependabot/npm_and_yarn/example-1.0.0", action),
        }),
      ).toBe("npm");
  });

  test("accepts an older base ancestor for an intermediate GitHub merge", () => {
    expect(
      authorize({
        ancestryProofs: updateChainProofs(),
        commits: updateChain(),
      }),
    ).toBe("npm");
  });

  test("rejects absent, arbitrary, diverged, and mismatched ancestry evidence", () => {
    const invalidProofSets = [
      [],
      [{}, ancestryProof(currentBaseSha, "identical")],
      [ancestryProof(firstBaseSha), ancestryProof("9".repeat(40))],
      [
        ancestryProof(firstBaseSha, "diverged"),
        ancestryProof(currentBaseSha, "identical"),
      ],
      [
        { ...ancestryProof(firstBaseSha), head_commit: "8".repeat(40) },
        ancestryProof(currentBaseSha, "identical"),
      ],
    ];
    for (const ancestryProofs of invalidProofSets)
      expect(() =>
        authorize({ ancestryProofs, commits: updateChain() }),
      ).toThrow();
  });

  test("requires the latest merge parent and latest commit to match event state", () => {
    const staleParentChain = updateChain();
    staleParentChain[2] = updateCommit(headSha, firstUpdateSha, firstBaseSha);
    for (const commits of [staleParentChain, [dependabotCommit(dependabotSha)]])
      expect(() =>
        authorize({ ancestryProofs: updateChainProofs(), commits }),
      ).toThrow();
  });

  test("rejects unverified history, invalid provenance, and scope changes", () => {
    const untrustedEvent = eventFor("dependabot/npm_and_yarn/example-1.0.0");
    untrustedEvent.pull_request.head.repo.full_name = "fork/repository";
    for (const input of [
      { commits: [dependabotCommit(headSha, false)] },
      { event: untrustedEvent },
      { changedFiles: ["README.md"] },
    ])
      expect(() => authorize(input)).toThrow();
  });

  test("keeps each ecosystem scope rooted in trusted base contents", () => {
    expect(
      authorize({
        changedFiles: ["uv.lock"],
        event: eventFor("dependabot/uv/example-1.0.0"),
        trustedBaseDirectory: trustedBase("uv.lock"),
      }),
    ).toBe("uv");
    expect(
      authorize({
        changedFiles: [".github/workflows/ci.yml"],
        event: eventFor("dependabot/github_actions/actions/checkout-7"),
        trustedBaseDirectory: trustedBase(".github/workflows/ci.yml"),
      }),
    ).toBe("github-actions");
    expect(() =>
      authorize({
        changedFiles: ["action.yml"],
        event: eventFor("dependabot/github_actions/example/action"),
        trustedBaseDirectory: trustedBase(".github/workflows/ci.yml"),
      }),
    ).toThrow();
  });
});
