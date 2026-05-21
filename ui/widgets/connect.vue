<!--
Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
Copyright (C) 2026 Snuffy2
SPDX-License-Identifier: AGPL-3.0-only
-->

<template>
  <window
    id="connect"
    flash-class="home-window-display"
    :display="display"
    @display="$emit('display', $event)"
  >
    <div id="connect-frame">
      <h1 class="window-title">Establish connection with</h1>

      <slot v-if="inputting"></slot>

      <connect-switch
        v-if="presetEditor === null && !inputting"
        :presets-length="presets.length"
        :tab="tab"
        @switch="switchTab"
      ></connect-switch>

      <preset-editor
        v-if="presetEditor !== null && !inputting"
        :mode="presetEditor.mode"
        :state="presetEditor.state"
        :policy="presetManagementPolicy"
        :admin-key-cached="adminKeyCached"
        :private-key-files="privateKeyFiles"
        :save-preset="presetSaveHandler"
        :delete-preset="presetDeleteHandler"
        @cancel="cancelPresetEditor"
      ></preset-editor>

      <connect-known
        v-if="tab === 'known' && presetEditor === null && !inputting"
        :presets="presets"
        :can-manage-presets="canManagePresets"
        :restricted-to-presets="restrictedToPresets"
        :refreshing-presets="refreshingPresets"
        @select-preset="selectPreset"
        @edit-preset="editPreset"
        @refresh-presets="refreshPresets"
      ></connect-known>

      <connect-new
        v-if="
          tab === 'new' &&
          presetEditor === null &&
          !inputting &&
          !restrictedToPresets
        "
        :connectors="connectors"
        @select="selectConnector"
      ></connect-new>

      <div v-if="busy" id="connect-busy-overlay"></div>
    </div>
  </window>
</template>

<script>
import "./connect.css";

/**
 * @fileoverview Root connection-establishment widget. Composes the new-remote
 * picker, the preset list, and the tab-switch control into a single
 * overlay window. Delegates connector and preset selection upward via emitted
 * events so the parent can drive the wizard flow.
 *
 * @prop {boolean}  display             - Controls overlay visibility.
 * @prop {boolean}  inputting           - When true, hides list panels and
 *   shows a slotted content (e.g. the wizard fieldset) instead.
 * @prop {Array}    presets             - Server-defined preset connections.
 * @prop {boolean}  restrictedToPresets - Hides "New remote" when true.
 * @prop {Array}    connectors          - Available connector types (SSH, Telnet…).
 * @prop {boolean}  busy                - When true, overlays the panel to block interaction.
 * @prop {boolean}  refreshingPresets   - When true, disables the preset refresh action.
 *
 * @emits display           - Forwarded from the window widget; payload: `{boolean}`.
 * @emits connector-select  - User chose a new connector type. Payload: connector object.
 * @emits preset-select     - User selected a preset. Payload: preset object.
 * @emits refresh-presets   - User requested backend preset reload.
 */

import Window from "./window.vue";
import ConnectSwitch from "./connect_switch.vue";
import ConnectKnown from "./connect_known.vue";
import ConnectNew from "./connect_new.vue";
import PresetEditor from "./preset_editor.vue";

export default {
  components: {
    window: Window,
    "connect-switch": ConnectSwitch,
    "connect-known": ConnectKnown,
    "connect-new": ConnectNew,
    "preset-editor": PresetEditor,
  },
  props: {
    display: {
      type: Boolean,
      default: false,
    },
    inputting: {
      type: Boolean,
      default: false,
    },
    presets: {
      type: Array,
      default: () => [],
    },
    restrictedToPresets: {
      type: Boolean,
      default: () => false,
    },
    connectors: {
      type: Array,
      default: () => [],
    },
    busy: {
      type: Boolean,
      default: false,
    },
    refreshingPresets: {
      type: Boolean,
      default: false,
    },
    canManagePresets: {
      type: Boolean,
      default: false,
    },
    presetEditor: {
      type: Object,
      default: () => null,
    },
    presetManagementPolicy: {
      type: Object,
      default: () => null,
    },
    presetSaveHandler: {
      type: Function,
      required: true,
    },
    presetDeleteHandler: {
      type: Function,
      required: true,
    },
    adminKeyCached: {
      type: Boolean,
      default: false,
    },
    privateKeyFiles: {
      type: Array,
      default: () => [],
    },
  },
  emits: [
    "display",
    "connector-select",
    "preset-select",
    "preset-edit",
    "preset-editor-cancel",
    "refresh-presets",
  ],
  /**
   * @returns {{tab: string, canSelect: boolean}}
   *   `tab` — active panel: `"known"` or `"new"`.
   *   `canSelect` — reserved flag for future debounce logic.
   */
  data() {
    return {
      tab: "known",
      canSelect: true,
    };
  },
  methods: {
    /**
     * Switches the active panel tab. No-op while the wizard is `inputting`.
     *
     * @param {string} to - Target tab name: `"new"` or `"known"`.
     * @returns {void}
     */
    switchTab(to) {
      if (this.inputting) {
        return;
      }

      if (to === "new" && this.restrictedToPresets) {
        return;
      }

      this.tab = to;
    },
    /**
     * Emits `connector-select` with the chosen connector. No-op while `inputting`.
     *
     * @param {Object} connector - The connector descriptor chosen by the user.
     * @emits connector-select
     * @returns {void}
     */
    selectConnector(connector) {
      if (this.inputting) {
        return;
      }

      this.$emit("connector-select", connector);
    },
    /**
     * Emits `preset-select` with the chosen preset. No-op while `inputting`.
     *
     * @param {Object} preset - The preset descriptor chosen by the user.
     * @emits preset-select
     * @returns {void}
     */
    selectPreset(preset) {
      if (this.inputting) {
        return;
      }

      this.$emit("preset-select", preset);
    },
    /**
     * Emits `refresh-presets` when the preset panel refresh button is clicked.
     *
     * @emits refresh-presets
     * @returns {void}
     */
    refreshPresets() {
      if (this.inputting) {
        return;
      }

      this.$emit("refresh-presets");
    },
    /**
     * Emits preset-edit event for the selected preset row.
     *
     * @param {Object} preset - Preset descriptor from known list.
     * @returns {void}
     */
    editPreset(preset) {
      if (this.inputting) {
        return;
      }

      this.$emit("preset-edit", preset);
    },
    /**
     * Emits preset editor close event.
     *
     * @returns {void}
     */
    cancelPresetEditor() {
      this.$emit("preset-editor-cancel");
    },
  },
};
</script>
