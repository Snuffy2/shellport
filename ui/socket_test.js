// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import { beforeEach, describe, expect, it, vi } from "vitest";

const streamMocks = vi.hoisted(() => {
  const state = {
    instances: [],
  };

  class FakeStreams {
    constructor(reader, sender, config) {
      this.reader = reader;
      this.sender = sender;
      this.config = config;
      this.served = false;
      this.stop = false;
      this.clearedWith = undefined;
      state.instances.push(this);
    }

    serve() {
      this.served = true;

      return new Promise(() => {});
    }

    pause() {
      return this.sender.send(new Uint8Array([1]));
    }

    resume() {
      return this.sender.send(new Uint8Array([2]));
    }

    clear(e) {
      this.stop = true;
      this.clearedWith = e;
      this.config.cleared(e);

      return Promise.resolve();
    }
  }

  return { FakeStreams, state };
});

vi.mock("./stream/streams.js", () => ({
  ECHO_FAILED: -1,
  Streams: streamMocks.FakeStreams,
}));

const { Socket } = await import("./socket.js");

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });

  return { promise, resolve, reject };
}

function buildConnection() {
  return {
    reader: {
      close: vi.fn(),
    },
    sender: {
      close: vi.fn(() => Promise.resolve()),
      setDelay: vi.fn(),
    },
    ws: {
      close: vi.fn(),
    },
  };
}

function buildCallbacks() {
  return {
    connecting: vi.fn(),
    connected: vi.fn(),
    failed: vi.fn(),
    close: vi.fn(),
    traffic: vi.fn(),
    echo: vi.fn(),
  };
}

describe("Socket", () => {
  beforeEach(() => {
    streamMocks.state.instances = [];
  });

  it("ignores and closes an in-flight dial after close", async () => {
    const socket = new Socket({}, {}, 1000, 1000);
    const dial = deferred();
    const conn = buildConnection();
    const callbacks = buildCallbacks();

    socket.dial.dial = vi.fn(() => dial.promise);

    const pending = socket.get(callbacks).catch((e) => e);
    await Promise.resolve();

    await socket.close();
    dial.resolve(conn);

    const result = await pending;

    assert(result instanceof Error);
    assert.strictEqual(socket.streamHandler, null);
    assert.strictEqual(streamMocks.state.instances.length, 0);
    expect(callbacks.connected).not.toHaveBeenCalled();
    expect(callbacks.failed).not.toHaveBeenCalled();
    expect(conn.ws.close).toHaveBeenCalledTimes(1);
  });

  it("does not let stale flow-control failures clear a newer stream", async () => {
    const socket = new Socket({}, {}, 1000, 1000);
    const callbacks = buildCallbacks();
    const firstConn = buildConnection();
    const secondConn = buildConnection();
    const pauseFailure = deferred();
    let firstDialCallbacks;

    socket.dial.dial = vi
      .fn()
      .mockImplementationOnce((dialCallbacks) => {
        firstDialCallbacks = dialCallbacks;

        return Promise.resolve(firstConn);
      })
      .mockResolvedValueOnce(secondConn);

    const first = await socket.get(callbacks);

    first.sender.send = vi.fn(() => pauseFailure.promise);
    firstDialCallbacks.inbound({ size: 1024 * 32 });
    firstDialCallbacks.inboundUnpacked(new Uint8Array(1));

    await first.clear(new Error("old connection dropped"));
    const second = await socket.get(callbacks);

    pauseFailure.reject(new Error("old pause failed"));
    await Promise.resolve();

    assert.strictEqual(socket.streamHandler, second);
    assert.notStrictEqual(first, second);
    assert.strictEqual(second.clearedWith, undefined);
  });

  it("does not let a stale clear callback tear down a newer stream", async () => {
    const socket = new Socket({}, {}, 1000, 1000);
    const callbacks = buildCallbacks();
    const firstConn = buildConnection();
    const secondConn = buildConnection();

    socket.dial.dial = vi
      .fn()
      .mockResolvedValueOnce(firstConn)
      .mockResolvedValueOnce(secondConn);

    const first = await socket.get(callbacks);
    socket.streamHandler = null;
    const second = await socket.get(callbacks);

    await first.clear(new Error("late old clear"));

    assert.strictEqual(socket.streamHandler, second);
    expect(firstConn.ws.close).toHaveBeenCalledTimes(1);
    expect(secondConn.ws.close).not.toHaveBeenCalled();
  });

  it("does not assign a stream handler if close is invoked from connected", async () => {
    const socket = new Socket({}, {}, 1000, 1000);
    const conn = buildConnection();
    const callbacks = buildCallbacks();

    socket.dial.dial = vi.fn(() => Promise.resolve(conn));
    callbacks.connected.mockImplementation(() => socket.close());

    const result = await socket.get(callbacks).catch((e) => e);

    assert(result instanceof Error);
    assert.strictEqual(socket.streamHandler, null);
    assert.strictEqual(streamMocks.state.instances[0].served, false);
    expect(conn.ws.close).toHaveBeenCalled();
  });

  it("does not reuse a stream handler that is already clearing", async () => {
    const socket = new Socket({}, {}, 1000, 1000);
    const callbacks = buildCallbacks();
    const firstConn = buildConnection();
    const secondConn = buildConnection();

    socket.dial.dial = vi
      .fn()
      .mockResolvedValueOnce(firstConn)
      .mockResolvedValueOnce(secondConn);

    const first = await socket.get(callbacks);
    first.stop = true;
    const second = await socket.get(callbacks);

    assert.notStrictEqual(first, second);
    assert.strictEqual(socket.streamHandler, second);
  });
});
