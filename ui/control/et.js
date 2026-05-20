// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import * as common from "../commands/common.js";
import * as reader from "../stream/reader.js";
import * as subscribe from "../stream/subscribe.js";
import * as iconvDecoder from "../iconv/decoder.js";
import * as iconvEncoder from "../iconv/encoder.js";

class Control {
  constructor(data, color) {
    this.background = color;
    this.charset = data.charset;
    this.enable = false;
    this.sender = data.send;
    this.closer = data.close;
    this.closed = false;
    this.resizer = data.resize;
    this.subs = new subscribe.Subscribe();
    let self = this;
    this.charsetEncoder = new iconvEncoder.IconvEncoder(
      (o) => self.sender(o),
      this.charset,
    );
    let charsetDecoder = new iconvDecoder.IconvDecoder(
      (o) => self.subs.resolve(o),
      this.charset,
    );
    data.events.place("stdout", async (rd) => {
      try {
        charsetDecoder.write(await reader.readCompletely(rd));
      } catch (e) {
        console.error("ET stdout stream/decode error", {
          error: e,
          reader: rd,
        });
      }
    });
    data.events.place("completed", () => {
      self.closed = true;
      self.background.forget();
      self.charsetEncoder.close();
      charsetDecoder.close();
      self.subs.reject("Remote connection has been terminated");
    });
  }

  echo() {
    return false;
  }

  resize(dim) {
    if (this.closed) {
      return;
    }
    this.resizer(dim.rows, dim.cols);
  }

  enabled() {
    this.enable = true;
  }

  disabled() {
    this.enable = false;
  }

  retap(_isOn) {}

  receive() {
    return this.subs.subscribe();
  }

  send(data) {
    if (this.closed) {
      return;
    }
    return this.charsetEncoder.write(data);
  }

  sendBinary(data) {
    if (this.closed) {
      return;
    }
    return this.sender(common.strToBinary(data));
  }

  color() {
    return this.background.hex();
  }

  close() {
    if (this.closer === null) {
      return;
    }
    let cc = this.closer;
    this.closer = null;
    return cc();
  }
}

export class ET {
  /**
   * constructor
   *
   * @param {import('../commands/color.js').Colors} c
   */
  constructor(c) {
    this.colors = c;
  }

  type() {
    return "ET";
  }

  ui() {
    return "Console";
  }

  build(data) {
    return new Control(data, this.colors.get(data.tabColor));
  }
}
