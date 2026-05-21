// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import { describe, it } from "vitest";
import { Preset, Presets } from "./presets.js";

describe("Presets", () => {
  it("preserves preset IDs and exports writable preset data", () => {
    const preset = new Preset({
      id: "preset-atlantis",
      title: "Atlantis",
      type: "SSH",
      host: "atlantis.home:22",
      tab_color: "#123456",
      meta: {
        User: "pi",
        Fingerprint: "SHA256:old",
      },
    });

    assert.strictEqual(preset.id(), "preset-atlantis");
    assert.deepStrictEqual(preset.toConfig(), {
      id: "preset-atlantis",
      title: "Atlantis",
      type: "SSH",
      host: "atlantis.home:22",
      tab_color: "#123456",
      meta: {
        User: "pi",
        Fingerprint: "SHA256:old",
      },
    });
    assert.strictEqual(preset.hasSavedPassword(), false);
  });

  it("exports all presets as config objects", () => {
    const presets = new Presets([
      {
        id: "preset-a",
        title: "A",
        type: "SSH",
        host: "a.home:22",
        meta: {},
      },
    ]);

    assert.deepStrictEqual(presets.toConfig(), [
      {
        id: "preset-a",
        title: "A",
        type: "SSH",
        host: "a.home:22",
        tab_color: "",
        meta: {},
      },
    ]);
  });

  it("tracks has_saved_password and defaults false when omitted", () => {
    const preset = new Preset({
      id: "preset-hidden",
      title: "Hidden",
      type: "SSH",
      host: "hidden.home:22",
      meta: {
        User: "pi",
      },
      has_saved_password: true,
    });

    assert.strictEqual(preset.hasSavedPassword(), true);
  });
});
