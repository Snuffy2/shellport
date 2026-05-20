// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "vitest";
import { triggerConsoleActive } from "./screen_console_activation.js";

describe("screen console", function () {
  test("waits for tab visibility DOM updates and a paint frame before refreshing the terminal", async function () {
    const calls = [];

    await triggerConsoleActive({
      active: true,
      nextTick: async () => {
        calls.push("nextTick");
      },
      nextFrame: async () => {
        calls.push("nextFrame");
      },
      isStillActive: () => true,
      activate: () => {
        calls.push("activate");
      },
      refresh: () => {
        calls.push("refresh");
      },
      deactivate: () => {
        calls.push("deactivate");
      },
    });

    expect(calls).toEqual(["nextTick", "activate", "nextFrame", "refresh"]);
  });

  test("deactivates immediately without waiting for a DOM update", async function () {
    const calls = [];

    await triggerConsoleActive({
      active: false,
      nextTick: async () => {
        calls.push("nextTick");
      },
      isStillActive: () => false,
      activate: () => {
        calls.push("activate");
      },
      deactivate: () => {
        calls.push("deactivate");
      },
    });

    expect(calls).toEqual(["deactivate"]);
  });

  test("skips stale activation after a rapid tab switch away", async function () {
    const calls = [];
    let resolveNextTick;
    let active = true;

    const staleActivation = triggerConsoleActive({
      active: true,
      nextTick: async () => {
        calls.push("nextTick");
        await new Promise((resolve) => {
          resolveNextTick = resolve;
        });
      },
      isStillActive: () => active,
      activate: () => {
        calls.push("activate");
      },
      deactivate: () => {
        calls.push("deactivate");
      },
    });
    active = false;
    await triggerConsoleActive({
      active: false,
      nextTick: async () => {
        calls.push("unexpectedNextTick");
      },
      isStillActive: () => active,
      activate: () => {
        calls.push("unexpectedActivate");
      },
      deactivate: () => {
        calls.push("deactivate");
      },
    });

    resolveNextTick();
    await staleActivation;

    expect(calls).toEqual(["nextTick", "deactivate"]);
  });

  test("skips refresh when activation becomes stale during the paint frame wait", async function () {
    const calls = [];
    let resolveNextFrame;
    let active = true;

    const staleActivation = triggerConsoleActive({
      active: true,
      nextTick: async () => {
        calls.push("nextTick");
      },
      nextFrame: async () => {
        calls.push("nextFrame");
        await new Promise((resolve) => {
          resolveNextFrame = resolve;
        });
      },
      isStillActive: () => active,
      activate: () => {
        calls.push("activate");
      },
      refresh: () => {
        calls.push("refresh");
      },
      deactivate: () => {
        calls.push("unexpectedDeactivate");
      },
    });

    await Promise.resolve();
    active = false;
    resolveNextFrame();
    await staleActivation;

    expect(calls).toEqual(["nextTick", "activate", "nextFrame"]);
  });
});
