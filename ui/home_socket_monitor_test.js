// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, test } from "vitest";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

function readProjectFile(relativePath) {
  return readFileSync(path.join(repoRoot, relativePath), "utf8");
}

describe("home socket monitoring", () => {
  test("home starts backend heartbeat monitoring on mount", () => {
    const source = readProjectFile("ui/home.vue");

    expect(source).toContain("this.startConnectionMonitor();");
    expect(source).toContain("startConnectionMonitor()");
    expect(source).toContain("this.connection.get(this.socket)");
  });

  test("home reconnects the backend monitor after disconnects", () => {
    const homeSource = readProjectFile("ui/home.vue");
    const socketCtlSource = readProjectFile("ui/home_socketctl.js");

    expect(homeSource).toContain("scheduleConnectionMonitorReconnect()");
    expect(homeSource).toContain("this.monitor.retryTimer = setTimeout");
    expect(socketCtlSource).toContain(
      "ctx.scheduleConnectionMonitorReconnect();",
    );
  });

  test("home resets backend monitor reconnect state after a healthy connection", () => {
    const homeSource = readProjectFile("ui/home.vue");
    const socketCtlSource = readProjectFile("ui/home_socketctl.js");

    expect(homeSource).toContain("resetConnectionMonitorReconnect()");
    expect(homeSource).toContain("this.monitor.retryDelay = 1000;");
    expect(socketCtlSource).toContain("ctx.resetConnectionMonitorReconnect();");
  });

  test("home clears backend monitor reconnect timers on unmount", () => {
    const source = readProjectFile("ui/home.vue");

    expect(source).toContain("clearConnectionMonitorReconnectTimer()");
    expect(source).toContain(
      'window.removeEventListener("online", this.onOnline);',
    );
  });

  test("home closes the active backend socket on unmount", () => {
    const source = readProjectFile("ui/home.vue");

    expect(source).toContain("if (this.connection === null) {");
    expect(source).toContain("this.connection.close().catch");
  });

  test("home ignores stale connect callbacks after cancellation", () => {
    const source = readProjectFile("ui/home.vue");

    expect(source).toContain("const serial = ++this.connector.serial;");
    expect(source).toContain("serial !== this.connector.serial");
    expect(source).toContain("this.connector.serial++;");
  });

  test("socket reuses a pending backend stream dial", () => {
    const source = readProjectFile("ui/socket.js");

    expect(source).toContain("this.streamHandlerPromise = null;");
    expect(source).toContain("if (this.streamHandlerPromise)");
    expect(source).toContain("return this.streamHandlerPromise;");
    expect(source).toContain(
      "const streamHandlerPromise = this.open(callbacks, ++this.openSerial);",
    );
  });

  test("socket keep-alive failures do not become unhandled rejections", () => {
    const source = readProjectFile("ui/socket.js");

    expect(source).toContain("xhr.options(address.keepAlive, {}).catch");
  });

  test("socket flow-control send failures clear the active stream", () => {
    const source = readProjectFile("ui/socket.js");

    expect(source).toContain("sendFlowControl");
    expect(source).toContain("if (self.streamHandler === streamHandler)");
  });

  test("socket waits for the initial nonce send during dialing", () => {
    const source = readProjectFile("ui/socket.js");

    expect(source).toContain("await sd.send(senderNonce);");
  });

  test("socket exposes a close method for active stream teardown", () => {
    const source = readProjectFile("ui/socket.js");

    expect(source).toContain("close()");
    expect(source).toContain("return this.streamHandler.clear(null);");
  });

  test("clean socket close resets stale status metrics and styling", () => {
    const source = readProjectFile("ui/home_socketctl.js");

    expect(source).toContain("this.status.delay = -1;");
    expect(source).toContain('this.windowClass = "";');
  });
});
