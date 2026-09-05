// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { execFileSync } from "node:child_process";

const semverTagPattern =
  /^v?(?<version>[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)$/u;
const commitPattern = /^[0-9a-f]{40}$/u;

function fail(message) {
  throw new Error(`Refusing Docker publication: ${message}`);
}

export function versionFromTag(tag) {
  const match = semverTagPattern.exec(tag);
  return match?.groups?.version ?? null;
}

export function resolveReleaseSource(event, git) {
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
  return { imageTag: version, sourceSHA: eventCommit, immutableVersion: true };
}

export function resolveWorkflowSource(event) {
  if (!commitPattern.test(event.eventSHA)) {
    fail(`event SHA ${event.eventSHA} is not a full lowercase commit SHA`);
  }
  const imageTag = event.inputTag || "edge";
  if (
    event.eventName === "workflow_dispatch" &&
    (imageTag === "edge" || !event.inputTag?.trim())
  ) {
    fail("manual workflow dispatch may not publish edge");
  }
  return {
    imageTag,
    sourceSHA: event.eventSHA,
    immutableVersion: versionFromTag(imageTag) !== null,
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

function writeOutput(result) {
  process.stdout.write(
    [
      `source_sha=${result.sourceSHA}`,
      `image_tag=${result.imageTag}`,
      `immutable_version=${result.immutableVersion}`,
    ].join("\n") + "\n",
  );
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  const event = {
    defaultBranch: process.env.DEFAULT_BRANCH,
    eventSHA: process.env.EVENT_SHA,
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
      resolveWorkflowSource({ ...event, eventName: process.env.EVENT_NAME }),
    );
  }
}
