// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import * as header from "./header.js";
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
});
