// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import * as address from "./address.js";
import * as command from "./commands.js";
import * as common from "./common.js";
import * as presets from "./presets.js";
import * as ssh from "./ssh.js";
import * as strings from "./string.js";

describe("SSH Command", () => {
  it("encodes terminal resize dimensions", async () => {
    let commandHandler = null;
    let streamSends = [];
    let initialSends = [];

    const streams = {
      request(_commandId, builder) {
        const streamSender = {
          send(marker, payload) {
            streamSends.push({
              marker,
              payload: Uint8Array.from(payload),
            });

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
        assert.strictEqual(type, "SSH");

        return {
          build() {
            return {
              charset: "",
              tabColor: "",
              send() {},
              close() {},
              resize() {},
            };
          },
          ui() {
            return "SSH";
          },
        };
      },
    };
    const wizard = new ssh.Command().wizard(
      new command.Info(new ssh.Command()),
      null,
      null,
      [],
      streams,
      { resolve() {} },
      controls,
      { save() {} },
    );
    const prompt = wizard.stepInitialPrompt().data();
    const parsedHost = address.parseHostPort("example.com:22", 22);
    const expectedUser = new strings.String(
      common.strToUint8Array("alice"),
    ).buffer();
    const expectedAddr = new address.Address(
      parsedHost.type,
      parsedHost.address,
      parsedHost.port,
    ).buffer();
    const expected = new Uint8Array(
      expectedUser.length + expectedAddr.length + 1,
    );

    prompt.respond({
      user: "alice",
      host: "example.com:22",
      authentication: "Password",
      encoding: "utf-8",
    });

    assert.ok(commandHandler);
    assert.strictEqual(initialSends.length, 1);
    expected.set(expectedUser, 0);
    expected.set(expectedAddr, expectedUser.length);
    expected[expectedUser.length + expectedAddr.length] = 0x01;
    assert.deepStrictEqual(initialSends[0], expected);

    await commandHandler.sendResize(40, 120);

    assert.strictEqual(streamSends.length, 1);
    assert.strictEqual(streamSends[0].marker, 0x01);
    assert.deepStrictEqual(
      Array.from(streamSends[0].payload),
      [0x00, 0x28, 0x00, 0x78],
    );
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
        assert.strictEqual(type, "SSH");

        return {
          build() {
            return {
              charset: "",
              tabColor: "",
              send() {},
              close() {},
              resize() {},
            };
          },
          ui() {
            return "SSH";
          },
        };
      },
    };
    const wizard = new ssh.Command().wizard(
      new command.Info(new ssh.Command()),
      new presets.Preset({
        id: "preset-atlantis",
        title: "Atlantis",
        type: "SSH",
        host: "example.com:22",
        meta: {},
      }),
      null,
      [],
      streams,
      { resolve() {} },
      controls,
      { save() {} },
    );
    const prompt = wizard.stepInitialPrompt().data();
    const parsedHost = address.parseHostPort("example.com:22", 22);
    const expectedUser = new strings.String(
      common.strToUint8Array("alice"),
    ).buffer();
    const expectedAddr = new address.Address(
      parsedHost.type,
      parsedHost.address,
      parsedHost.port,
    ).buffer();
    const expectedPresetID = new strings.String(
      common.strToUint8Array("preset-atlantis"),
    ).buffer();
    const expected = new Uint8Array(
      expectedUser.length + expectedAddr.length + 1 + expectedPresetID.length,
    );

    prompt.respond({
      user: "alice",
      host: "example.com:22",
      authentication: "Password",
      encoding: "utf-8",
    });

    expected.set(expectedUser, 0);
    expected.set(expectedAddr, expectedUser.length);
    expected[expectedUser.length + expectedAddr.length] = 0x01;
    expected.set(
      expectedPresetID,
      expectedUser.length + expectedAddr.length + 1,
    );

    assert.ok(commandHandler);
    assert.strictEqual(initialSends.length, 1);
    assert.deepStrictEqual(initialSends[0], expected);
  });
});
