<!--
Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
Copyright (C) 2026 Snuffy2
SPDX-License-Identifier: AGPL-3.0-only
-->

<template>
  <div id="connect-known-list">
    <div v-if="presetCount <= 0" id="connect-known-list-empty">
      No presets available
    </div>
    <div v-else>
      <div id="connect-known-list-presets">
        <ul class="hlst lstcl2">
          <li
            v-for="(preset, pk) in presets"
            :key="pk"
            :class="{ disabled: presetDisabled(preset) }"
          >
            <div class="lst-wrap" @click="selectPreset(preset)">
              <div class="labels">
                <span
                  class="type"
                  :style="'background-color: ' + preset.command.color()"
                >
                  {{ preset.command.name() }}
                </span>
              </div>

              <h4 :title="preset.preset.title()">
                {{ preset.preset.title() }}
              </h4>

              <button
                v-if="canManagePresets"
                type="button"
                class="preset-edit-button icon icon-pencil"
                aria-label="Edit preset"
                title="Edit preset"
                @click.stop="editPreset(preset)"
              ></button>
            </div>
          </li>
        </ul>

        <div v-if="restrictedToPresets" id="connect-known-list-presets-alert">
          The operator has restricted the outgoing connections. You can only
          connect to remotes from the pre-defined presets.
        </div>
      </div>
    </div>
    <div id="connect-known-list-actions">
      <button
        type="button"
        :disabled="refreshingPresets"
        @click="refreshPresets"
      >
        {{ refreshingPresets ? "Refreshing presets..." : "Refresh presets" }}
      </button>
    </div>
  </div>
</template>

<script>
import "./connect_known.css";

/**
 * @fileoverview Lists server-defined presets in the connection picker.
 *
 * Preset entries can be disabled when `restrictedToPresets` is true and the
 * preset lacks a host.
 *
 * @prop {Array}    presets             - Server-defined preset connections.
 * @prop {boolean}  restrictedToPresets - When true, only fully-specified presets are selectable.
 * @prop {boolean}  refreshingPresets   - When true, disables the refresh action.
 *
 * @emits select-preset  - User chose a preset. Payload: preset object.
 * @emits refresh-presets - User requested backend preset reload.
 */

export default {
  props: {
    presets: {
      type: Array,
      default: () => [],
    },
    canManagePresets: {
      type: Boolean,
      default: false,
    },
    restrictedToPresets: {
      type: Boolean,
      default: () => false,
    },
    refreshingPresets: {
      type: Boolean,
      default: () => false,
    },
  },
  emits: ["select-preset", "edit-preset", "refresh-presets"],
  computed: {
    /**
     * Returns the number of renderable presets.
     *
     * @returns {number} Preset count, or zero when presets is not an array.
     */
    presetCount() {
      return Array.isArray(this.presets) ? this.presets.length : 0;
    },
  },
  methods: {
    /**
     * Returns whether a preset should be rendered as non-interactive.
     *
     * A preset is disabled when `restrictedToPresets` is true and the preset
     * does not specify a host (i.e. requires the user to fill in the address).
     *
     * @param {Object} preset - The preset descriptor.
     * @returns {boolean} True if the preset should be disabled.
     */
    presetDisabled(preset) {
      if (!this.restrictedToPresets || preset.preset.host().length > 0) {
        return false;
      }

      return true;
    },
    /**
     * Emits `select-preset` with the chosen preset.
     * No-op while busy or if the preset is disabled.
     *
     * @param {Object} preset - The preset descriptor chosen by the user.
     * @emits select-preset
     * @returns {void}
     */
    selectPreset(preset) {
      if (this.presetDisabled(preset)) {
        return;
      }

      this.$emit("select-preset", preset);
    },
    /**
     * Emits `refresh-presets` unless a refresh is already running.
     *
     * @emits refresh-presets
     * @returns {void}
     */
    refreshPresets() {
      if (this.refreshingPresets) {
        return;
      }

      this.$emit("refresh-presets");
    },
    /**
     * Emits `edit-preset` with the chosen preset for the editor flow.
     * No-op while preset list is not manageable.
     *
     * @param {Object} preset - The preset descriptor.
     * @emits edit-preset
     * @returns {void}
     */
    editPreset(preset) {
      this.$emit("edit-preset", preset);
    },
  },
};
</script>
