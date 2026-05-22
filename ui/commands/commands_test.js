// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import { describe, it } from "vitest";
import * as command from "./commands.js";
import * as presets from "./presets.js";
import * as ssh from "./ssh.js";

describe("Command prompts", () => {
  it("exposes secondary prompt actions", () => {
    const prompt = command.prompt(
      "Title",
      "Message",
      "Continue",
      () => {},
      () => {},
      [],
      [
        {
          text: "Save",
          keepOpen: true,
          respond() {
            return "saved";
          },
        },
      ],
    );

    assert.deepStrictEqual(prompt.data().actions, [
      {
        text: "Save",
        keepOpen: true,
        respond: prompt.data().actions[0].respond,
      },
    ]);
    assert.strictEqual(prompt.data().actions[0].respond(), "saved");
  });

  it("forwards management callbacks to interactive command wizards", () => {
    const saveFingerprint = () => {};
    const saveAsPreset = () => {};
    let receivedSaveFingerprint = null;
    let receivedSaveAsPreset = null;
    const commands = new command.Commands([
      {
        id() {
          return 0;
        },
        name() {
          return "Fake";
        },
        description() {
          return "Fake command";
        },
        color() {
          return "#000";
        },
        wizard(
          _info,
          _preset,
          _session,
          _kept,
          _streams,
          _subs,
          _controls,
          _history,
          saver,
          savePreset,
        ) {
          receivedSaveFingerprint = saver;
          receivedSaveAsPreset = savePreset;
          return {
            run() {},
            started() {
              return false;
            },
            control() {
              return {
                ui() {
                  return "Fake";
                },
              };
            },
            close() {},
          };
        },
        execute() {},
        launch() {},
        launcher() {},
        represet(preset) {
          return preset;
        },
      },
    ]);

    commands
      .all()[0]
      .wizard(
        null,
        null,
        null,
        null,
        null,
        null,
        () => {},
        saveFingerprint,
        saveAsPreset,
      );

    assert.strictEqual(receivedSaveFingerprint, saveFingerprint);
    assert.strictEqual(receivedSaveAsPreset, saveAsPreset);
  });

  it("does not call the command close hook after a normal done step", async () => {
    let closeCalled = false;
    const steps = {
      subscribe() {
        return Promise.resolve(command.done(true, null, "", ""));
      },
    };
    const commands = new command.Commands([
      {
        id() {
          return 0;
        },
        name() {
          return "Fake";
        },
        description() {
          return "Fake command";
        },
        color() {
          return "#000";
        },
        wizard() {
          return {
            run() {},
            started() {
              return true;
            },
            control() {
              return {
                ui() {
                  return "Fake";
                },
              };
            },
            close() {
              closeCalled = true;
            },
          };
        },
        execute() {},
        launch() {},
        launcher() {},
        represet(preset) {
          return preset;
        },
      },
    ]);

    const wizard = commands
      .all()[0]
      .wizard(null, null, null, null, null, null, () => {});
    wizard.subs = steps;

    await wizard.next();

    assert.strictEqual(closeCalled, false);
  });

  it("sorts merged presets by preset title before command type", () => {
    const commands = new command.Commands([
      fakeCommand(0, "SSH"),
      fakeCommand(1, "Telnet"),
    ]);
    const merged = commands.mergePresets(
      new presets.Presets([
        presetConfig("zulu ssh", "SSH", "z.example.com:22"),
        presetConfig("alpha telnet", "Telnet", "a.example.com:23"),
        presetConfig("bravo ssh", "SSH", "b.example.com:22"),
      ]),
    );

    assert.deepStrictEqual(
      merged.map((preset) => preset.preset.title()),
      ["alpha telnet", "bravo ssh", "zulu ssh"],
    );
  });

  it("merges saved SSH presets that already include Host metadata", () => {
    const commands = new command.Commands([new ssh.Command()]);
    const merged = commands.mergePresets(
      new presets.Presets([
        {
          id: "preset-atlantis",
          title: "Atlantis",
          type: "SSH",
          host: "atlantis.home:22",
          tab_color: "",
          meta: {
            Host: "atlantis.home:22",
            User: "pi",
          },
        },
      ]),
    );

    assert.strictEqual(merged.length, 1);
    assert.strictEqual(merged[0].preset.meta("Host"), "atlantis.home:22");
  });
});

function fakeCommand(id, name) {
  return {
    id() {
      return id;
    },
    name() {
      return name;
    },
    description() {
      return name + " command";
    },
    color() {
      return "#000";
    },
    wizard() {},
    execute() {},
    launch() {},
    launcher() {},
    represet(preset) {
      return preset;
    },
  };
}

function presetConfig(title, type, host) {
  return {
    title,
    type,
    host,
    tab_color: "",
    meta: {},
  };
}
