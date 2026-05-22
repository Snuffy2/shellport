// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import * as header from "./header.js";
import * as reader from "./reader.js";
import * as sender from "./sender.js";
import * as streams from "./streams.js";

describe("Streams", () => {
  it("flushes buffered sender data before reporting clear completion", async () => {
    const sent = [];
    let transportClosed = false;
    const sd = new sender.Sender(
      async (rawData) => {
        if (transportClosed) {
          throw new Error("transport closed before flush");
        }

        sent.push(Array.from(rawData));
      },
      8,
      1000,
      10,
    );
    const st = new streams.Streams(
      {
        close() {},
      },
      sd,
      {
        echoInterval: 1000,
        echoUpdater() {},
        cleared() {
          transportClosed = true;
        },
      },
    );
    const pending = sd.send(Uint8Array.from([1, 2]));

    await st.clear(null);
    await pending;

    assert.strictEqual(transportClosed, true);
    assert.deepStrictEqual(sent, [[1, 2]]);
  });

  it("waits for command close before completing streams during clear", async () => {
    const events = [];
    const closeGate = new Promise((resolve) => {
      setTimeout(resolve, 0);
    });
    const st = new streams.Streams(
      {
        close() {},
      },
      {
        send() {
          return Promise.resolve();
        },
        close() {
          return Promise.resolve();
        },
      },
      {
        echoInterval: 1000,
        echoUpdater() {},
        cleared() {},
      },
    );
    const streamID = 6;

    st.streams[streamID].run(
      1,
      () => ({
        run() {
          return Promise.resolve();
        },
        initialize() {},
        async close() {
          await closeGate;
          events.push("close");
        },
        completed() {
          events.push("completed");
        },
      }),
      st.sender,
    );

    await st.clear(null);

    assert.deepStrictEqual(events, ["close", "completed"]);
  });

  it("does not complete streams whose command close fails during clear", async () => {
    const events = [];
    const st = new streams.Streams(
      {
        close() {},
      },
      {
        send() {
          return Promise.resolve();
        },
        close() {
          return Promise.resolve();
        },
      },
      {
        echoInterval: 1000,
        echoUpdater() {},
        cleared() {},
      },
    );
    const streamID = 7;

    st.streams[streamID].run(
      1,
      () => ({
        run() {
          return Promise.resolve();
        },
        initialize() {},
        close() {
          events.push("close");
          return Promise.reject(new Error("close failed"));
        },
        completed() {
          events.push("completed");
        },
      }),
      st.sender,
    );

    await st.clear(null);

    assert.deepStrictEqual(events, ["close"]);
  });

  it("acknowledges a late remote close after local close starts", async () => {
    const sent = [];
    const st = new streams.Streams(
      {
        close() {},
      },
      {
        send(data) {
          sent.push(Array.from(data));

          return Promise.resolve();
        },
      },
      {
        echoInterval: 1000,
        echoUpdater() {},
        cleared() {},
      },
    );
    const streamID = 3;
    const closeHeader = new header.Header(header.CLOSE);

    closeHeader.set(streamID);
    st.streams[streamID].run(
      1,
      (streamSender) => ({
        run() {
          return Promise.resolve();
        },
        initialize() {},
        close() {
          return streamSender.close();
        },
        completed() {},
      }),
      st.sender,
    );
    st.streams[streamID].close();

    await st.handleClose(closeHeader);

    assert.deepStrictEqual(sent, [
      [header.CLOSE | streamID],
      [header.COMPLETED | streamID],
    ]);
  });

  it("propagates completed acknowledgement send failures", async () => {
    const st = new streams.Streams(
      {
        close() {},
      },
      {
        send() {
          return Promise.reject(new Error("ack send failed"));
        },
      },
      {
        echoInterval: 1000,
        echoUpdater() {},
        cleared() {},
      },
    );
    const streamID = 2;
    const closeHeader = new header.Header(header.CLOSE);

    closeHeader.set(streamID);
    st.streams[streamID].run(
      1,
      () => ({
        run() {
          return Promise.resolve();
        },
        initialize() {},
        close() {
          return Promise.resolve("closed");
        },
        completed() {},
      }),
      st.sender,
    );

    await assert.rejects(() => st.handleClose(closeHeader), /ack send failed/);
  });

  it("does not acknowledge remote close when command close fails", async () => {
    const sent = [];
    const expectedError = new Error("command close failed");
    const st = new streams.Streams(
      {
        close() {},
      },
      {
        send(data) {
          sent.push(Array.from(data));

          return Promise.resolve();
        },
      },
      {
        echoInterval: 1000,
        echoUpdater() {},
        cleared() {},
      },
    );
    const streamID = 5;
    const closeHeader = new header.Header(header.CLOSE);

    closeHeader.set(streamID);
    st.streams[streamID].run(
      1,
      () => ({
        run() {
          return Promise.resolve();
        },
        initialize() {},
        close() {
          return Promise.reject(expectedError);
        },
        completed() {},
      }),
      st.sender,
    );

    await assert.rejects(() => st.handleClose(closeHeader), expectedError);
    assert.deepStrictEqual(sent, []);
  });

  it("drains late stream data after local close starts", async () => {
    const st = new streams.Streams(
      {
        close() {},
      },
      {
        send() {
          return Promise.resolve();
        },
      },
      {
        echoInterval: 1000,
        echoUpdater() {},
        cleared() {},
      },
    );
    let ticked = 0;
    const streamID = 4;
    const initialHeader = new header.InitialStream(0, 0);
    const streamHeader = new header.Header(header.STREAM);
    const dataHeader = new header.Stream(0, 0);
    const payload = Uint8Array.from([1, 2, 3]);

    initialHeader.set(1, 0, true);
    dataHeader.set(0, payload.length);
    const rd = new reader.Buffer(
      Uint8Array.from([...dataHeader.buffer(), ...payload]),
      () => {},
    );

    streamHeader.set(streamID);
    st.streams[streamID].run(
      1,
      (streamSender) => ({
        run() {
          return Promise.resolve();
        },
        initialize() {},
        tick() {
          ticked++;
        },
        close() {
          return streamSender.close();
        },
        completed() {},
      }),
      st.sender,
    );
    st.streams[streamID].initialize(initialHeader);
    st.streams[streamID].close();

    await st.handleStream(streamHeader, rd);

    assert.strictEqual(ticked, 0);
    assert.strictEqual(rd.remains(), 0);
  });

  it("clears the stream when a heartbeat send fails", async () => {
    const expectedError = new Error("heartbeat send failed");
    let clearedError = null;
    const st = new streams.Streams(
      {
        close() {},
      },
      {
        send() {
          return Promise.reject(expectedError);
        },
        close() {
          return Promise.resolve();
        },
      },
      {
        echoInterval: 1000,
        echoUpdater() {},
        cleared(e) {
          clearedError = e;
        },
      },
    );

    st.sendEcho();
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });

    assert.strictEqual(clearedError, expectedError);
  });

  it("handles clear callback failures after heartbeat send failures", async () => {
    const st = new streams.Streams(
      {
        close() {},
      },
      {
        send() {
          return Promise.reject(new Error("heartbeat send failed"));
        },
        close() {
          return Promise.resolve();
        },
      },
      {
        echoInterval: 1000,
        echoUpdater() {},
        cleared() {
          throw new Error("clear callback failed");
        },
      },
    );

    st.sendEcho();
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });

    assert.strictEqual(st.stop, true);
  });

  it("clears the stream after repeated missed heartbeats", async () => {
    let clearedError = null;
    const st = new streams.Streams(
      {
        close() {},
      },
      {
        send() {
          return Promise.resolve();
        },
        close() {
          return Promise.resolve();
        },
      },
      {
        echoInterval: 1000,
        echoUpdater() {},
        cleared(e) {
          clearedError = e;
        },
      },
    );

    st.sendEcho();
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });
    st.sendEcho();
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });
    st.sendEcho();
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });

    assert.match(clearedError.message, /missed heartbeat responses/);
  });

  it("handles clear callback failures after missed heartbeats", async () => {
    const st = new streams.Streams(
      {
        close() {},
      },
      {
        send() {
          return Promise.resolve();
        },
        close() {
          return Promise.resolve();
        },
      },
      {
        echoInterval: 1000,
        echoUpdater() {},
        cleared() {
          throw new Error("clear callback failed");
        },
      },
    );

    st.sendEcho();
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });
    st.sendEcho();
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });
    st.sendEcho();
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });

    assert.strictEqual(st.stop, true);
  });

  it("ignores stale echo responses without clearing the active heartbeat", async () => {
    const echoUpdates = [];
    const st = new streams.Streams(
      {
        close() {},
      },
      {
        send() {
          return Promise.resolve();
        },
        close() {
          return Promise.resolve();
        },
      },
      {
        echoInterval: 1000,
        echoUpdater(delay) {
          echoUpdates.push(delay);
        },
        cleared() {},
      },
    );
    st.lastEchoTime = new Date();
    st.lastEchoData = Uint8Array.from([1, 2, 3]);
    const staleEchoReader = new reader.Buffer(
      Uint8Array.from([header.CONTROL_ECHO, 9, 9, 9]),
      () => {},
    );

    await st.handleControl(staleEchoReader);

    assert.deepStrictEqual(Array.from(st.lastEchoData), [1, 2, 3]);
    assert.deepStrictEqual(echoUpdates, []);
  });
});
