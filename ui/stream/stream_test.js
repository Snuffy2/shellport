// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import * as header from "./header.js";
import { Stream } from "./stream.js";

describe("Stream", () => {
  it("ignores late initialization responses after local close starts", async () => {
    let initialized = 0;
    let completed = 0;
    const sent = [];
    const st = new Stream(3);
    const initialHeader = new header.InitialStream(0, 0);

    initialHeader.set(1, 0, true);

    st.run(
      1,
      (streamSender) => ({
        run() {
          return Promise.resolve();
        },
        initialize() {
          initialized++;
        },
        close() {
          return streamSender.close();
        },
        completed() {
          completed++;
        },
      }),
      {
        send(data) {
          sent.push(Array.from(data));

          return Promise.resolve();
        },
      },
    );

    st.close();
    st.initialize(initialHeader);

    assert.strictEqual(st.initializing(), false);
    assert.strictEqual(st.closing(), true);

    st.completed();

    assert.strictEqual(initialized, 0);
    assert.strictEqual(completed, 1);
    assert.deepStrictEqual(sent, [[header.CLOSE | 3]]);
  });

  it("returns command close failures to callers", async () => {
    const expectedError = new Error("close failed");
    const st = new Stream(4);

    st.run(
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
      {
        send() {
          return Promise.resolve();
        },
      },
    );

    await assert.rejects(() => st.close(), expectedError);
  });
});
