// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "vitest";
import { triggerConsoleActive } from "./screen_console_activation.js";

describe("screen console", function () {
  test("waits for tab visibility DOM updates before activating the terminal", async function () {
    const calls = [];

    await triggerConsoleActive({
      active: true,
      nextTick: async () => {
        calls.push("nextTick");
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

    expect(calls).toEqual(["nextTick", "activate", "refresh"]);
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
});
