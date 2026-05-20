// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import * as reader from "../stream/reader.js";
import * as header from "../stream/header.js";
import * as address from "./address.js";
import * as et from "./et.js";
import * as command from "./commands.js";
import * as presets from "./presets.js";
import * as strings from "./string.js";

describe("ET Command", () => {
  const callbacks = {
    "initialization.failed"() {},
    initialized() {},
    "hook.before_connected"() {},
    "connect.failed"() {},
    "connect.succeed"() {},
    "connect.fingerprint"() {},
    "connect.credential"() {},
    "@stdout"() {},
    close() {},
    "@completed"() {},
  };

  function makeReader(buffer) {
    let rd = new reader.Reader(new reader.Multiple(() => {}), (data) => data);

    rd.feed(buffer);

    return rd;
  }

  it("uses the ET command id", () => {
    assert.strictEqual(new et.Command().id(), 0x03);
  });

  it("includes ET metadata in the initial payload", async () => {
    let sent = null;
    const commandHandler = new et.ET(
      null,
      {
        user: new TextEncoder().encode("alice"),
        host: address.parseHostPort("example.com:22", 22),
        auth: 0x02,
        charset: "utf-8",
        etServerPort: "22022",
        etCommand: "/usr/local/bin/et",
        presetID: "preset-et",
      },
      callbacks,
    );

    commandHandler.run({
      send(data) {
        sent = data;
      },
    });

    const rd = makeReader(sent);

    assert.deepStrictEqual(
      (await strings.String.read(rd)).data(),
      new TextEncoder().encode("alice"),
    );
    assert.strictEqual((await address.Address.read(rd)).port(), 22);
    assert.strictEqual((await reader.readOne(rd))[0], 0x02);
    assert.deepStrictEqual(
      (await strings.String.read(rd)).data(),
      new TextEncoder().encode("22022"),
    );
    assert.deepStrictEqual(
      (await strings.String.read(rd)).data(),
      new TextEncoder().encode("/usr/local/bin/et"),
    );
    assert.deepStrictEqual(
      (await strings.String.read(rd)).data(),
      new TextEncoder().encode("preset-et"),
    );
  });

  it("validates ET server port and command fields", () => {
    const wizard = new et.Command().wizard(
      null,
      presets.emptyPreset(),
      {},
      [],
      null,
      null,
      {
        get(type) {
          assert.strictEqual(type, "ET");

          return {};
        },
      },
      null,
    );
    const fields = wizard.stepInitialPrompt().data().inputs;
    const port = fields.find((field) => field.name === "ET Server Port");
    const commandField = fields.find((field) => field.name === "ET Command");

    assert.strictEqual(
      port.verify("2022"),
      "Will connect to etserver port 2022",
    );
    assert.throws(() => port.verify("0"), /between 1 and 65535/);
    assert.throws(() => port.verify("abc"), /numeric/);
    assert.strictEqual(commandField.verify("et"), "Will run et");
    assert.throws(() => commandField.verify("et --flag"), /without arguments/);
  });

  it("maps unsupported proxy initialization failure to a clear message", () => {
    let commandHandler = null;
    let resolvedStep = null;
    const initialHeader = new header.InitialStream(0, 0);
    const wizard = new et.Command();

    wizard
      .execute(
        null,
        {
          user: "alice",
          host: "example.com:22",
          authentication: "Private Key",
          charset: "utf-8",
          etServerPort: "2022",
          etCommand: "et",
        },
        {},
        [],
        {
          request(id, callback) {
            assert.strictEqual(id, 0x03);

            commandHandler = callback({ close() {} });
          },
        },
        {
          resolve(step) {
            resolvedStep = step;
          },
        },
        {
          get(type) {
            assert.strictEqual(type, "ET");

            return {};
          },
        },
        null,
      )
      .stepInitialPrompt();

    initialHeader.set(0x03, 0x04, false);
    commandHandler.initialize(initialHeader);

    assert.strictEqual(resolvedStep.type(), command.NEXT_DONE);
    assert.strictEqual(resolvedStep.data().success, false);
    assert.strictEqual(
      resolvedStep.data().errorMessage,
      "ET does not support SOCKS5 proxying in this version",
    );
  });

  it("maps bad metadata initialization failure to a clear message", () => {
    let commandHandler = null;
    let resolvedStep = null;
    const initialHeader = new header.InitialStream(0, 0);

    new et.Command()
      .execute(
        null,
        {
          user: "alice",
          host: "example.com:22",
          authentication: "Private Key",
          charset: "utf-8",
          etServerPort: "2022",
          etCommand: "et",
        },
        {},
        [],
        {
          request(id, callback) {
            assert.strictEqual(id, 0x03);

            commandHandler = callback({ close() {} });
          },
        },
        {
          resolve(step) {
            resolvedStep = step;
          },
        },
        {
          get(type) {
            assert.strictEqual(type, "ET");

            return {};
          },
        },
        null,
      )
      .stepInitialPrompt();

    initialHeader.set(0x03, 0x05, false);
    commandHandler.initialize(initialHeader);

    assert.strictEqual(resolvedStep.type(), command.NEXT_DONE);
    assert.strictEqual(resolvedStep.data().success, false);
    assert.strictEqual(resolvedStep.data().errorMessage, "Invalid ET metadata");
  });

  it("fires stdout as the resolved event name", () => {
    let stdoutCalled = false;
    const commandHandler = new et.ET(
      {
        close() {},
        sendData() {},
        send() {},
      },
      {
        user: new TextEncoder().encode("alice"),
        host: address.parseHostPort("example.com:22", 22),
        auth: 0x02,
        charset: "utf-8",
        etServerPort: "2022",
        etCommand: "et",
      },
      {
        "initialization.failed"() {},
        initialized() {},
        "hook.before_connected"() {},
        "connect.failed"() {},
        "connect.succeed"() {},
        "connect.fingerprint"() {},
        "connect.credential"() {},
        "@stdout"() {},
        close() {},
        "@completed"() {},
      },
    );
    commandHandler.events.place("stdout", () => {
      stdoutCalled = true;
    });

    commandHandler.connected = true;
    const h = new header.Stream(0, 0);
    const rd = new reader.Reader(new reader.Multiple(() => {}), (data) => data);

    h.set(0x00, 0);
    commandHandler.tick(h, rd);

    assert.strictEqual(stdoutCalled, true);
  });

  it("builds a hook-output prompt", () => {
    const wizard = new et.Command().wizard(
      null,
      presets.emptyPreset(),
      {},
      [],
      null,
      null,
      {
        get() {
          return {
            build() {
              return {};
            },
            ui() {
              return "";
            },
          };
        },
      },
      null,
    );

    const prompt = wizard.stepHookOutputPrompt("Waiting", "abc");

    assert.strictEqual(prompt.type(), command.NEXT_WAIT);
    assert.strictEqual(prompt.data().title, "Waiting");
    assert.strictEqual(prompt.data().message, "abc");
  });
});
