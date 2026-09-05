// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";

import { describe, expect, test } from "vitest";

import {
  resolveReleaseSource,
  resolveWorkflowSource,
} from "../.github/scripts/release-provenance.mjs";
import {
  assertPlatformIndex,
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
    branchCommit: () => eventSHA,
    commit: (value) => value,
    tagCommit: () => eventSHA,
    tagIdentity: () => ({ oid: eventSHA, type: "commit" }),
    isAncestor: () => true,
    ...overrides,
  };
}

function workflowEvent(overrides = {}) {
  return {
    defaultBranch: "main",
    eventName: "workflow_dispatch",
    eventRef: "refs/heads/main",
    eventSHA,
    inputTag: "nightly",
    ...overrides,
  };
}

describe("release provenance", function () {
  test("uses the event commit only when the published tag still names it", function () {
    expect(resolveReleaseSource(releaseEvent(), repository())).toEqual({
      sourceSHA: eventSHA,
      imageTag: "1.2.3",
      immutableVersion: true,
      releaseRefOID: eventSHA,
      releaseRefType: "commit",
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
  test("records the direct tag object as well as its peeled commit", function () {
    expect(
      resolveReleaseSource(
        releaseEvent(),
        repository({
          tagIdentity: () => ({ oid: "b".repeat(40), type: "tag" }),
        }),
      ),
    ).toMatchObject({
      releaseRefOID: "b".repeat(40),
      releaseRefType: "tag",
    });
  });
  test.each([
    "v01.2.3",
    "v1.02.3",
    "v1.2.03",
    "v1.2.3-",
    "v1.2.3-alpha..1",
    "v1.2.3-beta.01",
  ])("rejects invalid semantic release tag %s", (releaseTag) => {
    expect(() =>
      resolveReleaseSource(releaseEvent({ releaseTag }), repository()),
    ).toThrow("not a supported semantic version");
  });
  test("keeps the implicit edge tag for main pushes", function () {
    expect(
      resolveWorkflowSource(
        workflowEvent({ eventName: "push", inputTag: "" }),
        repository(),
      ),
    ).toEqual({
      imageTag: "edge",
      immutableVersion: false,
      releaseRefOID: "",
      releaseRefType: "",
      sourceSHA: eventSHA,
    });
  });
  test.each([
    ["edge", "may not publish edge"],
    ["latest", "may not publish latest"],
    ["1.2.3", "may not publish version tags"],
    ["v1.2.3", "may not publish version tags"],
    ["1.2.3-beta.01", "may not publish version tags"],
    ["", "invalid Docker image tag"],
    [" nightly ", "invalid Docker image tag"],
    ["bad/tag", "invalid Docker image tag"],
  ])("rejects reserved or invalid manual tag %j", (inputTag, message) => {
    expect(() =>
      resolveWorkflowSource(workflowEvent({ inputTag }), repository()),
    ).toThrow(message);
  });
  test.each([
    [
      "non-default ref",
      workflowEvent({ eventRef: "refs/heads/release" }),
      repository(),
      "is not refs/heads/main",
    ],
    [
      "stale default-branch commit",
      workflowEvent(),
      repository({ branchCommit: () => "b".repeat(40) }),
      "is not the tip of main",
    ],
    [
      "unsupported event",
      workflowEvent({ eventName: "pull_request" }),
      repository(),
      "unsupported workflow event",
    ],
  ])("rejects manual publication from %s", (_name, event, git, message) => {
    expect(() => resolveWorkflowSource(event, git)).toThrow(message);
  });
  test("accepts a custom tag only from the current default-branch tip", function () {
    expect(resolveWorkflowSource(workflowEvent(), repository())).toMatchObject({
      immutableVersion: false,
      imageTag: "nightly",
    });
  });
});

describe("registry immutability guard", function () {
  test("retries the current release's failed latest write from its verified archive", function () {
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
  test("makes an older matching immutable release a no-op without latest", function () {
    const immutableTag = "ghcr.io/snuffy2/shellport:1.2.3";
    expect(
      tagsToCopy({
        tags: [immutableTag],
        immutableTag,
        immutableState: "matching",
      }),
    ).toEqual([]);
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

describe("OCI archive policy", function () {
  const descriptor = (os, architecture, digestCharacter) => ({
    digest: `sha256:${digestCharacter.repeat(64)}`,
    platform: { architecture, os },
  });
  const attestation = (digestCharacter, subjectCharacter) => ({
    annotations: {
      "vnd.docker.reference.digest": `sha256:${subjectCharacter.repeat(64)}`,
      "vnd.docker.reference.type": "attestation-manifest",
    },
    digest: `sha256:${digestCharacter.repeat(64)}`,
    platform: { architecture: "unknown", os: "unknown" },
  });

  test("requires one amd64 and one arm64 Linux image", function () {
    expect(() =>
      assertPlatformIndex({
        manifests: [
          descriptor("linux", "amd64", "a"),
          descriptor("linux", "arm64", "b"),
          attestation("c", "a"),
          attestation("d", "b"),
        ],
      }),
    ).not.toThrow();
  });
  test.each([
    [descriptor("linux", "amd64", "a"), descriptor("linux", "amd64", "b")],
    [descriptor("linux", "amd64", "a")],
    [descriptor("linux", "amd64", "a"), descriptor("linux", "s390x", "b")],
  ])("rejects the invalid platform set %#", (...manifests) => {
    expect(() => assertPlatformIndex({ manifests })).toThrow(
      "not linux/amd64 and linux/arm64",
    );
  });
  test.each([
    [
      descriptor("linux", "amd64", "a"),
      descriptor("linux", "arm64", "b"),
      { digest: `sha256:${"c".repeat(64)}` },
    ],
    [
      descriptor("linux", "amd64", "a"),
      descriptor("linux", "arm64", "b"),
      attestation("c", "e"),
    ],
  ])("rejects an unrelated extra descriptor %#", (...manifests) => {
    expect(() => assertPlatformIndex({ manifests })).toThrow(
      /invalid manifest descriptor|attestations do not match/u,
    );
  });
});

describe("release publisher serialization", function () {
  test("shares one non-cancelling group for release and manual publishers", function () {
    const group = releaseWorkflow.match(/^ {2}group: (?<value>.+)$/mu)?.groups
      ?.value;
    const cancellation = releaseWorkflow.match(
      /^ {2}cancel-in-progress: (?<value>.+)$/mu,
    )?.groups?.value;

    expect(group).toContain("release-publishers");
    expect(group).not.toContain("github.run_id");
    expect(group).toContain("main-edge");
    expect(cancellation).toContain("github.event_name == 'push'");
    expect(releaseWorkflow).toContain("Revalidate publication source");
    expect(releaseWorkflow).toContain(
      "Confirm publication source remains current",
    );
    expect(releaseWorkflow).toContain(
      "ref: ${{ github.event.repository.default_branch }}",
    );
    expect(releaseWorkflow).toContain("include-hidden-files: true");
    expect(releaseWorkflow).toContain("https://$REGISTRY/token");
    expect(releaseWorkflow).toContain("Authorization: Bearer $bearer");
    expect(releaseWorkflow).not.toContain(
      '--user "$REGISTRY_USERNAME:$REGISTRY_TOKEN" --header \'Accept:',
    );
    expect(releaseWorkflow).toContain("steps.plan.outputs.tags_to_copy");
    expect(releaseWorkflow).toContain(
      "steps.latest.outputs.publish_latest == 'true'",
    );
    expect(releaseWorkflow).toContain(
      'gh api "repos/$GITHUB_REPOSITORY/releases/latest"',
    );
    expect(releaseWorkflow).toContain(
      '"$current_latest_release_tag" != "$RELEASE_TAG"',
    );
    expect(releaseWorkflow).toContain("revalidate_source");
    expect(releaseWorkflow).toContain(
      '"$current_latest_release_tag" == "$RELEASE_TAG"',
    );
    expect(releaseWorkflow).toContain("Verify every published tag");
    expect(releaseWorkflow).toContain("tar --extract --to-stdout");
    expect(releaseWorkflow).not.toContain(
      'tar --extract --file "$OCI_ARCHIVE" --directory',
    );
    expect(releaseWorkflow).toContain(
      '"$published_digest" == "$EXPECTED_DIGEST"',
    );
    expect(releaseWorkflow).toContain("copy --all");
    expect(releaseWorkflow).toContain("--preserve-digests");
  });
});
