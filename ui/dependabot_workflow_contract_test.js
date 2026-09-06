// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import * as prettier from "prettier";
import { describe, expect, test } from "vitest";

function yamlValue(node) {
  if (
    [
      "plain",
      "quoteDouble",
      "quoteSingle",
      "blockFolded",
      "blockLiteral",
    ].includes(node.type)
  )
    return node.value ?? "";
  if (node.type === "mapping")
    return Object.fromEntries(
      (node.children ?? []).map((item) => {
        const [key, value] = item.children ?? [];
        return [yamlValue(key).toString(), yamlValue(value)];
      }),
    );
  if (node.type === "sequence")
    return (node.children ?? []).map((item) => yamlValue(item));
  for (const child of node.children ?? []) {
    const value = yamlValue(child);
    if (value !== null) return value;
  }
  return null;
}

async function workflow(path) {
  const source = readFileSync(resolve(path), "utf8");
  const parsed = await prettier.__debug.parse(source, { parser: "yaml" });
  return yamlValue(parsed.ast);
}

function steps(job) {
  return job.steps;
}

function authorizationStep(job) {
  const step = steps(job).find((candidate) =>
    candidate.run?.includes("dependabot-auto-merge.mjs"),
  );
  expect(step).toBeDefined();
  return step;
}

function trustedCheckoutBefore(job) {
  const jobSteps = steps(job);
  const authorizationIndex = jobSteps.indexOf(authorizationStep(job));
  const checkout = jobSteps
    .slice(0, authorizationIndex)
    .find(
      (candidate) =>
        candidate.uses?.startsWith("actions/checkout@") &&
        candidate.with?.ref === "${{ github.event.pull_request.base.sha }}",
    );
  expect(checkout).toBeDefined();
  expect(checkout.with["persist-credentials"]).toBe("false");
}

function requiresEligibleDependabot(condition) {
  const value = String(condition);
  for (const term of [
    "repository.fork == false",
    "pull_request.user.login == 'dependabot[bot]'",
    "pull_request.head.repo.full_name == github.repository",
    "pull_request.base.ref == github.event.repository.default_branch",
  ])
    expect(value).toContain(term);
}

function requiresDependabotPullRequest(condition) {
  const value = String(condition);
  expect(value).toContain("github.event_name == 'pull_request'");
  requiresDependabotAuthor(condition);
}

function requiresDependabotAuthor(condition) {
  const value = String(condition);
  expect(value).toContain("pull_request.user.login == 'dependabot[bot]'");
  for (const excludedRestriction of [
    "repository.fork == false",
    "pull_request.head.repo.full_name == github.repository",
    "pull_request.base.ref == github.event.repository.default_branch",
  ])
    expect(value).not.toContain(excludedRestriction);
}

function assertsAuthoritativeDataflow(job) {
  const run = authorizationStep(job).run;
  expect(run).toMatch(/pulls.*files/);
  expect(run).toMatch(/pulls.*commits/);
  expect(run).toContain("compare/");
  expect(run).toContain("dependabot-auto-merge.mjs");
}

function authorizationJob(workflowDefinition) {
  const job = Object.values(workflowDefinition.jobs).find((candidate) =>
    steps(candidate).some((step) =>
      step.run?.includes("dependabot-auto-merge.mjs"),
    ),
  );
  expect(job).toBeDefined();
  return job;
}

describe("Dependabot workflow trust contracts", () => {
  test("uses trusted read-only authorization with PR data and ancestry evidence", async () => {
    const autoMerge = await workflow(
      ".github/workflows/dependabot-auto-merge.yml",
    );
    const ci = await workflow(".github/workflows/ci.yml");
    const authorization = authorizationJob(autoMerge);
    const ciAuthorization = authorizationJob(ci);
    expect(authorization.permissions).toMatchObject({
      contents: "read",
      "pull-requests": "read",
    });
    expect(ciAuthorization.permissions).toMatchObject({
      contents: "read",
      "pull-requests": "read",
    });
    requiresDependabotAuthor(authorization.if);
    requiresDependabotPullRequest(steps(ciAuthorization)[0].if);
    requiresDependabotPullRequest(authorizationStep(ciAuthorization).if);
    trustedCheckoutBefore(authorization);
    trustedCheckoutBefore(ciAuthorization);
    assertsAuthoritativeDataflow(authorization);
    assertsAuthoritativeDataflow(ciAuthorization);
  });

  test("keeps write jobs dependent on authorization and checkout-free", async () => {
    const autoMerge = await workflow(
      ".github/workflows/dependabot-auto-merge.yml",
    );
    const authorization = authorizationJob(autoMerge);
    const authorizationName = Object.entries(autoMerge.jobs).find(
      ([, candidate]) => Object.is(candidate, authorization),
    )[0];
    const writeJobs = Object.values(autoMerge.jobs).filter((job) =>
      [job.permissions?.contents, job.permissions?.["pull-requests"]].includes(
        "write",
      ),
    );
    expect(writeJobs).not.toHaveLength(0);
    for (const job of writeJobs) {
      expect(String(job.needs)).toContain(authorizationName);
      expect(
        steps(job).some((step) => step.uses?.startsWith("actions/checkout@")),
      ).toBe(false);
    }
    const enable = writeJobs.find(
      (job) => !String(job.if).includes("failure()"),
    );
    expect(enable).toBeDefined();
    expect(enable.if).toBeUndefined();
  });

  test("uses cancellation-safe cleanup under the eligibility guard", async () => {
    const autoMerge = await workflow(
      ".github/workflows/dependabot-auto-merge.yml",
    );
    const cleanup = Object.values(autoMerge.jobs).find((job) =>
      String(job.if).includes("failure()"),
    );
    expect(cleanup).toBeDefined();
    expect(String(cleanup.if)).toContain("!cancelled()");
    requiresEligibleDependabot(cleanup.if);
  });

  test("authorizes eligible Dependabot PRs before normal CI checks out their head", async () => {
    const ci = await workflow(".github/workflows/ci.yml");
    const ciAuthorization = authorizationJob(ci);
    trustedCheckoutBefore(ciAuthorization);
    const jobSteps = steps(ciAuthorization);
    const authorizationIndex = jobSteps.indexOf(
      authorizationStep(ciAuthorization),
    );
    const headCheckoutIndex = jobSteps.findIndex(
      (step) => step.uses?.startsWith("actions/checkout@") && !step.with?.ref,
    );
    expect(headCheckoutIndex).toBeGreaterThan(authorizationIndex);
  });
});
