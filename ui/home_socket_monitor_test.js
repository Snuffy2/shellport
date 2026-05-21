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

  test("socket reuses a pending backend stream dial", () => {
    const source = readProjectFile("ui/socket.js");

    expect(source).toContain("this.streamHandlerPromise = null;");
    expect(source).toContain("if (this.streamHandlerPromise)");
    expect(source).toContain("return this.streamHandlerPromise;");
    expect(source).toContain(
      "this.streamHandlerPromise = this.open(callbacks);",
    );
  });
});
