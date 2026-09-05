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
  if (!Array.isArray(tags) || tags.some((tag) => typeof tag !== "string" || !tag)) {
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

if (process.argv[1] === new URL(import.meta.url).pathname) {
  const state = resolveImmutableTag({
    expectedDigest: process.env.EXPECTED_DIGEST,
    publishedDigest: process.env.PUBLISHED_DIGEST,
  });
  process.stdout.write(`immutable_state=${state}\n`);
}
