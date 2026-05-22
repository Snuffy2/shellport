// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, test, vi } from "vitest";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

function readProjectFile(relativePath) {
  return readFileSync(path.join(repoRoot, relativePath), "utf8");
}

function readSource() {
  return readProjectFile("ui/app.js");
}

function extractFunctionFromSource(name) {
  const source = readSource();
  const start = source.indexOf(`export async function ${name}(`);
  if (start < 0) {
    throw new Error(`Unable to locate function ${name} in source`);
  }

  const openParen = source.indexOf("(", start);
  if (openParen < 0) {
    throw new Error(`Unable to locate function signature for ${name}`);
  }

  let parenDepth = 1;
  let index = openParen + 1;
  for (; index < source.length; index++) {
    const char = source[index];

    if (char === "(") {
      parenDepth += 1;
      continue;
    }

    if (char === ")") {
      parenDepth -= 1;
      if (parenDepth === 0) {
        break;
      }
    }
  }

  if (index >= source.length || parenDepth !== 0) {
    throw new Error(`Unable to parse function signature for ${name}`);
  }

  const functionStart = source.indexOf("{", index);
  if (functionStart < 0) {
    throw new Error(`Unable to locate function body for ${name}`);
  }

  index = functionStart;
  let depth = 0;
  for (; index < source.length; index++) {
    const char = source[index];

    if (char === "{") {
      depth += 1;
      continue;
    }

    if (char === "}") {
      depth -= 1;
      if (depth === 0) {
        break;
      }
    }
  }

  if (index >= source.length || depth !== 0) {
    throw new Error(`Unable to parse function body for ${name}`);
  }

  const functionSource =
    `export async function ${name}` +
    source.slice(start + `export async function ${name}`.length, index + 1);

  const exported = Function(
    `${functionSource.replace("export async function", "async function")}\nreturn ${name};`,
  )();

  return exported;
}

describe("app preset management wiring", () => {
  test("passes policy and write callbacks to home", () => {
    const source = readSource();

    expect(source).toContain(
      ':preset-management-policy="presetData.management"',
    );
    expect(source).toContain(':save-preset-config="savePresetConfig"');
    expect(source).toContain(
      ':admin-password-required="presetAdminPasswordRequired"',
    );
  });

  test("keeps admin password in page memory only", () => {
    const source = readSource();

    expect(source).toContain('presetAdminPassphrase: ""');
    expect(source).toContain(
      "this.presetAdminPassphrase = result.adminPassword;",
    );
    expect(source).not.toContain("localStorage");
    expect(source).not.toContain("sessionStorage");
  });

  test("full preset writes preserve hidden passwords and send clear IDs", () => {
    const source = readSource();

    expect(source).toContain(
      'headers["X-Preserve-Hidden-Preset-Passwords"] = "yes";',
    );
    expect(source).toContain('headers["X-Clear-Hidden-Preset-Passwords"]');
    expect(source).toContain("JSON.stringify(clearPasswordIDs)");
  });
});

describe("savePresetConfigRequest", () => {
  const savePresetConfigRequest = extractFunctionFromSource(
    "savePresetConfigRequest",
  );

  test("rejects full-list save when management is disabled", async () => {
    const headerBuilder = vi.fn(async () => ({}));
    const xhrPut = vi.fn();

    await expect(
      savePresetConfigRequest({
        updatedPresets: [],
        options: {},
        presetData: {
          management: {
            can_manage: false,
          },
        },
        presetConfigPassphrase: "shared-key",
        presetAdminPassphrase: "",
        presetConfigHeadersForPassphrase: headerBuilder,
        xhrPut,
        presetConfigInterface: "/shellport/config/presets",
      }),
    ).rejects.toThrow("Preset management is not allowed");

    expect(headerBuilder).not.toHaveBeenCalled();
    expect(xhrPut).not.toHaveBeenCalled();
  });

  test("saves full preset list with preserve/clear headers and returns presets", async () => {
    const headerBuilder = vi.fn(async (passphrase) => {
      expect(passphrase).toBe("admin-pass");
      return {
        "Content-Type": "application/json",
      };
    });
    const xhrPut = vi.fn().mockResolvedValueOnce({
      status: 200,
      responseText: JSON.stringify({ presets: [{ id: "preset-1" }] }),
    });

    const result = await savePresetConfigRequest({
      updatedPresets: [{ id: "preset-1" }],
      options: {
        adminPassword: "admin-pass",
        clearPasswordIDs: ["preset-1", "preset,2"],
        clearPrivateKeyIDs: ["preset-3"],
      },
      presetData: {
        management: {
          can_manage: true,
          requires_admin_password: true,
        },
      },
      presetConfigPassphrase: "shared-key",
      presetAdminPassphrase: "cached-admin",
      presetConfigHeadersForPassphrase: headerBuilder,
      xhrPut,
      presetConfigInterface: "/shellport/config/presets",
    });

    expect(xhrPut).toHaveBeenCalledTimes(1);
    expect(xhrPut).toHaveBeenCalledWith(
      "/shellport/config/presets",
      expect.objectContaining({
        "Content-Type": "application/json",
        "X-Preserve-Hidden-Preset-Passwords": "yes",
        "X-Clear-Hidden-Preset-Passwords": '["preset-1","preset,2"]',
        "X-Clear-Hidden-Preset-Private-Keys": '["preset-3"]',
      }),
      JSON.stringify({ presets: [{ id: "preset-1" }] }),
    );
    expect(result).toEqual({
      adminPassword: "admin-pass",
      privateKeyFiles: [],
      presets: [{ id: "preset-1" }],
    });
  });

  test("uses cached admin password when payload admin password is blank", async () => {
    const headerBuilder = vi.fn(async (passphrase) => {
      expect(passphrase).toBe("cached-admin");
      return {
        "Content-Type": "application/json",
      };
    });
    const xhrPut = vi.fn().mockResolvedValueOnce({
      status: 200,
      responseText: JSON.stringify({ presets: [{ id: "preset-1" }] }),
    });

    const result = await savePresetConfigRequest({
      updatedPresets: [{ id: "preset-1" }],
      options: {
        adminPassword: "",
      },
      presetData: {
        management: {
          can_manage: true,
          requires_admin_password: true,
        },
      },
      presetConfigPassphrase: "shared-key",
      presetAdminPassphrase: "cached-admin",
      presetConfigHeadersForPassphrase: headerBuilder,
      xhrPut,
      presetConfigInterface: "/shellport/config/presets",
    });

    expect(xhrPut).toHaveBeenCalledTimes(1);
    expect(result).toEqual({
      adminPassword: null,
      privateKeyFiles: [],
      presets: [{ id: "preset-1" }],
    });
  });

  test("uses user password instead of cached admin password when admin password is not required", async () => {
    const headerBuilder = vi.fn(async (passphrase) => {
      expect(passphrase).toBe("shared-key");
      return {
        "Content-Type": "application/json",
      };
    });
    const xhrPut = vi.fn().mockResolvedValueOnce({
      status: 200,
      responseText: JSON.stringify({ presets: [{ id: "preset-1" }] }),
    });

    await savePresetConfigRequest({
      updatedPresets: [{ id: "preset-1" }],
      options: {},
      presetData: {
        management: {
          can_manage: true,
          requires_admin_password: false,
        },
      },
      presetConfigPassphrase: "shared-key",
      presetAdminPassphrase: "stale-admin",
      presetConfigHeadersForPassphrase: headerBuilder,
      xhrPut,
      presetConfigInterface: "/shellport/config/presets",
    });

    expect(headerBuilder).toHaveBeenCalledTimes(1);
  });

  test("throws when full-list save returns non-200", async () => {
    const headerBuilder = vi.fn(async () => ({}));
    const xhrPut = vi.fn().mockResolvedValueOnce({
      status: 500,
      responseText: JSON.stringify({ presets: [] }),
    });

    await expect(
      savePresetConfigRequest({
        updatedPresets: [],
        presetData: {
          management: {
            can_manage: true,
          },
        },
        presetConfigPassphrase: "shared-key",
        presetAdminPassphrase: "",
        presetConfigHeadersForPassphrase: headerBuilder,
        xhrPut,
        presetConfigInterface: "/shellport/config/presets",
      }),
    ).rejects.toThrow("Preset config write failed: 500");
  });

  test("includes sanitized response text in full-list save errors", async () => {
    const headerBuilder = vi.fn(async () => ({}));
    const xhrPut = vi.fn().mockResolvedValueOnce({
      status: 400,
      responseText: "invalid preset\nmetadata",
    });

    await expect(
      savePresetConfigRequest({
        updatedPresets: [],
        presetData: {
          management: {
            can_manage: true,
          },
        },
        presetConfigPassphrase: "shared-key",
        presetAdminPassphrase: "",
        presetConfigHeadersForPassphrase: headerBuilder,
        xhrPut,
        presetConfigInterface: "/shellport/config/presets",
      }),
    ).rejects.toThrow(
      "Preset config write failed: 400: invalid preset metadata",
    );
  });

  test("attaches response status to full-list save errors", async () => {
    const headerBuilder = vi.fn(async () => ({}));
    const xhrPut = vi.fn().mockResolvedValueOnce({
      status: 403,
      responseText: JSON.stringify({ presets: [] }),
    });

    await expect(
      savePresetConfigRequest({
        updatedPresets: [],
        presetData: {
          management: {
            can_manage: true,
            requires_admin_password: true,
          },
        },
        presetConfigPassphrase: "shared-key",
        presetAdminPassphrase: "cached-admin",
        presetConfigHeadersForPassphrase: headerBuilder,
        xhrPut,
        presetConfigInterface: "/shellport/config/presets",
      }),
    ).rejects.toMatchObject({ status: 403 });
  });
});
