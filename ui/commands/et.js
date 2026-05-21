// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import * as reader from "../stream/reader.js";
import * as address from "./address.js";
import * as command from "./commands.js";
import * as common from "./common.js";
import * as event from "./events.js";
import Exception from "./exception.js";
import * as presets from "./presets.js";
import { ConnectionRequestLifecycle } from "./request_lifecycle.js";
import * as strings from "./string.js";

const _AUTHMETHOD_NONE = 0x00;
const _AUTHMETHOD_PASSPHRASE = 0x01;
const AUTHMETHOD_PRIVATE_KEY = 0x02;

const COMMAND_ID = 0x03;

const MAX_USERNAME_LEN = 127;
const MAX_PASSWORD_LEN = 4096;
const DEFAULT_PORT = 22;
const DEFAULT_ET_SERVER_PORT = "2022";
const DEFAULT_ET_COMMAND = "et";

const SERVER_REMOTE_STDOUT = 0x00;
const SERVER_HOOK_OUTPUT_BEFORE_CONNECTING = 0x01;
const SERVER_CONNECT_FAILED = 0x02;
const SERVER_CONNECTED = 0x03;
const SERVER_CONNECT_REQUEST_FINGERPRINT = 0x05;
const SERVER_CONNECT_REQUEST_CREDENTIAL = 0x06;

const CLIENT_DATA_STDIN = 0x00;
const CLIENT_DATA_RESIZE = 0x01;
const CLIENT_CONNECT_RESPOND_FINGERPRINT = 0x02;
const CLIENT_CONNECT_RESPOND_CREDENTIAL = 0x03;

const SERVER_REQUEST_ERROR_BAD_USERNAME = 0x01;
const SERVER_REQUEST_ERROR_BAD_ADDRESS = 0x02;
const SERVER_REQUEST_ERROR_BAD_AUTHMETHOD = 0x03;
const SERVER_REQUEST_ERROR_UNSUPPORTED_PROXY = 0x04;
const SERVER_REQUEST_ERROR_BAD_METADATA = 0x05;

const FingerprintPromptVerifyPassed = 0x00;
const FingerprintPromptVerifyNoRecord = 0x01;
const FingerprintPromptVerifyMismatch = 0x02;

export class ET {
  /**
   * constructor
   *
   * @param {stream.Sender} sd Stream sender
   * @param {object} config configuration
   * @param {object} callbacks Event callbacks
   *
   */
  constructor(sd, config, callbacks) {
    this.sender = sd;
    this.config = config;
    this.connected = false;
    this.events = new event.Events(
      [
        "initialization.failed",
        "initialized",
        "hook.before_connected",
        "connect.failed",
        "connect.succeed",
        "connect.fingerprint",
        "connect.credential",
        "@stdout",
        "close",
        "@completed",
      ],
      callbacks,
    );
  }

  /**
   * Send intial request
   *
   * @param {stream.InitialSender} initialSender Initial stream request sender
   *
   */
  run(initialSender) {
    let user = new strings.String(this.config.user),
      userBuf = user.buffer(),
      addr = new address.Address(
        this.config.host.type,
        this.config.host.address,
        this.config.host.port,
      ),
      addrBuf = addr.buffer(),
      authMethod = new Uint8Array([this.config.auth]),
      etServerPort = new strings.String(
        common.strToUint8Array(
          this.config.etServerPort || DEFAULT_ET_SERVER_PORT,
        ),
      ),
      etServerPortBuf = etServerPort.buffer(),
      etCommand = new strings.String(
        common.strToUint8Array(this.config.etCommand || DEFAULT_ET_COMMAND),
      ),
      etCommandBuf = etCommand.buffer(),
      presetIDBuf = this.config.presetID
        ? new strings.String(
            common.strToUint8Array(this.config.presetID),
          ).buffer()
        : new Uint8Array(0);

    let data = new Uint8Array(
      userBuf.length +
        addrBuf.length +
        1 +
        etServerPortBuf.length +
        etCommandBuf.length +
        presetIDBuf.length,
    );

    data.set(userBuf, 0);
    data.set(addrBuf, userBuf.length);
    data.set(authMethod, userBuf.length + addrBuf.length);
    data.set(etServerPortBuf, userBuf.length + addrBuf.length + 1);
    data.set(
      etCommandBuf,
      userBuf.length + addrBuf.length + 1 + etServerPortBuf.length,
    );
    if (presetIDBuf.length > 0) {
      data.set(
        presetIDBuf,
        userBuf.length +
          addrBuf.length +
          1 +
          etServerPortBuf.length +
          etCommandBuf.length,
      );
    }

    return initialSender.send(data);
  }

  /**
   * Receive the initial stream request
   *
   * @param {header.InitialStream} streamInitialHeader Server respond on the
   *                                                   initial stream request
   *
   */
  initialize(streamInitialHeader) {
    if (!streamInitialHeader.success()) {
      this.events.fire("initialization.failed", streamInitialHeader);

      return;
    }

    this.events.fire("initialized", streamInitialHeader);
  }

  /**
   * Tick the command
   *
   * @param {header.Stream} streamHeader Stream data header
   * @param {reader.Limited} rd Data reader
   *
   * @returns {any} The result of the ticking
   *
   * @throws {Exception} When the stream header type is unknown
   *
   */
  tick(streamHeader, rd) {
    switch (streamHeader.marker()) {
      case SERVER_CONNECT_REQUEST_CREDENTIAL:
        if (!this.connected) {
          return this.events.fire("connect.credential", rd, this.sender);
        }
        break;

      case SERVER_CONNECT_REQUEST_FINGERPRINT:
        if (!this.connected) {
          return this.events.fire("connect.fingerprint", rd, this.sender);
        }
        break;

      case SERVER_CONNECTED:
        if (!this.connected) {
          this.connected = true;

          return this.events.fire("connect.succeed", rd, this);
        }
        break;

      case SERVER_CONNECT_FAILED:
        if (!this.connected) {
          return this.events.fire("connect.failed", rd);
        }
        break;

      case SERVER_HOOK_OUTPUT_BEFORE_CONNECTING:
        if (!this.connected) {
          return this.events.fire("hook.before_connected", rd);
        }
        break;

      case SERVER_REMOTE_STDOUT:
        if (this.connected) {
          return this.events.fire("stdout", rd);
        }
        break;
    }

    throw new Exception("Unknown stream header marker");
  }

  /**
   * Send close signal to remote
   *
   */
  async sendClose() {
    return await this.sender.close();
  }

  /**
   * Send data to remote
   *
   * @param {Uint8Array} data
   *
   */
  async sendData(data) {
    return this.sender.sendData(CLIENT_DATA_STDIN, data);
  }

  /**
   * Send resize request
   *
   * @param {number} rows
   * @param {number} cols
   *
   */
  async sendResize(rows, cols) {
    let data = new DataView(new ArrayBuffer(4));

    data.setUint16(0, rows);
    data.setUint16(2, cols);

    return this.sender.send(CLIENT_DATA_RESIZE, new Uint8Array(data.buffer));
  }

  /**
   * Close the command
   *
   */
  async close() {
    await this.sendClose();

    return this.events.fire("close");
  }

  /**
   * Tear down the command completely
   *
   */
  completed() {
    return this.events.fire("completed");
  }
}

function validETServerPort(portText) {
  if (!/^[0-9]+$/.test(portText)) {
    return false;
  }

  const port = Number.parseInt(portText, 10);
  return port >= 1 && port <= 65535;
}

const initialFieldDef = {
  Host: {
    name: "Host",
    description: "",
    type: "text",
    value: "",
    example: "ssh.example.com:22",
    readonly: false,
    suggestions() {
      return [];
    },
    verify(d) {
      if (d.length <= 0) {
        throw new Error("Hostname must be specified");
      }

      let addr = common.splitHostPort(d, DEFAULT_PORT);

      if (addr.addr.length <= 0) {
        throw new Error("Cannot be empty");
      }

      if (addr.addr.length > address.MAX_ADDR_LEN) {
        throw new Error(
          "Can no longer than " + address.MAX_ADDR_LEN + " bytes",
        );
      }

      if (addr.port <= 0) {
        throw new Error("Port must be specified");
      }

      return "Look like " + addr.type + " address";
    },
  },
  User: {
    name: "User",
    description: "",
    type: "text",
    value: "",
    example: "guest",
    readonly: false,
    suggestions() {
      return [];
    },
    verify(d) {
      if (d.length <= 0) {
        throw new Error("Username must be specified");
      }

      if (d.length > MAX_USERNAME_LEN) {
        throw new Error(
          "Username must not longer than " + MAX_USERNAME_LEN + " bytes",
        );
      }

      return "We'll login as user \"" + d + '"';
    },
  },
  Encoding: {
    name: "Encoding",
    description: "The character encoding of the server",
    type: "select",
    value: "utf-8",
    example: "utf-8",
    readonly: true,
    suggestions() {
      return [];
    },
    verify(d) {
      if (d === "utf-8") {
        return "";
      }

      throw new Error("ET v1 supports only utf-8 encoding");
    },
  },
  "ET Server Port": {
    name: "ET Server Port",
    description: "Remote etserver TCP port",
    type: "text",
    value: DEFAULT_ET_SERVER_PORT,
    example: DEFAULT_ET_SERVER_PORT,
    readonly: false,
    suggestions() {
      return [];
    },
    verify(d) {
      if (!validETServerPort(d)) {
        if (/^[0-9]+$/.test(d)) {
          throw new Error("ET Server Port must be between 1 and 65535");
        }

        throw new Error("ET Server Port must be numeric");
      }

      const port = Number.parseInt(d, 10);
      return "Will connect to etserver port " + port;
    },
  },
  "ET Command": {
    name: "ET Command",
    description: "Local ET client command path",
    type: "text",
    value: DEFAULT_ET_COMMAND,
    example: DEFAULT_ET_COMMAND,
    readonly: true,
    suggestions() {
      return [];
    },
    verify(d) {
      if (d !== DEFAULT_ET_COMMAND) {
        throw new Error("ET Command is fixed by the ShellPort backend");
      }

      return "Will run " + d;
    },
  },
  Password: {
    name: "Password",
    description: "",
    type: "password",
    value: "",
    example: "----------",
    readonly: false,
    suggestions() {
      return [];
    },
    verify(d) {
      if (d.length <= 0) {
        throw new Error("Password must be specified");
      }

      if (d.length > MAX_PASSWORD_LEN) {
        throw new Error(
          "It's too long, make it shorter than " + MAX_PASSWORD_LEN + " bytes",
        );
      }

      return "We'll login with this password";
    },
  },
  "Private Key": {
    name: "Private Key",
    description:
      'Like the one inside <i style="color: #fff; font-style: normal;">' +
      "~/.ssh/id_rsa</i>, can&apos;t be encrypted<br /><br />" +
      'To decrypt the Private Key, use command: <i style="color: #fff;' +
      ' font-style: normal;">ssh-keygen -f /path/to/private_key -p</i><br />' +
      "<br />" +
      "It is strongly recommended to use one Private Key per SSH server if " +
      "the Private Key will be submitted to ShellPort. To generate a new SSH " +
      'key pair, use command <i style="color: #fff; font-style: normal;">' +
      "ssh-keygen -o -f /path/to/my_server_key</i> and then deploy the " +
      'generated <i style="color: #fff; font-style: normal;">' +
      "/path/to/my_server_key.pub</i> file onto the target SSH server",
    type: "textfile",
    value: "",
    example: "",
    readonly: false,
    suggestions() {
      return [];
    },
    verify(d) {
      if (d.length <= 0) {
        throw new Error("Private Key must be specified");
      }

      if (d.length > MAX_PASSWORD_LEN) {
        throw new Error(
          "It's too long, make it shorter than " + MAX_PASSWORD_LEN + " bytes",
        );
      }

      const lines = d.trim().split("\n");
      let firstLineReaded = false;

      for (let i in lines) {
        if (!firstLineReaded) {
          if (lines[i].indexOf("-") === 0) {
            firstLineReaded = true;

            if (lines[i].indexOf("RSA") <= 0) {
              break;
            }
          }

          continue;
        }

        if (lines[i].indexOf("Proc-Type: 4,ENCRYPTED") === 0) {
          throw new Error("Cannot use encrypted Private Key file");
        }

        if (lines[i].indexOf(":") > 0) {
          continue;
        }

        if (lines[i].indexOf("MII") < 0) {
          throw new Error("Cannot use encrypted Private Key file");
        }

        break;
      }

      return "We'll login with this Private Key";
    },
  },
  Authentication: {
    name: "Authentication",
    description:
      "Please make sure the authentication method that you selected is " +
      "supported by the server, otherwise it will be ignored and likely " +
      "cause the login to fail",
    type: "radio",
    value: "",
    example: "Private Key",
    readonly: false,
    suggestions() {
      return [];
    },
    verify(d) {
      switch (d) {
        case "Private Key":
          return "";

        default:
          throw new Error("Authentication method must be specified");
      }
    },
  },
  Fingerprint: {
    name: "Fingerprint",
    description:
      "Please carefully verify the fingerprint. DO NOT continue " +
      "if the fingerprint is unknown to you, otherwise you maybe " +
      "giving your own secrets to an imposter",
    type: "textdata",
    value: "",
    example: "",
    readonly: false,
    suggestions() {
      return [];
    },
    verify() {
      return "";
    },
  },
};

/**
 * Return auth method from given string
 *
 * @param {string} d string data
 *
 * @returns {number} Auth method
 *
 * @throws {Exception} When auth method is invalid
 *
 */
function getAuthMethodFromStr(d) {
  switch (d) {
    case "Private Key":
      return AUTHMETHOD_PRIVATE_KEY;

    default:
      throw new Exception("ET v1 supports Private Key authentication only");
  }
}

/**
 * Return ET Command data decoded from launcher field
 *
 * @param {string} d launcher field data
 *
 * @returns {string} ET command path
 *
 * @throws {Exception} When launcher field cannot be decoded
 *
 */
function decodeLauncherETCommand(d) {
  try {
    return decodeURIComponent(d);
  } catch (e) {
    if (!(e instanceof URIError)) {
      throw e;
    }

    throw new Exception("ET Command field was malformed");
  }
}

function normalizeETPresetFields(fields) {
  for (let i in fields) {
    if (
      fields[i].name === "Authentication" &&
      fields[i].value !== "Private Key"
    ) {
      fields[i].value = "Private Key";
    }

    if (
      fields[i].name === "ET Server Port" &&
      !validETServerPort(fields[i].value)
    ) {
      fields[i].value = DEFAULT_ET_SERVER_PORT;
    }

    if (fields[i].name === "ET Command") {
      fields[i].value = DEFAULT_ET_COMMAND;
    }
  }

  return fields;
}

class Wizard {
  /**
   * constructor
   *
   * @param {command.Info} info
   * @param {presets.Preset} preset
   * @param {object} session
   * @param {Array<string>} keptSessions
   * @param {streams.Streams} streams
   * @param {subscribe.Subscribe} subs
   * @param {controls.Controls} controls
   * @param {object} _history Deprecated connection history placeholder.
   *
   */
  constructor(
    info,
    preset,
    session,
    keptSessions,
    streams,
    subs,
    controls,
    _history,
    saveFingerprint = null,
  ) {
    this.info = info;
    this.preset = preset;
    this.hasStarted = false;
    this.streams = streams;
    this.session = session
      ? session
      : {
          credential: "",
        };
    this.keptSessions = keptSessions;
    this.step = subs;
    this.controls = controls.get("ET");
    this.saveFingerprint = saveFingerprint;
    this.requestLifecycle = new ConnectionRequestLifecycle(
      this.step,
      (title, message) => this.stepErrorDone(title, message),
    );
  }

  run() {
    this.step.resolve(this.stepInitialPrompt());
  }

  started() {
    return this.hasStarted;
  }

  control() {
    return this.controls;
  }

  close() {
    this.requestLifecycle.cancel();
  }

  stepErrorDone(title, message) {
    return command.done(false, null, title, message);
  }

  stepSuccessfulDone(data) {
    return command.done(
      true,
      data,
      "Success!",
      "We have connected to the remote",
    );
  }

  stepWaitForAcceptWait() {
    return command.wait(
      "Requesting",
      "Waiting for the request to be accepted by the backend",
    );
  }

  stepWaitForEstablishWait(host) {
    return command.wait(
      "Connecting to " + host,
      "Establishing connection with the remote host, may take a while",
    );
  }

  stepContinueWaitForEstablishWait() {
    return command.wait(
      "Connecting",
      "Establishing connection with the remote host, may take a while",
    );
  }

  stepHookOutputPrompt(title, msg) {
    return command.wait(
      title,
      strings.truncate(
        msg,
        common.MAX_HOOK_OUTPUT_LEN,
        common.HOOK_OUTPUT_STR_ELLIPSIS,
      ),
    );
  }

  buildCommand(sender, configInput, sessionData) {
    let self = this;

    let config = {
      user: common.strToUint8Array(configInput.user),
      auth: getAuthMethodFromStr(configInput.authentication),
      charset: configInput.charset,
      credential: sessionData.credential,
      host: address.parseHostPort(configInput.host, DEFAULT_PORT),
      fingerprint: configInput.fingerprint,
      etServerPort: configInput.etServerPort,
      etCommand: configInput.etCommand,
      presetID: configInput.presetID ? configInput.presetID : "",
    };

    let keptSessions = self.keptSessions ? [].concat(...self.keptSessions) : [];

    return new ET(sender, config, {
      "initialization.failed"(hd) {
        self.requestLifecycle.accepted();
        switch (hd.data()) {
          case SERVER_REQUEST_ERROR_BAD_USERNAME:
            self.step.resolve(
              self.stepErrorDone("Request failed", "Invalid username"),
            );
            return;

          case SERVER_REQUEST_ERROR_BAD_ADDRESS:
            self.step.resolve(
              self.stepErrorDone("Request failed", "Invalid address"),
            );
            return;

          case SERVER_REQUEST_ERROR_BAD_AUTHMETHOD:
            self.step.resolve(
              self.stepErrorDone(
                "Request failed",
                "ET v1 supports Private Key authentication only",
              ),
            );
            return;

          case SERVER_REQUEST_ERROR_UNSUPPORTED_PROXY:
            self.step.resolve(
              self.stepErrorDone(
                "Request failed",
                "ET does not support SOCKS5 proxying in this version",
              ),
            );
            return;

          case SERVER_REQUEST_ERROR_BAD_METADATA:
            self.step.resolve(
              self.stepErrorDone("Request failed", "Invalid ET metadata"),
            );
            return;
        }

        self.step.resolve(
          self.stepErrorDone("Request failed", "Unknown error: " + hd.data()),
        );
      },
      initialized() {
        self.requestLifecycle.accepted();
        self.step.resolve(self.stepWaitForEstablishWait(configInput.host));
      },
      async "connect.failed"(rd) {
        let d = new TextDecoder("utf-8").decode(
          await reader.readCompletely(rd),
        );
        self.step.resolve(self.stepErrorDone("Connection failed", d));
      },
      async "hook.before_connected"(rd) {
        const d = new TextDecoder("utf-8").decode(
          await reader.readCompletely(rd),
        );
        self.step.resolve(
          self.stepHookOutputPrompt("Waiting for server hook", d),
        );
      },
      "connect.succeed"(rd, commandHandler) {
        void rd;

        self.connectionSucceed = true;

        self.step.resolve(
          self.stepSuccessfulDone(
            new command.Result(
              configInput.user + "@" + configInput.host,
              self.info,
              self.controls.build({
                charset: configInput.charset,
                tabColor: configInput.tabColor,
                send(data) {
                  return commandHandler.sendData(data);
                },
                close() {
                  return commandHandler.sendClose();
                },
                resize(rows, cols) {
                  return commandHandler.sendResize(rows, cols);
                },
                events: commandHandler.events,
              }),
              self.controls.ui(),
            ),
          ),
        );

        void sessionData;
        void keptSessions;
      },
      async "connect.fingerprint"(rd, sd) {
        self.step.resolve(
          await self.stepFingerprintPrompt(
            rd,
            sd,
            (v) => {
              if (!configInput.fingerprint) {
                return FingerprintPromptVerifyNoRecord;
              }

              if (configInput.fingerprint === v) {
                return FingerprintPromptVerifyPassed;
              }

              return FingerprintPromptVerifyMismatch;
            },
            (newFingerprint) => {
              configInput.fingerprint = newFingerprint;
            },
            configInput.saveFingerprint ? configInput.saveFingerprint : null,
          ),
        );
      },
      async "connect.credential"(rd, sd) {
        self.step.resolve(
          self.stepCredentialPrompt(rd, sd, config, (newCred, fromPreset) => {
            sessionData.credential = newCred;

            if (fromPreset && keptSessions.indexOf("credential") < 0) {
              keptSessions.push("credential");
            }
          }),
        );
      },
      "@stdout"(_rd) {},
      close() {},
      "@completed"() {
        self.step.resolve(
          self.stepErrorDone(
            "Operation has failed",
            "Connection has been cancelled",
          ),
        );
      },
    });
  }

  stepInitialPrompt() {
    let self = this;

    return command.prompt(
      "ET",
      "Eternal Terminal Host",
      "Connect",
      (r) => {
        self.hasStarted = true;

        let request;
        try {
          request = self.startBackendRequest(
            {
              user: r.user,
              authentication: r.authentication,
              host: r.host,
              charset: r.encoding,
              etServerPort: r["et server port"],
              etCommand: r["et command"],
              tabColor: self.preset ? self.preset.tabColor() : "",
              fingerprint: self.preset
                ? self.preset.metaDefault("Fingerprint", "")
                : "",
              presetID: self.preset ? self.preset.id() : "",
              saveFingerprint: self.saveFingerprint,
            },
            self.session,
          );
        } catch (e) {
          self.step.resolve(
            self.stepErrorDone(
              "Request failed",
              "Unable to start connection request: " + e,
            ),
          );
          return;
        }
        if (request === null) {
          return;
        }

        self.step.resolve(self.stepWaitForAcceptWait());
      },
      () => {},
      normalizeETPresetFields(
        command.fieldsWithPreset(
          initialFieldDef,
          [
            {
              name: "Host",
              suggestions(_input) {
                return [];
              },
            },
            { name: "User" },
            { name: "Authentication" },
            { name: "Encoding" },
            { name: "ET Server Port" },
            { name: "ET Command" },
          ],
          self.preset,
          () => {},
        ),
      ),
    );
  }

  startBackendRequest(configInput, sessionData) {
    return this.requestLifecycle.start(() => {
      return this.streams.request(COMMAND_ID, (sd) => {
        return this.buildCommand(sd, configInput, sessionData);
      });
    });
  }

  async stepFingerprintPrompt(
    rd,
    sd,
    verify,
    newFingerprint,
    saveFingerprint = null,
  ) {
    const self = this;

    let fingerprintData = new TextDecoder("utf-8").decode(
        await reader.readCompletely(rd),
      ),
      fingerprintChanged = false;

    switch (verify(fingerprintData)) {
      case FingerprintPromptVerifyPassed:
        sd.send(CLIENT_CONNECT_RESPOND_FINGERPRINT, new Uint8Array([0]));

        return self.stepContinueWaitForEstablishWait();

      case FingerprintPromptVerifyMismatch:
        fingerprintChanged = true;
    }

    const acceptFingerprint = () => {
      newFingerprint(fingerprintData);

      sd.send(CLIENT_CONNECT_RESPOND_FINGERPRINT, new Uint8Array([0]));

      self.step.resolve(self.stepContinueWaitForEstablishWait());
    };
    const actions = [];

    if (saveFingerprint !== null) {
      actions.push({
        text: "Save",
        async respond() {
          try {
            await saveFingerprint(fingerprintData);
          } catch (e) {
            throw new Error("Unable to save fingerprint: " + e, {
              cause: e,
            });
          }
          acceptFingerprint();
        },
      });
    }

    return command.prompt(
      !fingerprintChanged
        ? "Do you recognize this server?"
        : "Danger! Server fingerprint has changed!",
      !fingerprintChanged
        ? "Verify server fingerprint displayed below"
        : "It's very unusual. Please verify the new server fingerprint below",
      "Continue",
      () => acceptFingerprint(),
      () => {
        sd.send(CLIENT_CONNECT_RESPOND_FINGERPRINT, new Uint8Array([1]));

        self.step.resolve(
          command.wait("Rejecting", "Sending rejection to the backend"),
        );
      },
      command.fields(initialFieldDef, [
        {
          name: "Fingerprint",
          value: fingerprintData,
        },
      ]),
      actions,
    );
  }

  async stepCredentialPrompt(rd, sd, config, newCredential) {
    const self = this;

    let fields = [];

    if (config.credential.length > 0) {
      sd.send(
        CLIENT_CONNECT_RESPOND_CREDENTIAL,
        new TextEncoder().encode(config.credential),
      );

      return self.stepContinueWaitForEstablishWait();
    }

    switch (config.auth) {
      case AUTHMETHOD_PRIVATE_KEY:
        fields = [{ name: "Private Key" }];
        break;

      default:
        throw new Exception(
          'Auth method "' + config.auth + '" was unsupported',
        );
    }

    let presetCredentialUsed = false;
    const inputFields = command.fieldsWithPreset(
      initialFieldDef,
      fields,
      self.preset,
      (r) => {
        if (r !== fields[0].name) {
          return;
        }

        presetCredentialUsed = true;
      },
    );

    return command.prompt(
      "Provide credential",
      "Please input your credential",
      "Login",
      (r) => {
        let vv = r[fields[0].name.toLowerCase()];

        sd.send(
          CLIENT_CONNECT_RESPOND_CREDENTIAL,
          new TextEncoder().encode(vv),
        );

        newCredential(vv, presetCredentialUsed);

        self.step.resolve(self.stepContinueWaitForEstablishWait());
      },
      () => {
        sd.close();

        self.step.resolve(
          command.wait(
            "Cancelling login",
            "Cancelling login request, please wait",
          ),
        );
      },
      inputFields,
    );
  }
}

class Executer extends Wizard {
  /**
   * constructor
   *
   * @param {command.Info} info
   * @param {object} config
   * @param {object} session
   * @param {Array<string>} keptSessions
   * @param {streams.Streams} streams
   * @param {subscribe.Subscribe} subs
   * @param {controls.Controls} controls
   * @param {object|null} history Deprecated connection history placeholder.
   *
   */
  constructor(
    info,
    config,
    session,
    keptSessions,
    streams,
    subs,
    controls,
    history,
  ) {
    super(
      info,
      presets.emptyPreset(),
      session,
      keptSessions,
      streams,
      subs,
      controls,
      history,
    );

    this.config = config;
  }

  stepInitialPrompt() {
    const self = this;

    self.hasStarted = true;

    let request;
    try {
      request = self.startBackendRequest(
        {
          user: self.config.user,
          authentication: self.config.authentication,
          host: self.config.host,
          charset: self.config.charset ? self.config.charset : "utf-8",
          etServerPort: self.config.etServerPort,
          etCommand: self.config.etCommand,
          tabColor: self.config.tabColor ? self.config.tabColor : "",
          fingerprint: self.config.fingerprint,
          presetID: self.config.presetID ? self.config.presetID : "",
          saveFingerprint: self.config.saveFingerprint
            ? self.config.saveFingerprint
            : null,
        },
        self.session,
      );
    } catch (e) {
      return self.stepErrorDone(
        "Request failed",
        "Unable to start connection request: " + e,
      );
    }
    if (request === null) {
      return self.stepErrorDone(
        "Request failed",
        "Unable to start connection request",
      );
    }

    return self.stepWaitForAcceptWait();
  }
}

export class Command {
  constructor() {}

  id() {
    return COMMAND_ID;
  }

  name() {
    return "ET";
  }

  description() {
    return "Eternal Terminal";
  }

  color() {
    return "#96a";
  }

  wizard(
    info,
    preset,
    session,
    keptSessions,
    streams,
    subs,
    controls,
    history,
    saveFingerprint = null,
  ) {
    return new Wizard(
      info,
      preset,
      session,
      keptSessions,
      streams,
      subs,
      controls,
      history,
      saveFingerprint,
    );
  }

  execute(
    info,
    config,
    session,
    keptSessions,
    streams,
    subs,
    controls,
    history,
  ) {
    return new Executer(
      info,
      config,
      session,
      keptSessions,
      streams,
      subs,
      controls,
      history,
    );
  }

  launch(info, launcher, streams, subs, controls, history) {
    const d = launcher.split("|", 5);

    if (d.length < 2 || d.length > 5) {
      throw new Exception('Given launcher "' + launcher + '" was invalid');
    }

    const userHostName = d[0].match(new RegExp("^(.*)\\@(.*)$"));

    if (!userHostName || userHostName.length !== 3) {
      throw new Exception('Given launcher "' + launcher + '" was malformed');
    }

    let user = userHostName[1],
      host = userHostName[2],
      auth = d[1],
      charset = d.length >= 3 ? d[2] : "utf-8",
      etServerPort = DEFAULT_ET_SERVER_PORT,
      etCommand = DEFAULT_ET_COMMAND;

    if (d.length >= 4) {
      if (/^[0-9]+$/.test(d[3])) {
        etServerPort = d[3];
        if (d.length >= 5) {
          etCommand = decodeLauncherETCommand(d[4]);
        }
      } else {
        etCommand = decodeLauncherETCommand(d[3]);
      }
    }

    try {
      initialFieldDef["User"].verify(user);
      initialFieldDef["Host"].verify(host);
      initialFieldDef["Authentication"].verify(auth);
      initialFieldDef["Encoding"].verify(charset);
      initialFieldDef["ET Server Port"].verify(etServerPort);
      initialFieldDef["ET Command"].verify(etCommand);
    } catch (e) {
      throw new Exception(
        'Given launcher "' + launcher + '" was malformed ' + e,
      );
    }

    return this.execute(
      info,
      {
        user: user,
        host: host,
        authentication: auth,
        charset: charset,
        etServerPort: etServerPort,
        etCommand: etCommand,
      },
      null,
      null,
      streams,
      subs,
      controls,
      history,
    );
  }

  launcher(config) {
    const launcher =
      config.user +
      "@" +
      config.host +
      "|" +
      config.authentication +
      "|" +
      (config.charset ? config.charset : "utf-8");
    const etServerPort = config.etServerPort || DEFAULT_ET_SERVER_PORT;
    const etCommand = config.etCommand || DEFAULT_ET_COMMAND;

    if (
      etServerPort === DEFAULT_ET_SERVER_PORT &&
      etCommand === DEFAULT_ET_COMMAND
    ) {
      return launcher;
    }

    const launcherWithPort = launcher + "|" + etServerPort;
    if (etCommand === DEFAULT_ET_COMMAND) {
      return launcherWithPort;
    }

    return launcherWithPort + "|" + encodeURIComponent(etCommand);
  }

  represet(preset) {
    const host = preset.host();

    if (host.length > 0) {
      preset.insertMeta("Host", host);
    }

    return preset;
  }
}
