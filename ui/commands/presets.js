// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * @file Preset configuration management for the ShellPort UI.
 *
 * Presets are pre-filled connection profiles loaded from server configuration.
 * {@link Preset} wraps a single profile and validates its structure;
 * {@link Presets} holds the full collection and provides filtered lookup.
 * Use {@link emptyPreset} to obtain a blank default preset when no server
 * configuration is present.
 */

import Exception from "./exception.js";

/**
 * Default preset item, contains data of a default preset
 *
 */
const presetItem = {
  id: "",
  title: "",
  type: "",
  host: "",
  tab_color: "",
  meta: {},
  has_saved_password: false,
  has_saved_private_key: false,
  private_key_file: "",
  private_key_filename: "",
};

/**
 * Verify that every value in `preset.meta` is a string.
 *
 * @private
 * @param {object} preset Preset object containing a `meta` dictionary.
 * @throws {Exception} When any meta value is not a string.
 */
function verifyPresetItemMeta(preset) {
  for (let i in preset.meta) {
    if (typeof preset.meta[i] === "string") {
      continue;
    }

    throw new Exception(
      'The data type of meta field "' +
        i +
        '" was "' +
        typeof preset.meta[i] +
        '" instead of expected "string"',
    );
  }
}

/**
 * Parse and validate a raw preset object against the `presetItem` template,
 * filling in defaults for missing optional fields.
 *
 * @private
 * @param {object} item Raw preset data from server configuration.
 * @returns {object} Validated preset object conforming to the `presetItem`
 *   shape.
 * @throws {Exception} When a field has the wrong type or meta values are
 *   non-string.
 */
function parsePresetItem(item) {
  let preset = {};

  for (let i in presetItem) {
    preset[i] = presetItem[i];
  }

  for (let i in presetItem) {
    if (typeof presetItem[i] === typeof item[i]) {
      preset[i] = item[i];
      continue;
    }

    if (typeof item[i] === "undefined" || !item[i]) {
      preset[i] = presetItem[i];
      continue;
    }

    throw new Exception(
      'Expecting the data type of "' +
        i +
        '" is "' +
        typeof presetItem[i] +
        '", received data of "' +
        typeof item[i] +
        '" type instead',
    );
  }

  verifyPresetItemMeta(preset.meta);

  return preset;
}

/**
 * A single connection preset loaded from server configuration.
 *
 * Provides typed accessors for preset fields and meta values, plus helpers
 * for inserting new meta entries and listing existing keys.
 */
export class Preset {
  /**
   * constructor
   *
   * @param {object} preset preset data
   *
   */
  constructor(preset) {
    this.preset = parsePresetItem(preset);
  }

  /**
   * Return the stable preset ID.
   *
   * @returns {string}
   */
  id() {
    return this.preset.id;
  }

  /**
   * Return the title of the preset
   *
   * @returns {string}
   *
   */
  title() {
    return this.preset.title;
  }

  /**
   * Return the type of the preset
   *
   * @returns {string}
   *
   */
  type() {
    return this.preset.type;
  }

  /**
   * Return the host of the preset
   *
   * @returns {string}
   *
   */
  host() {
    return this.preset.host;
  }

  /**
   * Return the tab color of the preset
   *
   * @returns {string}
   *
   */
  tabColor() {
    return this.preset.tab_color;
  }

  /**
   * Return the given meta of current preset
   *
   * @param {string} name name of the meta data
   *
   * @throws {Exception} when invalid data is given
   *
   * @returns {string}
   *
   */
  meta(name) {
    if (typeof this.preset.meta[name] !== "string") {
      throw new Exception('Meta "' + name + '" was undefined');
    }

    return this.preset.meta[name];
  }

  /**
   * Return the given meta of current preset, and if failed, return the given
   * default value
   *
   * @param {string} name name of the meta data
   * @param {string} defaultValue default value to be returned when the meta was
   *                              not found
   *
   * @returns {string}
   *
   */
  metaDefault(name, defaultValue) {
    try {
      return this.meta(name);
    } catch (e) {
      return defaultValue;
    }
  }

  /**
   * Return whether this preset has a hidden saved password.
   *
   * @returns {boolean}
   */
  hasSavedPassword() {
    return this.preset.has_saved_password;
  }

  /**
   * Return whether this preset has a hidden saved private key.
   *
   * @returns {boolean}
   */
  hasSavedPrivateKey() {
    return this.preset.has_saved_private_key;
  }

  /**
   * Return the saved private key file reference, if one is available.
   *
   * @returns {string}
   */
  privateKeyFile() {
    return this.preset.private_key_file;
  }

  /**
   * Return the saved private key filename, if one is available.
   *
   * @returns {string}
   */
  privateKeyFilename() {
    return this.preset.private_key_filename;
  }

  /**
   * Insert new meta item
   *
   * @param {string} name name of the meta data
   * @param {string} data data of the meta data
   *
   * @throws {Exception} when invalid data is given
   *
   */
  insertMeta(name, data) {
    if (typeof this.preset.meta[name] !== "undefined") {
      throw new Exception('Meta "' + name + '" has already been defined');
    }

    this.preset.meta[name] = data;
  }

  /**
   * Insert a meta item only when it is not already present.
   *
   * @param {string} name name of the meta data
   * @param {string} data data of the meta data
   *
   */
  insertMetaDefault(name, data) {
    if (typeof this.preset.meta[name] !== "undefined") {
      return;
    }

    this.preset.meta[name] = data;
  }

  /**
   * Export all meta keys
   *
   * @returns {Array<string>} All meta keys
   *
   */
  metaKeys() {
    let keys = [];

    for (let k in this.preset.meta) {
      keys.push(k);
    }

    return keys;
  }

  /**
   * Return this preset in backend config shape.
   *
   * @returns {object}
   */
  toConfig() {
    let meta = {};

    for (let key in this.preset.meta) {
      meta[key] = this.preset.meta[key];
    }

    return {
      id: this.preset.id,
      title: this.preset.title,
      type: this.preset.type,
      host: this.preset.host,
      tab_color: this.preset.tab_color,
      meta,
    };
  }
}

/**
 * Returns an empty preset
 *
 * @returns {Preset}
 *
 */
export function emptyPreset() {
  return new Preset({
    id: "",
    title: "Default",
    type: "Default",
    host: "",
    tab_color: "",
    meta: {},
  });
}

/**
 * Collection of all preset profiles, with filtered lookup by type, host, and
 * meta value.
 */
export class Presets {
  /**
   * constructor
   *
   * @param {Array<object>} presets Array of preset data
   *
   */
  constructor(presets) {
    this.presets = [];

    for (let i = 0; i < presets.length; i++) {
      this.presets.push(new Preset(presets[i]));
    }
  }

  /**
   * Return all presets of a type
   *
   * @param {string} type type of the presets data
   *
   * @returns {Array<Preset>}
   *
   */
  fetch(type) {
    let presets = [];

    for (let i = 0; i < this.presets.length; i++) {
      if (this.presets[i].type() !== type) {
        continue;
      }

      presets.push(this.presets[i]);
    }

    return presets;
  }

  /**
   * Return presets with matched type and meta data
   *
   * @param {string} type type of the presets data
   * @param {string} metaName name of the meta data
   * @param {string} metaVal value of the meta data
   *
   * @returns {Array<Preset>}
   *
   */
  meta(type, metaName, metaVal) {
    let presets = [];

    for (let i = 0; i < this.presets.length; i++) {
      if (this.presets[i].type() !== type) {
        continue;
      }

      try {
        if (this.presets[i].meta(metaName) !== metaVal) {
          continue;
        }
      } catch (e) {
        if (!(e instanceof Exception)) {
          throw e;
        }

        continue;
      }

      presets.push(this.presets[i]);
    }

    return presets;
  }

  /**
   * Return presets with matched type and host
   *
   * @param {string} type type of the presets
   * @param {string} host host of the presets
   *
   * @returns {Array<Preset>}
   *
   */
  hosts(type, host) {
    let presets = [];

    for (let i = 0; i < this.presets.length; i++) {
      if (this.presets[i].type() !== type) {
        continue;
      }

      if (this.presets[i].host() !== host) {
        continue;
      }

      presets.push(this.presets[i]);
    }

    return presets;
  }

  /**
   * Return all presets in backend config shape.
   *
   * @returns {Array<object>}
   */
  toConfig() {
    let configs = [];

    for (let i = 0; i < this.presets.length; i++) {
      configs.push(this.presets[i].toConfig());
    }

    return configs;
  }
}
