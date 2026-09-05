// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

const digestPattern = /^sha256:[0-9a-f]{64}$/u;

function fail(message) {
  throw new Error(`Refusing Docker publication: ${message}`);
}

function assertDigest(digest, name) {
  if (!digestPattern.test(digest)) {
    fail(`${name} is not a sha256 OCI manifest digest`);
  }
}

export function resolveImmutableTag({ expectedDigest, publishedDigest }) {
  assertDigest(expectedDigest, "verified OCI archive index digest");
  if (publishedDigest === undefined || publishedDigest === "") return "absent";
  assertDigest(publishedDigest, "published registry manifest digest");
  if (publishedDigest !== expectedDigest) {
    fail(
      `immutable image version names ${publishedDigest}, not verified archive ${expectedDigest}`,
    );
  }
  return "matching";
}

export function tagsToCopy({ tags, immutableTag, immutableState }) {
  if (
    !Array.isArray(tags) ||
    tags.some((tag) => typeof tag !== "string" || !tag)
  ) {
    fail("metadata action returned invalid image tags");
  }
  if (immutableState === "matching") {
    if (typeof immutableTag !== "string" || immutableTag.length === 0) {
      fail("matching immutable version is missing its image tag");
    }
    return tags.filter((tag) => tag !== immutableTag);
  }
  if (immutableState === "" || immutableState === "absent") return tags;
  fail(`unknown immutable image state ${immutableState}`);
}

export function assertPlatformIndex(index) {
  if (!Array.isArray(index?.manifests)) {
    fail("verified OCI archive is missing a manifest index");
  }
  const platforms = index.manifests
    .map((manifest) => manifest?.platform)
    .filter(
      (platform) =>
        platform &&
        platform.os !== "unknown" &&
        platform.architecture !== "unknown",
    )
    .map((platform) => `${platform.os}/${platform.architecture}`)
    .sort();
  if (
    platforms.length !== 2 ||
    platforms[0] !== "linux/amd64" ||
    platforms[1] !== "linux/arm64"
  ) {
    fail(
      `verified OCI archive platforms are ${platforms.join(", ") || "empty"}, not linux/amd64 and linux/arm64`,
    );
  }
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  if (process.env.MODE === "validate-platforms") {
    assertPlatformIndex(JSON.parse(process.env.MANIFEST_INDEX));
  } else {
    const state =
      process.env.IMMUTABLE_VERSION === "true"
        ? resolveImmutableTag({
            expectedDigest: process.env.EXPECTED_DIGEST,
            publishedDigest: process.env.PUBLISHED_DIGEST,
          })
        : "";
    const tags = tagsToCopy({
      tags: process.env.TAGS.split("\n").filter(Boolean),
      immutableTag: process.env.IMMUTABLE_TAG,
      immutableState: state,
    });
    process.stdout.write(
      `immutable_state=${state}\ntags_to_copy<<__RELEASE_TAGS__\n` +
        `${tags.join("\n")}\n__RELEASE_TAGS__\n`,
    );
  }
}
