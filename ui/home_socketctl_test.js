// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import { describe, it } from "vitest";
import { build } from "./home_socketctl.js";
import { ECHO_FAILED } from "./socket.js";

function buildContext() {
  return {
    connector: {
      inputting: true,
    },
    reconnects: 0,
    resets: 0,
    resetConnectionMonitorReconnect() {
      this.resets++;
    },
    scheduleConnectionMonitorReconnect() {
      this.reconnects++;
    },
  };
}

describe("home socket status controller", () => {
  it("clears stale clean-disconnect status", () => {
    const ctx = buildContext();
    const ctl = build(ctx);

    ctl.connected();
    ctl.echo(700);
    ctl.close(null);

    assert.strictEqual(ctl.message, "");
    assert.strictEqual(ctl.classStyle, "");
    assert.strictEqual(ctl.windowClass, "");
    assert.strictEqual(ctl.status.delay, -1);
    assert.strictEqual(ctx.connector.inputting, false);
    assert.strictEqual(ctx.reconnects, 1);
  });

  it("marks missed echo as unmeasurable", () => {
    const ctl = build(buildContext());

    ctl.connected();
    ctl.echo(ECHO_FAILED);

    assert.strictEqual(ctl.message, "");
    assert.strictEqual(ctl.classStyle, "red flash");
    assert.strictEqual(ctl.windowClass, "red");
    assert.strictEqual(ctl.status.delay, -1);
  });

  it("resets per-session counters when reconnecting", () => {
    const ctl = build(buildContext());

    ctl.traffic(100, 50);
    ctl.echo(200);
    ctl.close(null);
    ctl.connected();
    ctl.update(new Date(Date.now() + 20000));

    assert.strictEqual(ctl.status.inbound, 0);
    assert.strictEqual(ctl.status.outbound, 0);
    assert.strictEqual(ctl.status.delayHistory[31].data, 0);
  });
});
