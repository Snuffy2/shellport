// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * @file xhr.js
 * @description Minimal XHR helpers for HTTP requests used by the ShellPort
 * authentication and keep-alive flows. Wraps XMLHttpRequest in Promises and
 * exposes `get` and `options` convenience exports.
 */

/**
 * Sends an asynchronous XHR request and returns a promise that resolves with
 * the completed `XMLHttpRequest` object, or rejects on network/timeout error.
 *
 * @private
 * @param {string} method - HTTP method (e.g. `"GET"`, `"OPTIONS"`).
 * @param {string} url - Target URL.
 * @param {Object.<string, string>} headers - Map of request header name → value.
 * @param {string|null} body - Optional request body.
 * @param {number} timeout - Optional request timeout in milliseconds.
 * @returns {Promise<XMLHttpRequest>} Resolves with the finished XHR instance,
 *   giving callers access to status, response text, and headers.
 * @throws Will reject if the request encounters a network error or times out.
 */
function send(method, url, headers, body = null, timeout = 0) {
  return new Promise((res, rej) => {
    let authReq = new XMLHttpRequest();

    authReq.addEventListener("readystatechange", () => {
      if (authReq.readyState !== authReq.DONE) {
        return;
      }

      res(authReq);
    });

    authReq.addEventListener("error", (e) => {
      rej(e);
    });

    authReq.addEventListener("timeout", (e) => {
      rej(e);
    });

    authReq.open(method, url, true);
    authReq.timeout = timeout;

    for (let h in headers) {
      authReq.setRequestHeader(h, headers[h]);
    }

    authReq.send(body);
  });
}

/**
 * Issues an HTTP GET request.
 *
 * @param {string} url - Target URL.
 * @param {Object.<string, string>} headers - Additional request headers.
 * @returns {Promise<XMLHttpRequest>} Resolves with the completed XHR instance.
 */
export function get(url, headers) {
  return send("GET", url, headers);
}

/**
 * Issues an HTTP PUT request with a request body.
 *
 * @param {string} url - Target URL.
 * @param {Object.<string, string>} headers - Additional request headers.
 * @param {string} body - Request body.
 * @returns {Promise<XMLHttpRequest>} Resolves with the completed XHR instance.
 */
export function put(url, headers, body) {
  return send("PUT", url, headers, body);
}

/**
 * Issues an HTTP OPTIONS request, used as a keep-alive ping for the backend
 * WebSocket endpoint to prevent idle connection drops by proxies.
 *
 * @param {string} url - Target URL.
 * @param {Object.<string, string>} headers - Additional request headers.
 * @param {number} timeout - Optional request timeout in milliseconds.
 * @returns {Promise<XMLHttpRequest>} Resolves with the completed XHR instance.
 */
export function options(url, headers, timeout = 0) {
  return send("OPTIONS", url, headers, null, timeout);
}
