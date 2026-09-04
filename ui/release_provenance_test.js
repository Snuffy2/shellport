// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";

import { describe, expect, test } from "vitest";

import {
  resolveReleaseSource,
  resolveWorkflowSource,
} from "../.github/scripts/release-provenance.mjs";
import {
  resolveImmutableTag,
  tagsToCopy,
} from "../.github/scripts/release-registry-guard.mjs";

const eventSHA = "a".repeat(40);
const indexDigest = `sha256:${"b".repeat(64)}`;
const otherDigest = `sha256:${"c".repeat(64)}`;
const releaseWorkflow = readFileSync(
  new URL("../.github/workflows/release.yml", import.meta.url),
  "utf8",
);

function releaseEvent(overrides = {}) {
  return {
    defaultBranch: "main",
    eventSHA,
    releaseTag: "v1.2.3",
    releaseTarget: "main",
    ...overrides,
  };
}

function repository(overrides = {}) {
  return {
    commit: (value) => value,
    tagCommit: () => eventSHA,
    isAncestor: () => true,
    ...overrides,
  };
}

describe("release provenance", function () {
  test("uses the event commit only when the published tag still names it", function () {
    expect(resolveReleaseSource(releaseEvent(), repository())).toEqual({
      sourceSHA: eventSHA,
      imageTag: "1.2.3",
      immutableVersion: true,
    });
  });
  test.each([
    [
      "wrong target",
      releaseEvent({ releaseTarget: "release" }),
      repository(),
      "not main",
    ],
    [
      "missing tag",
      releaseEvent({ releaseTag: "candidate" }),
      repository(),
      "not a supported",
    ],
    [
      "non-ancestor",
      releaseEvent(),
      repository({ isAncestor: () => false }),
      "not an ancestor",
    ],
    [
      "moved tag",
      releaseEvent(),
      repository({ tagCommit: () => "b".repeat(40) }),
      "not event commit",
    ],
  ])("rejects %s", (_name, event, git, message) => {
    expect(() => resolveReleaseSource(event, git)).toThrow(message);
  });
  test("guards manual semantic versions but preserves the edge path", function () {
    expect(
      resolveWorkflowSource({ eventSHA, inputTag: "1.2.3" }),
    ).toMatchObject({ immutableVersion: true, imageTag: "1.2.3" });
    expect(
      resolveWorkflowSource({ eventSHA, inputTag: "nightly" }),
    ).toMatchObject({ immutableVersion: false, imageTag: "nightly" });
  });
});

describe("registry immutability guard", function () {
  test("retries a failed latest write from the same verified version archive", function () {
    const immutableTag = "ghcr.io/snuffy2/shellport:1.2.3";
    expect(
      resolveImmutableTag({
        expectedDigest: indexDigest,
        publishedDigest: indexDigest,
      }),
    ).toBe("matching");
    expect(
      tagsToCopy({
        tags: [immutableTag, "ghcr.io/snuffy2/shellport:latest"],
        immutableTag,
        immutableState: "matching",
      }),
    ).toEqual(["ghcr.io/snuffy2/shellport:latest"]);
  });
  test("publishes a previously absent immutable version", function () {
    expect(
      resolveImmutableTag({ expectedDigest: indexDigest, publishedDigest: "" }),
    ).toBe("absent");
  });
  test("rejects a full workflow rerun that rebuilds a different archive", function () {
    expect(() =>
      resolveImmutableTag({
        expectedDigest: indexDigest,
        publishedDigest: otherDigest,
      }),
    ).toThrow("not verified archive");
  });
});

describe("release publisher serialization", function () {
  test("shares one non-cancelling group for release and manual publishers", function () {
    const group = releaseWorkflow.match(/^  group: (?<value>.+)$/mu)?.groups?.value;
    const cancellation = releaseWorkflow.match(
      /^  cancel-in-progress: (?<value>.+)$/mu,
    )?.groups?.value;

    expect(group).toContain("release-publishers");
    expect(group).not.toContain("github.run_id");
    expect(group).toContain("main-edge");
    expect(cancellation).toContain("github.event_name == 'push'");
    expect(releaseWorkflow).toContain('"$IMMUTABLE_STATE" == "matching"');
    expect(releaseWorkflow).toContain('"$tag" == "$IMMUTABLE_TAG"');
    expect(releaseWorkflow).toContain("copy --all");
  });
});
