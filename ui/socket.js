// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import * as crypt from "./crypto.js";
import * as reader from "./stream/reader.js";
import * as sender from "./stream/sender.js";
import * as streams from "./stream/streams.js";
import * as xhr from "./xhr.js";

/**
 * @file socket.js
 * @description Manages the encrypted WebSocket connection to the ShellPort
 * backend. Provides the {@link Dial} class that handles low-level WebSocket
 * lifecycle and GCM stream encryption, and the {@link Socket} class that
 * integrates it with the multiplexed stream layer.
 */

export const ECHO_FAILED = streams.ECHO_FAILED;

/** @type {number} Maximum adaptive sender delay in milliseconds. */
const maxSenderDelay = 200;
/** @type {number} Minimum adaptive sender delay in milliseconds. */
const minSenderDelay = 30;
/** @type {number} Maximum keep-alive OPTIONS timeout in milliseconds. */
const maxKeepAliveTimeout = 10000;
/** @type {number} Minimum keep-alive OPTIONS timeout in milliseconds. */
const minKeepAliveTimeout = 1000;

async function cleanupDialConnection(conn, reason) {
  if (!conn) {
    return;
  }

  if (conn.ws) {
    conn.ws.close();
  }

  if (conn.sender) {
    try {
      await conn.sender.close();
    } catch (e) {
      process.env.NODE_ENV === "development" && console.trace(e);
    }
  }

  if (conn.reader) {
    try {
      conn.reader.closeWithReason
        ? conn.reader.closeWithReason(reason)
        : conn.reader.close();
    } catch (e) {
      process.env.NODE_ENV === "development" && console.trace(e);
    }
  }
}

/**
 * Manages WebSocket connection setup and the AES-GCM encrypted framing layer.
 *
 * Each `Dial` instance holds the target address, dial timeout, and a key
 * provider. Calling {@link Dial#dial} establishes the WebSocket, negotiates
 * nonces, and returns an encrypted reader/sender pair.
 *
 * @private
 */
class Dial {
  /**
   * Creates a new Dial instance.
   *
   * @param {{ webSocket: string, keepAlive: string }} address - WebSocket URL
   *   and its HTTP keep-alive counterpart.
   * @param {number} timeout - Connection timeout in milliseconds.
   * @param {{ fetch: function(): Promise<Uint8Array> }} privateKey - Key provider
   *   whose `fetch()` method returns the raw AES key bytes.
   */
  constructor(address, timeout, privateKey) {
    this.address = address;
    this.timeout = timeout;
    this.privateKey = privateKey;
    this.keepAliveTicker = null;
  }

  /**
   * Opens a raw WebSocket connection to `address.webSocket` and resolves once
   * the connection is established, or rejects after `timeout` ms.
   *
   * Also starts a keep-alive XHR interval against `address.keepAlive` to
   * prevent idle connection drops from intermediary proxies.
   *
   * @param {{ webSocket: string, keepAlive: string }} address - Target URLs.
   * @param {number} timeout - Maximum wait time in milliseconds before closing
   *   the socket and rejecting the promise.
   * @returns {Promise<WebSocket>} Resolves with the open WebSocket instance.
   */
  connect(address, timeout) {
    const self = this;
    return new Promise((resolve, reject) => {
      let ws = new WebSocket(address.webSocket),
        promised = false,
        timeoutTimer = setTimeout(() => {
          ws.close();
        }, timeout),
        myRes = (w) => {
          if (promised) {
            return;
          }

          clearTimeout(timeoutTimer);
          promised = true;

          return resolve(w);
        },
        myRej = (e) => {
          if (promised) {
            return;
          }

          clearTimeout(timeoutTimer);
          promised = true;

          return reject(e);
        };

      const keepAliveTimeout = Math.max(
        Math.min(self.timeout / 2, maxKeepAliveTimeout),
        minKeepAliveTimeout,
      );

      if (!self.keepAliveTicker) {
        self.keepAliveTicker = setInterval(
          () => {
            xhr.options(address.keepAlive, {}, keepAliveTimeout).catch((e) => {
              process.env.NODE_ENV === "development" && console.trace(e);
            });
          },
          Math.max(self.timeout / 2, 1000),
        );
      }

      ws.addEventListener("open", (_event) => {
        myRes(ws);
      });

      ws.addEventListener("close", (event) => {
        event.toString = () => {
          return "WebSocket Error (" + event.code + ")";
        };

        myRej(event);
        clearInterval(self.keepAliveTicker);
        self.keepAliveTicker = null;
      });

      ws.addEventListener("error", (_event) => {
        ws.close();
        clearInterval(self.keepAliveTicker);
        self.keepAliveTicker = null;
      });
    });
  }

  /**
   * Retrieves the raw key bytes from the key provider.
   *
   * @returns {Promise<Uint8Array>} The raw key material returned by the provider.
   */
  async buildKeyString() {
    return this.privateKey.fetch();
  }

  /**
   * Derives and imports an AES-GCM `CryptoKey` from the key provider output.
   *
   * @returns {Promise<CryptoKey>} The imported AES-GCM key ready for encrypt/decrypt.
   */
  async buildKey() {
    let kStr = await this.buildKeyString();

    return await crypt.buildGCMKey(kStr);
  }

  /**
   * Establishes the full encrypted session: opens the WebSocket, performs
   * nonce exchange, imports the AES-GCM key, and wires up the encrypted
   * reader and sender.
   *
   * The returned object's `reader` operates on the decrypted stream; the
   * `sender` wraps outbound data in AES-GCM frames before transmission.
   *
   * @param {{ inbound: function(Blob): void, inboundUnpacked: function(Uint8Array): void,
   *   outbound: function(Uint8Array): void }} callbacks - Traffic-monitoring hooks
   *   called for raw inbound blobs, their unpacked byte arrays, and outbound frames.
   * @returns {Promise<{ reader: reader.Multiple, sender: sender.Sender, ws: WebSocket }>}
   *   The encrypted reader/sender pair and the underlying WebSocket.
   * @throws {Error} If the WebSocket connection or nonce exchange fails.
   */
  async dial(callbacks) {
    let ws = await this.connect(this.address, this.timeout);
    let rd = null,
      sd = null;

    try {
      rd = new reader.Reader(new reader.Multiple(() => {}), (data) => {
        return new Promise((resolve) => {
          let bufferReader = new FileReader();

          bufferReader.onload = (event) => {
            let d = new Uint8Array(event.target.result);

            resolve(d);

            callbacks.inboundUnpacked(d);
          };

          bufferReader.readAsArrayBuffer(data);
        });
      });

      ws.addEventListener("message", (event) => {
        callbacks.inbound(event.data);

        rd.feed(event.data);
      });

      ws.addEventListener("error", (event) => {
        event.toString = () => {
          return (
            "WebSocket Error (" + (event.code ? event.code : "Unknown") + ")"
          );
        };

        rd.closeWithReason(event);
      });

      ws.addEventListener("close", (_event) => {
        rd.closeWithReason("Connection is closed");
      });

      let sdDataConvert = (rawData) => {
          return rawData;
        },
        getSdDataConvert = () => {
          return sdDataConvert;
        };
      sd = new sender.Sender(
        async (rawData) => {
          try {
            let data = await getSdDataConvert()(rawData);

            ws.send(data.buffer);
            callbacks.outbound(data);
          } catch (e) {
            ws.close();
            rd.closeWithReason(e);

            if (process.env.NODE_ENV === "development") {
              console.error(e);
            }

            throw e;
          }
        },
        4096 - 64, // Server has a 4096 bytes receive buffer, can be no greater,
        minSenderDelay, // 30ms input delay
        10, // max 10 buffered requests
      );

      let senderNonce = crypt.generateNonce();
      await sd.send(senderNonce);

      let receiverNonce = await reader.readN(rd, crypt.GCMNonceSize);

      let key = await this.buildKey();

      sdDataConvert = async (rawData) => {
        let encoded = await crypt.encryptGCM(key, senderNonce, rawData);

        crypt.increaseNonce(senderNonce);

        let dataToSend = new Uint8Array(encoded.byteLength + 2);

        dataToSend[0] = (encoded.byteLength >> 8) & 0xff;
        dataToSend[1] = encoded.byteLength & 0xff;

        dataToSend.set(new Uint8Array(encoded), 2);

        return dataToSend;
      };

      let cgmReader = new reader.Multiple(async (r) => {
        try {
          let dSizeBytes = await reader.readN(rd, 2),
            dSize = 0;

          dSize = dSizeBytes[0];
          dSize <<= 8;
          dSize |= dSizeBytes[1];

          let decoded = await crypt.decryptGCM(
            key,
            receiverNonce,
            await reader.readN(rd, dSize),
          );

          crypt.increaseNonce(receiverNonce);

          r.feed(
            new reader.Buffer(new Uint8Array(decoded), () => {}),
            () => {},
          );
        } catch (e) {
          r.closeWithReason(e);
        }
      });

      return {
        reader: cgmReader,
        sender: sd,
        ws: ws,
      };
    } catch (e) {
      ws.close();
      if (sd !== null) {
        try {
          await sd.close();
        } catch (closeErr) {
          process.env.NODE_ENV === "development" && console.trace(closeErr);
        }
      }
      if (rd !== null) {
        rd.closeWithReason(e);
      }
      throw e;
    }
  }
}

/**
 * High-level WebSocket session manager.
 *
 * Wraps {@link Dial} and the multiplexed {@link streams.Streams} layer.
 * Caches the active stream handler so subsequent `get()` calls return the
 * same instance without re-dialing.
 */
export class Socket {
  /**
   * Creates a new Socket.
   *
   * @param {{ webSocket: string, keepAlive: string }} address - Backend URLs.
   * @param {{ fetch: function(): Promise<Uint8Array> }} privateKey - Key provider.
   * @param {number} timeout - Dial timeout in milliseconds.
   * @param {number} echoInterval - Echo heartbeat interval in milliseconds.
   */
  constructor(address, privateKey, timeout, echoInterval) {
    this.dial = new Dial(address, timeout, privateKey);
    this.echoInterval = echoInterval;
    this.streamHandler = null;
    this.streamHandlerPromise = null;
    this.openSerial = 0;
  }

  /**
   * Returns the active multiplexed stream handler, dialing if not yet connected.
   *
   * Implements back-pressure by tracking raw inbound vs. unpacked byte counts:
   * when the ratio exceeds `receiveToPauseFactor` the stream is paused, and
   * resumed once decryption catches up.
   *
   * @param {{ connecting: function(): void, connected: function(): void,
   *   failed: function(Error): void, close: function(Error|null): void,
   *   traffic: function(number, number): void,
   *   echo: function(number): void }} callbacks - Lifecycle and traffic callbacks.
   * @returns {Promise<streams.Streams>} The active stream manager.
   * @throws {Error} Re-throws any dial failure after calling `callbacks.failed`.
   */
  async get(callbacks) {
    if (this.streamHandler && !this.streamHandler.stop) {
      return this.streamHandler;
    }

    if (this.streamHandlerPromise) {
      return this.streamHandlerPromise;
    }

    const streamHandlerPromise = this.open(callbacks, ++this.openSerial);
    this.streamHandlerPromise = streamHandlerPromise;

    try {
      return await streamHandlerPromise;
    } finally {
      if (this.streamHandlerPromise === streamHandlerPromise) {
        this.streamHandlerPromise = null;
      }
    }
  }

  /**
   * Closes the active stream handler, if one is connected.
   *
   * @returns {Promise<void>} Resolves once the active stream clear completes.
   */
  close() {
    this.openSerial++;
    this.streamHandlerPromise = null;

    if (this.streamHandler === null) {
      return Promise.resolve();
    }

    return this.streamHandler.clear(null);
  }

  /**
   * Opens a new backend WebSocket stream and starts its heartbeat service.
   *
   * @private
   * @param {{ connecting: function(): void, connected: function(): void,
   *   failed: function(Error): void, close: function(Error|null): void,
   *   traffic: function(number, number): void,
   *   echo: function(number): void }} callbacks - Lifecycle and traffic callbacks.
   * @param {number} openSerial Serial number for this open attempt.
   * @returns {Promise<streams.Streams>} The active stream manager.
   * @throws {Error} Re-throws any dial failure after calling `callbacks.failed`.
   */
  async open(callbacks, openSerial) {
    let self = this;

    callbacks.connecting();

    const receiveToPauseFactor = 6,
      minReceivedToPause = 1024 * 16;

    let streamPaused = false,
      currentReceived = 0,
      currentUnpacked = 0;

    const shouldPause = () => {
      return (
        currentReceived > minReceivedToPause &&
        currentReceived > currentUnpacked * receiveToPauseFactor
      );
    };
    const sendFlowControl = (streamHandler, send) => {
      send().catch((e) => {
        if (self.streamHandler === streamHandler) {
          streamHandler.clear(e);
        }
      });
    };

    try {
      let conn = await this.dial.dial({
        inbound(data) {
          currentReceived += data.size;

          callbacks.traffic(data.size, 0);
        },
        inboundUnpacked(data) {
          currentUnpacked += data.length;

          if (currentUnpacked >= currentReceived) {
            currentUnpacked = 0;
            currentReceived = 0;
          }

          if (self.streamHandler !== null) {
            const streamHandler = self.streamHandler;

            if (streamPaused && !shouldPause()) {
              streamPaused = false;
              sendFlowControl(streamHandler, () => streamHandler.resume());

              return;
            } else if (!streamPaused && shouldPause()) {
              streamPaused = true;
              sendFlowControl(streamHandler, () => streamHandler.pause());

              return;
            }
          }
        },
        outbound(data) {
          callbacks.traffic(0, data.length);
        },
      });

      if (openSerial !== this.openSerial) {
        await cleanupDialConnection(conn, "Socket open cancelled");

        throw new Error("Socket open cancelled");
      }

      let streamHandler = new streams.Streams(conn.reader, conn.sender, {
        echoInterval: self.echoInterval,
        echoUpdater(delay) {
          const sendDelay = delay / 2;

          if (sendDelay > maxSenderDelay) {
            conn.sender.setDelay(maxSenderDelay);
          } else if (sendDelay < minSenderDelay) {
            conn.sender.setDelay(minSenderDelay);
          } else {
            conn.sender.setDelay(sendDelay);
          }

          return callbacks.echo(delay);
        },
        cleared(e) {
          const activeStream = self.streamHandler === streamHandler;

          if (activeStream) {
            self.streamHandler = null;
          }

          // Close connection first otherwise we may
          // risk sending things out
          conn.ws.close();

          if (activeStream) {
            callbacks.close(e);
          }
        },
      });

      this.streamHandler = streamHandler;
      callbacks.connected();

      if (
        openSerial !== this.openSerial ||
        this.streamHandler !== streamHandler
      ) {
        await cleanupDialConnection(conn, "Socket open cancelled");

        throw new Error("Socket open cancelled");
      }

      streamHandler.serve().catch((e) => {
        if (process.env.NODE_ENV !== "development") {
          return;
        }

        console.trace(e);
      });
    } catch (e) {
      if (openSerial === this.openSerial) {
        callbacks.failed(e);
      }

      throw e;
    }

    return this.streamHandler;
  }
}
