// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import { afterEach, beforeEach, describe, it } from "vitest";
import * as xhr from "./xhr.js";

class FakeXMLHttpRequest {
  constructor() {
    this.DONE = 4;
    this.readyState = 0;
    this.timeout = 0;
    this.headers = {};
    this.listeners = {};
    FakeXMLHttpRequest.last = this;
  }

  addEventListener(eventName, handler) {
    this.listeners[eventName] = handler;
  }

  open(method, url, async) {
    this.method = method;
    this.url = url;
    this.async = async;
  }

  setRequestHeader(header, value) {
    this.headers[header] = value;
  }

  send(body) {
    this.body = body;
  }

  complete() {
    this.readyState = this.DONE;
    this.listeners.readystatechange();
  }
}

describe("xhr", () => {
  const realXMLHttpRequest = globalThis.XMLHttpRequest;

  beforeEach(() => {
    FakeXMLHttpRequest.last = null;
    globalThis.XMLHttpRequest = FakeXMLHttpRequest;
  });

  afterEach(() => {
    globalThis.XMLHttpRequest = realXMLHttpRequest;
  });

  it("applies explicit OPTIONS request timeouts", async () => {
    const requestPromise = xhr.options("/shellport/socket", {}, 1234);
    const request = FakeXMLHttpRequest.last;

    assert.strictEqual(request.method, "OPTIONS");
    assert.strictEqual(request.timeout, 1234);

    request.complete();
    assert.strictEqual(await requestPromise, request);
  });
});
