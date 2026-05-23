// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { prepareDevStaticAssets } from "./dev-static-assets.mjs";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const devConfigTemplate = path.join(
  repoRoot,
  "scripts",
  "shellport.dev.conf.yml",
);
const devConfig = path.join(repoRoot, ".tmp", "dev", "shellport.conf.yml");

let shuttingDown = false;
let goProcess = null;
let viteServer = null;
let cleanupDevStaticAssets = () => {};

/**
 * Prefix a child process's stdout and stderr before forwarding them.
 *
 * @param {import("node:child_process").ChildProcess} child Child process.
 * @param {string} label Output label to add before each chunk.
 */
function forwardOutput(child, label) {
  child.stdout.on(
    "data",
    /**
     * Forward a stdout chunk from the child process.
     *
     * @param {Buffer | string} data Output chunk.
     */
    (data) => {
      process.stdout.write(`[${label}] ${data}`);
    },
  );
  child.stderr.on(
    "data",
    /**
     * Forward a stderr chunk from the child process.
     *
     * @param {Buffer | string} data Error output chunk.
     */
    (data) => {
      process.stderr.write(`[${label}] ${data}`);
    },
  );
}

/**
 * Prepare a writable runtime config for local development.
 */
function prepareDevConfig() {
  if (fs.existsSync(devConfig)) {
    return;
  }
  fs.mkdirSync(path.dirname(devConfig), { recursive: true });
  fs.copyFileSync(devConfigTemplate, devConfig);
}

/**
 * Start the Go backend with the local development configuration.
 */
function startGo() {
  const child = spawn("go", ["run", "shellport.go"], {
    cwd: repoRoot,
    detached: process.platform !== "win32",
    env: {
      ...process.env,
      CGO_ENABLED: "0",
      SHELLPORT_CONFIG: devConfig,
      SHELLPORT_DEBUG: "_",
    },
    stdio: ["ignore", "pipe", "pipe"],
  });

  forwardOutput(child, "go");

  child.on(
    "error",
    /**
     * Shut down the supervisor if the Go backend cannot start.
     *
     * @param {Error} error Spawn error.
     */
    (error) => {
      process.stderr.write(
        `[dev] failed to start Go backend: ${error.message}\n`,
      );
      void shutdown(1);
    },
  );

  child.on(
    "exit",
    /**
     * Stop the dev stack if the backend exits before supervisor shutdown.
     *
     * @param {number | null} code Process exit code.
     * @param {NodeJS.Signals | null} signal Signal that ended the process.
     */
    (code, signal) => {
      goProcess = null;
      if (shuttingDown) {
        return;
      }
      process.stderr.write(
        `[dev] Go backend exited unexpectedly with code ${code} signal ${signal}\n`,
      );
      void shutdown(code === 0 || code === null ? 1 : code);
    },
  );

  goProcess = child;
}

/**
 * Start the Vite development server with the repository config.
 *
 * @returns {Promise<void>} Resolves after Vite is listening.
 */
async function startVite() {
  viteServer = await createServer({
    configFile: path.join(repoRoot, "vite.config.js"),
    root: repoRoot,
  });
  await viteServer.listen();
  viteServer.printUrls();
}

/**
 * Stop the Go backend process group, escalating if it does not exit quickly.
 *
 * @returns {Promise<void>} Resolves after the backend has exited.
 */
async function stopGo() {
  if (!goProcess) {
    return;
  }

  const child = goProcess;
  goProcess = null;

  await new Promise(
    /**
     * Send shutdown signals and resolve when the backend process exits.
     *
     * @param {(...args: unknown[]) => void} resolve Promise resolver.
     */
    (resolve) => {
      let escalationTimer = null;
      /**
       * Report whether the child process has already terminated.
       *
       * @returns {boolean} True once exit code or signal is populated.
       */
      const hasExited = () =>
        child.exitCode !== null || child.signalCode !== null;
      /**
       * Send a process signal if the child process is still running.
       *
       * @param {NodeJS.Signals} signal Signal to send.
       */
      const sendSignal = (signal) => {
        if (hasExited()) {
          return;
        }
        try {
          if (process.platform === "win32") {
            child.kill(signal);
          } else {
            process.kill(-child.pid, signal);
          }
        } catch (error) {
          if (error.code !== "ESRCH") {
            throw error;
          }
        }
      };

      if (hasExited()) {
        clearTimeout(escalationTimer);
        resolve(child.exitCode, child.signalCode);
        return;
      }

      child.once(
        "exit",
        /**
         * Clear pending escalation and resolve with the process exit details.
         *
         * @param {unknown[]} args Process exit event arguments.
         */
        (...args) => {
          clearTimeout(escalationTimer);
          resolve(...args);
        },
      );
      sendSignal("SIGINT");
      escalationTimer = setTimeout(
        /**
         * Escalate shutdown if the interrupt signal did not stop the backend.
         */
        () => {
          sendSignal("SIGTERM");
        },
        3000,
      );
      escalationTimer.unref();
    },
  );
}

/**
 * Stop the Vite development server if it is running.
 *
 * @returns {Promise<void>} Resolves after the server closes.
 */
async function stopVite() {
  if (!viteServer) {
    return;
  }
  const server = viteServer;
  viteServer = null;
  await server.close();
}

/**
 * Stop all development processes and exit the supervisor.
 *
 * @param {number} exitCode Process exit code to use.
 * @returns {Promise<void>} Resolves only if process exit is intercepted.
 */
async function shutdown(exitCode) {
  if (shuttingDown) {
    return;
  }
  shuttingDown = true;
  await Promise.allSettled([stopVite(), stopGo()]);
  try {
    try {
      cleanupDevStaticAssets();
    } catch (error) {
      process.stderr.write(
        `[dev] failed to clean up static assets: ${
          error instanceof Error ? error.message : String(error)
        }\n`,
      );
    }
  } finally {
    cleanupDevStaticAssets = () => {};
    process.exit(exitCode);
  }
}

process.on(
  "SIGINT",
  /**
   * Stop the dev stack after an interactive interrupt.
   */
  () => {
    void shutdown(0);
  },
);

process.on(
  "SIGTERM",
  /**
   * Stop the dev stack after a termination request.
   */
  () => {
    void shutdown(0);
  },
);

try {
  cleanupDevStaticAssets = prepareDevStaticAssets(repoRoot);
  prepareDevConfig();
  startGo();
  await startVite();
} catch (error) {
  process.stderr.write(`[dev] ${error.message}\n`);
  await shutdown(1);
}
