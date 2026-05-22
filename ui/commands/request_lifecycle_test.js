// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import { afterEach, describe, it, vi } from "vitest";
import {
  CONNECTION_REQUEST_TIMEOUT_MS,
  ConnectionRequestLifecycle,
} from "./request_lifecycle.js";

describe("ConnectionRequestLifecycle", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("times out startup requests that are not accepted by the backend", () => {
    vi.useFakeTimers();
    const resolved = [];
    let closed = 0;
    const lifecycle = new ConnectionRequestLifecycle(
      {
        resolve(step) {
          resolved.push(step);
        },
      },
      (title, message) => ({ title, message }),
    );

    lifecycle.start(() => ({
      stream: {
        close() {
          closed++;
        },
      },
    }));

    vi.advanceTimersByTime(CONNECTION_REQUEST_TIMEOUT_MS);

    assert.strictEqual(closed, 1);
    assert.deepStrictEqual(resolved, [
      {
        title: "Request timed out",
        message:
          "The backend did not accept the connection request within 15 seconds.",
      },
    ]);
  });

  it("clears the timeout when the backend accepts the request", () => {
    vi.useFakeTimers();
    const resolved = [];
    let closed = 0;
    const lifecycle = new ConnectionRequestLifecycle(
      {
        resolve(step) {
          resolved.push(step);
        },
      },
      (title, message) => ({ title, message }),
    );

    lifecycle.start(() => ({
      stream: {
        close() {
          closed++;
        },
      },
    }));
    assert.strictEqual(lifecycle.accepted(), true);
    vi.advanceTimersByTime(CONNECTION_REQUEST_TIMEOUT_MS);

    assert.strictEqual(closed, 0);
    assert.deepStrictEqual(resolved, []);
  });

  it("rejects late backend acceptance after timeout", () => {
    vi.useFakeTimers();
    const lifecycle = new ConnectionRequestLifecycle(
      {
        resolve() {},
      },
      (title, message) => ({ title, message }),
    );

    lifecycle.start(() => ({
      stream: {
        close() {},
      },
    }));
    vi.advanceTimersByTime(CONNECTION_REQUEST_TIMEOUT_MS);

    assert.strictEqual(lifecycle.accepted(), false);
    assert.strictEqual(lifecycle.active(), false);
  });

  it("does not publish late steps after cancellation", () => {
    const resolved = [];
    const lifecycle = new ConnectionRequestLifecycle(
      {
        resolve(step) {
          resolved.push(step);
        },
      },
      (title, message) => ({ title, message }),
    );

    lifecycle.start(() => ({
      stream: {
        close() {},
      },
    }));
    lifecycle.cancel();

    assert.strictEqual(lifecycle.resolve({ title: "late" }), false);
    assert.deepStrictEqual(resolved, [
      {
        title: "Action cancelled",
        message: "Action has been cancelled without success",
      },
    ]);
  });

  it("does not publish late steps after completion", () => {
    const resolved = [];
    const lifecycle = new ConnectionRequestLifecycle(
      {
        resolve(step) {
          resolved.push(step);
        },
      },
      (title, message) => ({ title, message }),
    );

    lifecycle.start(() => ({
      stream: {
        close() {},
      },
    }));

    assert.strictEqual(lifecycle.resolve({ title: "connected" }), true);
    assert.strictEqual(lifecycle.complete(), true);
    assert.strictEqual(lifecycle.resolve({ title: "late" }), false);
    assert.deepStrictEqual(resolved, [{ title: "connected" }]);
  });

  it("cancels the active request stream", () => {
    const resolved = [];
    let closed = 0;
    const lifecycle = new ConnectionRequestLifecycle(
      {
        resolve(step) {
          resolved.push(step);
        },
      },
      (title, message) => ({ title, message }),
    );

    lifecycle.start(() => ({
      stream: {
        close() {
          closed++;
        },
      },
    }));
    lifecycle.cancel();
    lifecycle.cancel();

    assert.strictEqual(closed, 1);
    assert.deepStrictEqual(resolved, [
      {
        title: "Action cancelled",
        message: "Action has been cancelled without success",
      },
    ]);
  });

  it("handles asynchronous close failures during cleanup", async () => {
    const lifecycle = new ConnectionRequestLifecycle(
      {
        resolve() {},
      },
      (title, message) => ({ title, message }),
    );

    lifecycle.start(() => ({
      stream: {
        close() {
          return Promise.reject(new Error("sender closed"));
        },
      },
    }));
    lifecycle.cancel();
    await Promise.resolve();

    assert.strictEqual(lifecycle.request, null);
  });

  it("handles rejected initial sends as terminal request failures", async () => {
    const resolved = [];
    let closed = 0;
    const lifecycle = new ConnectionRequestLifecycle(
      {
        resolve(step) {
          resolved.push(step);
        },
      },
      (title, message) => ({ title, message }),
    );

    lifecycle.start(() => ({
      result: Promise.reject(new Error("send failed")),
      stream: {
        close() {
          closed++;
        },
      },
    }));
    await Promise.resolve();

    assert.strictEqual(closed, 1);
    assert.strictEqual(lifecycle.request, null);
    assert.deepStrictEqual(resolved, [
      {
        title: "Request failed",
        message: "Unable to send connection request: Error: send failed",
      },
    ]);
  });
});
