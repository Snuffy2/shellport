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
      activate: () => {
        calls.push("activate");
      },
      deactivate: () => {
        calls.push("deactivate");
      },
    });

    expect(calls).toEqual(["nextTick", "activate"]);
  });
});
