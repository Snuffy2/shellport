// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * @fileoverview Shared lifecycle guard for command startup requests.
 */

/** @type {number} Maximum time to wait for backend request acceptance. */
export const CONNECTION_REQUEST_TIMEOUT_MS = 15000;

/**
 * Tracks a pending backend stream request while the UI shows "Requesting".
 */
export class ConnectionRequestLifecycle {
  /**
   * @param {object} step Wizard step channel.
   * @param {function(string, string): object} buildErrorStep Error step builder.
   * @param {number} timeoutMs Backend acceptance timeout in milliseconds.
   */
  constructor(step, buildErrorStep, timeoutMs = CONNECTION_REQUEST_TIMEOUT_MS) {
    this.step = step;
    this.buildErrorStep = buildErrorStep;
    this.timeoutMs = timeoutMs;
    this.timeoutID = null;
    this.request = null;
    this.failed = false;
  }

  /**
   * Starts the request and its acceptance timeout.
   *
   * @param {function(): object} requestFactory Allocates the stream request.
   * @returns {object|null} Request object, or null when startup failed.
   */
  start(requestFactory) {
    this.clearTimeout();
    this.failed = false;

    try {
      this.request = requestFactory();
    } catch (e) {
      this.fail("Request failed", "Unable to start connection request: " + e);

      return null;
    }

    if (
      this.request &&
      this.request.result &&
      typeof this.request.result.catch === "function"
    ) {
      this.request.result.catch((e) => {
        this.fail("Request failed", "Unable to send connection request: " + e);
      });
    }

    this.timeoutID = setTimeout(() => {
      this.fail(
        "Request timed out",
        "The backend did not accept the connection request within " +
          Math.round(this.timeoutMs / 1000) +
          " seconds.",
      );
    }, this.timeoutMs);

    return this.request;
  }

  /**
   * Marks the backend request as accepted.
   *
   * @returns {void}
   */
  accepted() {
    this.clearTimeout();
  }

  /**
   * Cancels the current request and publishes a cancellation step.
   *
   * @returns {void}
   */
  cancel() {
    this.fail("Action cancelled", "Action has been cancelled without success");
  }

  /**
   * Publishes a terminal error step once.
   *
   * @param {string} title Error title.
   * @param {string} message Error message.
   * @returns {void}
   */
  fail(title, message) {
    if (this.failed) {
      return;
    }

    this.failed = true;
    this.clearTimeout();
    this.closeRequest();
    this.step.resolve(this.buildErrorStep(title, message));
  }

  /**
   * Clears the pending timeout, if any.
   *
   * @returns {void}
   */
  clearTimeout() {
    if (this.timeoutID === null) {
      return;
    }

    clearTimeout(this.timeoutID);
    this.timeoutID = null;
  }

  /**
   * Sends a stream close for the active startup request.
   *
   * @returns {void}
   */
  closeRequest() {
    if (!this.request || !this.request.stream) {
      return;
    }

    try {
      this.request.stream.close();
    } catch (e) {
      process.env.NODE_ENV === "development" && console.trace(e);
    }

    this.request = null;
  }
}
