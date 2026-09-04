// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { execFileSync } from "node:child_process";

function fail(message) {
  throw new Error(`Refusing Docker publication: ${message}`);
}

function flattenPages(value) {
  if (!Array.isArray(value)) {
    fail("GitHub Packages API returned a non-array response");
  }
  return value.flat(Infinity);
}

export function assertImageTagAvailable({ owner, packageName, imageTag, api }) {
  const viewer = api("user");
  if (typeof viewer?.login !== "string" || viewer.login.length === 0) {
    fail("could not verify the authenticated GitHub identity");
  }
  const packages = flattenPages(
    api(`users/${owner}/packages?package_type=container&per_page=100`),
  );
  const target = packages.find((candidate) => candidate?.name === packageName);
  if (target === undefined) return;
  if (
    target.package_type !== "container" ||
    target.owner?.login?.toLowerCase() !== owner.toLowerCase()
  ) {
    fail("the matching package has an unexpected namespace or type");
  }
  const versions = flattenPages(
    api(
      `users/${owner}/packages/container/${packageName}/versions?per_page=100`,
    ),
  );
  if (
    versions.some((version) =>
      version?.metadata?.container?.tags?.includes(imageTag),
    )
  ) {
    fail(`immutable image tag ${imageTag} already exists`);
  }
}

function githubAPI(endpoint) {
  const output = execFileSync(
    "gh",
    ["api", "--paginate", "--slurp", endpoint],
    {
      encoding: "utf8",
      env: process.env,
      stdio: ["ignore", "pipe", "inherit"],
    },
  );
  try {
    return JSON.parse(output);
  } catch (error) {
    fail(`GitHub Packages API returned invalid JSON: ${error.message}`);
  }
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  const owner = process.env.PACKAGE_OWNER;
  const packageName = process.env.PACKAGE_NAME;
  const imageTag = process.env.IMAGE_TAG;
  if (!owner || !packageName || !imageTag)
    fail("registry guard is missing package owner, name, or image tag");
  assertImageTagAvailable({ owner, packageName, imageTag, api: githubAPI });
}
