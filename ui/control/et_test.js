// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import * as reader from "../stream/reader.js";
import * as et from "./et.js";

describe("ET Control", () => {
  /**
   * Build an ET control with captured callbacks and outbound data.
   *
   * @returns {{control: object, events: object, sent: Array<Array<number>>,
   *   resizes: Array<object>, closes: Array<string>, forgotten: Array<string>}}
   *   Built control and captured side effects.
   */
  function buildControl() {
    const events = {};
    const sent = [];
    const resizes = [];
    const closes = [];
    const forgotten = [];
    const control = new et.ET({
      get() {
        return {
          forget() {
            forgotten.push("forget");
          },
          hex() {
            return "#000000";
          },
        };
      },
    }).build({
      charset: "utf-8",
      close() {
        closes.push("close");
      },
      events: {
        place(name, callback) {
          events[name] = callback;
        },
      },
      resize(rows, cols) {
        resizes.push({ rows, cols });
      },
      send(data) {
        sent.push(Array.from(data));
      },
      tabColor: "",
    });

    return { control, events, sent, resizes, closes, forgotten };
  }

  it("decodes stdout and encodes stdin through the selected charset", async () => {
    const { control, events, sent } = buildControl();

    await events.stdout(
      new reader.Buffer(new TextEncoder().encode("hello"), () => {}),
    );
    assert.strictEqual(await control.receive(), "hello");

    control.send("ok");
    assert.deepStrictEqual(sent, [[0x6f, 0x6b]]);
  });

  it("sends binary unchanged and forwards resize", () => {
    const { control, sent, resizes } = buildControl();

    control.sendBinary("\x1b[A");
    control.resize({ rows: 40, cols: 120 });

    assert.deepStrictEqual(sent, [[0x1b, 0x5b, 0x41]]);
    assert.deepStrictEqual(resizes, [{ rows: 40, cols: 120 }]);
  });

  it("closes through the stream closer once", () => {
    const { control, closes } = buildControl();

    control.close();
    control.close();

    assert.deepStrictEqual(closes, ["close"]);
  });

  it("rejects receive subscriptions after completion", async () => {
    const { control, events, sent, resizes, forgotten } = buildControl();

    await events.completed();

    assert.deepStrictEqual(forgotten, ["forget"]);
    await assert.rejects(
      async () => control.receive(),
      /Remote connection has been terminated/,
    );

    control.send("ignored");
    control.sendBinary("ignored");
    control.resize({ rows: 1, cols: 2 });

    assert.deepStrictEqual(sent, []);
    assert.deepStrictEqual(resizes, []);
  });
});
