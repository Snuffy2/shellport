// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { execFileSync } from "node:child_process";

const numericIdentifier = "(?:0|[1-9][0-9]*)";
const prereleaseIdentifier = `(?:${numericIdentifier}|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)`;
const semverTagPattern = new RegExp(
  `^v?(?<version>${numericIdentifier}\\.${numericIdentifier}\\.${numericIdentifier}` +
    `(?:-${prereleaseIdentifier}(?:\\.${prereleaseIdentifier})*)?)$`,
  "u",
);
const versionLikeTagPattern = /^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-.].*)?$/u;
const dockerTagPattern = /^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$/u;
const commitPattern = /^[0-9a-f]{40}$/u;

function fail(message) {
  throw new Error(`Refusing Docker publication: ${message}`);
}

export function versionFromTag(tag) {
  const match = semverTagPattern.exec(tag);
  return match?.groups?.version ?? null;
}

export function resolveReleaseSource(event, git) {
  if (!commitPattern.test(event.eventSHA)) {
    fail(`event SHA ${event.eventSHA} is not a full lowercase commit SHA`);
  }
  const eventCommit = git.commit(event.eventSHA);
  if (event.releaseTarget !== event.defaultBranch) {
    fail(`release target ${event.releaseTarget} is not ${event.defaultBranch}`);
  }
  const version = versionFromTag(event.releaseTag);
  if (version === null) {
    fail(`release tag ${event.releaseTag} is not a supported semantic version`);
  }
  const tagCommit = git.tagCommit(event.releaseTag);
  if (tagCommit !== eventCommit) {
    fail(
      `release tag ${event.releaseTag} resolves to ${tagCommit}, not event commit ${eventCommit}`,
    );
  }
  if (!git.isAncestor(eventCommit, event.defaultBranch)) {
    fail(
      `event commit ${eventCommit} is not an ancestor of ${event.defaultBranch}`,
    );
  }
  const tagIdentity = git.tagIdentity(event.releaseTag);
  return {
    imageTag: version,
    immutableVersion: true,
    releaseRefOID: tagIdentity.oid,
    releaseRefType: tagIdentity.type,
    sourceSHA: eventCommit,
  };
}

export function resolveWorkflowSource(event, git) {
  if (!commitPattern.test(event.eventSHA)) {
    fail(`event SHA ${event.eventSHA} is not a full lowercase commit SHA`);
  }
  const defaultRef = `refs/heads/${event.defaultBranch}`;
  if (event.eventRef !== defaultRef) {
    fail(`event ref ${event.eventRef} is not ${defaultRef}`);
  }
  if (git.branchCommit(event.defaultBranch) !== event.eventSHA) {
    fail(
      `event commit ${event.eventSHA} is not the tip of ${event.defaultBranch}`,
    );
  }
  if (event.eventName === "push") {
    if (event.inputTag) fail("main push unexpectedly supplied an image tag");
    return {
      imageTag: "edge",
      immutableVersion: false,
      releaseRefOID: "",
      releaseRefType: "",
      sourceSHA: event.eventSHA,
    };
  }
  if (event.eventName !== "workflow_dispatch") {
    fail(`unsupported workflow event ${event.eventName}`);
  }
  const imageTag = event.inputTag?.trim() ?? "";
  if (imageTag !== event.inputTag || !dockerTagPattern.test(imageTag)) {
    fail("manual workflow dispatch supplied an invalid Docker image tag");
  }
  if (imageTag === "edge" || imageTag === "latest") {
    fail(`manual workflow dispatch may not publish ${imageTag}`);
  }
  if (versionLikeTagPattern.test(imageTag)) {
    fail("manual workflow dispatch may not publish version tags");
  }
  return {
    imageTag,
    sourceSHA: event.eventSHA,
    immutableVersion: false,
    releaseRefOID: "",
    releaseRefType: "",
  };
}

function runGit(args) {
  return execFileSync("git", args, { encoding: "utf8" }).trim();
}

function gitForRelease(defaultBranch, releaseTag) {
  runGit([
    "fetch",
    "--force",
    "--no-tags",
    "origin",
    `refs/heads/${defaultBranch}:refs/remotes/origin/${defaultBranch}`,
    `refs/tags/${releaseTag}:refs/tags/${releaseTag}`,
  ]);
  return {
    commit(value) {
      return runGit(["rev-parse", "--verify", `${value}^{commit}`]);
    },
    tagCommit(tag) {
      return runGit(["rev-parse", "--verify", `${tag}^{commit}`]);
    },
    tagIdentity(tag) {
      const oid = runGit(["rev-parse", "--verify", `refs/tags/${tag}`]);
      return { oid, type: runGit(["cat-file", "-t", oid]) };
    },
    branchCommit(branch) {
      return runGit(["rev-parse", "--verify", `origin/${branch}^{commit}`]);
    },
    isAncestor(commit, branch) {
      try {
        runGit(["merge-base", "--is-ancestor", commit, `origin/${branch}`]);
        return true;
      } catch {
        return false;
      }
    },
  };
}

function gitForBranch(defaultBranch) {
  runGit([
    "fetch",
    "--force",
    "--no-tags",
    "origin",
    `refs/heads/${defaultBranch}:refs/remotes/origin/${defaultBranch}`,
  ]);
  return {
    branchCommit(branch) {
      return runGit(["rev-parse", "--verify", `origin/${branch}^{commit}`]);
    },
  };
}

function writeOutput(result) {
  process.stdout.write(
    [
      `source_sha=${result.sourceSHA}`,
      `image_tag=${result.imageTag}`,
      `immutable_version=${result.immutableVersion}`,
      `release_ref_oid=${result.releaseRefOID}`,
      `release_ref_type=${result.releaseRefType}`,
    ].join("\n") + "\n",
  );
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  const event = {
    defaultBranch: process.env.DEFAULT_BRANCH,
    eventSHA: process.env.EVENT_SHA,
    eventRef: process.env.EVENT_REF,
    inputTag: process.env.INPUT_TAG,
    releaseTag: process.env.RELEASE_TAG,
    releaseTarget: process.env.RELEASE_TARGET,
  };
  if (process.env.EVENT_NAME === "release") {
    if (!event.defaultBranch || !event.releaseTag || !event.releaseTarget) {
      fail("release event is missing immutable provenance fields");
    }
    writeOutput(
      resolveReleaseSource(
        event,
        gitForRelease(event.defaultBranch, event.releaseTag),
      ),
    );
  } else {
    writeOutput(
      resolveWorkflowSource(
        { ...event, eventName: process.env.EVENT_NAME },
        gitForBranch(event.defaultBranch),
      ),
    );
  }
}
