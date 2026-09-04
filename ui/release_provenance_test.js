// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "vitest";

import {
  resolveReleaseSource,
  resolveWorkflowSource,
} from "../.github/scripts/release-provenance.mjs";
import { assertImageTagAvailable } from "../.github/scripts/release-registry-guard.mjs";

const eventSHA = "a".repeat(40);
const packageEndpoint =
  "users/Snuffy2/packages?package_type=container&per_page=100";
const versionsEndpoint =
  "users/Snuffy2/packages/container/shellport/versions?per_page=100";

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

function apiWith(responses) {
  return (endpoint) => {
    if (!(endpoint in responses))
      throw new Error(`unexpected API call: ${endpoint}`);
    return responses[endpoint];
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
  test("permits a verified namespace with no package", function () {
    assertImageTagAvailable({
      owner: "Snuffy2",
      packageName: "shellport",
      imageTag: "1.2.3",
      api: apiWith({ user: { login: "Snuffy2" }, [packageEndpoint]: [[]] }),
    });
  });
  test.each([
    [
      "permission failure",
      () => {
        throw new Error("HTTP 403");
      },
      "HTTP 403",
    ],
    [
      "network failure",
      () => {
        throw new Error("network unavailable");
      },
      "network unavailable",
    ],
  ])("does not convert %s into a missing package", (_name, api, message) => {
    expect(() =>
      assertImageTagAvailable({
        owner: "Snuffy2",
        packageName: "shellport",
        imageTag: "1.2.3",
        api,
      }),
    ).toThrow(message);
  });
  test("rejects an existing immutable tag", function () {
    expect(() =>
      assertImageTagAvailable({
        owner: "Snuffy2",
        packageName: "shellport",
        imageTag: "1.2.3",
        api: apiWith({
          user: { login: "Snuffy2" },
          [packageEndpoint]: [
            [
              {
                name: "shellport",
                package_type: "container",
                owner: { login: "Snuffy2" },
              },
            ],
          ],
          [versionsEndpoint]: [
            [{ metadata: { container: { tags: ["1.2.3"] } } }],
          ],
        }),
      }),
    ).toThrow("already exists");
  });
});
