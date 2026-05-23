// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import * as address from "./address.js";
import * as command from "./commands.js";
import * as telnet from "./telnet.js";

describe("Telnet Command", () => {
  it("keeps the initial prompt free of connection help text", () => {
    const wizard = new telnet.Command().wizard(
      new command.Info(new telnet.Command()),
      null,
      null,
      [],
      null,
      null,
      {
        get(type) {
          assert.strictEqual(type, "Telnet");

          return {};
        },
      },
      null,
    );
    const fields = wizard.stepInitialPrompt().data().inputs;
    const host = fields.find((field) => field.name === "Host");
    const encoding = fields.find((field) => field.name === "Encoding");

    assert.strictEqual(host.description, "");
    assert.strictEqual(encoding.description, "");
  });

  it("does not expose a terminal resize API", async () => {
    let commandHandler = null;
    let initialSends = [];

    const streams = {
      request(_commandId, builder) {
        const streamSender = {
          send() {
            return Promise.resolve();
          },
        };

        commandHandler = builder(streamSender);

        commandHandler.run({
          send(payload) {
            initialSends.push(Uint8Array.from(payload));

            return Promise.resolve();
          },
        });
      },
    };
    const controls = {
      get(type) {
        assert.strictEqual(type, "Telnet");

        return {
          build() {
            return {
              charset: "",
              tabColor: "",
              send() {},
              close() {},
              events: {},
            };
          },
          ui() {
            return "Telnet";
          },
        };
      },
    };
    const wizard = new telnet.Command().wizard(
      new command.Info(new telnet.Command()),
      null,
      null,
      [],
      streams,
      { resolve() {} },
      controls,
      { save() {} },
    );

    wizard.stepInitialPrompt().data().respond({
      host: "example.com:23",
      encoding: "utf-8",
    });

    assert.ok(commandHandler);
    assert.strictEqual(initialSends.length, 1);
    assert.strictEqual(typeof commandHandler.sendResize, "undefined");
    assert.ok(!("sendResize" in commandHandler));
  });

  it("builds the initial connect command from form values", async () => {
    let commandHandler = null;
    let initialSends = [];

    const streams = {
      request(_commandId, builder) {
        const streamSender = {
          send() {
            return Promise.resolve();
          },
        };

        commandHandler = builder(streamSender);

        commandHandler.run({
          send(payload) {
            initialSends.push(Uint8Array.from(payload));

            return Promise.resolve();
          },
        });
      },
    };
    const controls = {
      get(type) {
        assert.strictEqual(type, "Telnet");

        return {
          build() {
            return {
              charset: "",
              tabColor: "",
              send() {},
              close() {},
              events: {},
            };
          },
          ui() {
            return "Telnet";
          },
        };
      },
    };
    const wizard = new telnet.Command().wizard(
      new command.Info(new telnet.Command()),
      null,
      null,
      [],
      streams,
      { resolve() {} },
      controls,
      { save() {} },
    );
    const parsedHost = address.parseHostPort("example.com:23", 23);
    const expected = new address.Address(
      parsedHost.type,
      parsedHost.address,
      parsedHost.port,
    ).buffer();

    wizard.stepInitialPrompt().data().respond({
      host: "example.com:23",
      encoding: "utf-8",
    });

    assert.ok(commandHandler);
    assert.strictEqual(initialSends.length, 1);
    assert.deepStrictEqual(initialSends[0], expected);
  });

  it("publishes a cancellation error when the backend completes before connect", () => {
    let commandHandler = null;
    const resolved = [];
    const streams = {
      request(_commandId, builder) {
        const streamSender = {
          send() {
            return Promise.resolve();
          },
        };

        commandHandler = builder(streamSender);

        commandHandler.run({
          send() {
            return Promise.resolve();
          },
        });

        return {
          result: Promise.resolve(),
          stream: {
            close() {},
          },
        };
      },
    };
    const controls = {
      get(type) {
        assert.strictEqual(type, "Telnet");

        return {
          build() {
            return {
              charset: "",
              tabColor: "",
              send() {},
              close() {},
              events: {},
            };
          },
          ui() {
            return "Telnet";
          },
        };
      },
    };
    const wizard = new telnet.Command().wizard(
      new command.Info(new telnet.Command()),
      null,
      null,
      [],
      streams,
      {
        resolve(step) {
          resolved.push(step);
        },
      },
      controls,
      { save() {} },
    );

    wizard.stepInitialPrompt().data().respond({
      host: "example.com:23",
      encoding: "utf-8",
    });
    commandHandler.completed();

    const finalStep = resolved.at(-1);

    assert.strictEqual(finalStep.type(), command.NEXT_DONE);
    assert.strictEqual(finalStep.data().success, false);
    assert.strictEqual(finalStep.data().errorTitle, "Operation has failed");
    assert.strictEqual(
      finalStep.data().errorMessage,
      "Connection has been cancelled",
    );
  });
});
