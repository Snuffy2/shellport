<!--
Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
Copyright (C) 2026 Snuffy2
SPDX-License-Identifier: AGPL-3.0-only
-->

<template>
  <div
    :id="id"
    class="window window1"
    :class="[{ display: displaying }, { [flashClass]: displaying }]"
  >
    <div class="window-frame">
      <slot />
    </div>
  </div>
</template>

<script>
/**
 * @fileoverview Generic floating overlay window widget. Renders a `.window1`
 * div that toggles the `display` and `flashClass` CSS classes based on an
 * internal `displaying` boolean. Exposes `show()` / `hide()` methods driven by
 * the `display` prop watcher.
 *
 * All overlay panels (connect, status, tab-window) wrap their content in this
 * component to get consistent show/hide behaviour.
 *
 * @prop {string}  id         - HTML id applied to the root element.
 * @prop {boolean} display    - External signal to show (true) or hide (false) the window.
 * @prop {string}  flashClass - CSS class added alongside `.display` when the window is shown,
 *   used to trigger a flash/entry animation.
 *
 * @emits display - Emitted whenever the visibility state changes.
 *   Payload: `{boolean}` — true when shown, false when hidden.
 */

export default {
  props: {
    id: {
      type: String,
      default: "",
    },
    display: {
      type: Boolean,
      default: false,
    },
    flashClass: {
      type: String,
      default: "",
    },
  },
  emits: ["display"],
  /**
   * @returns {{displaying: boolean}}
   *   `displaying` — internal visibility flag; true while the overlay is shown.
   *   Driven by `show()` / `hide()` and mirrored to the template via CSS classes.
   */
  data() {
    return {
      displaying: false,
    };
  },
  watch: {
    display(newVal) {
      newVal ? this.show() : this.hide();
    },
  },
  methods: {
    /**
     * Makes the overlay visible and emits `display` with `true`.
     *
     * @emits display - Payload: `{true}`.
     * @returns {void}
     */
    show() {
      this.displaying = true;

      this.$emit("display", this.displaying);
    },
    /**
     * Hides the overlay and emits `display` with `false`.
     *
     * @emits display - Payload: `{false}`.
     * @returns {void}
     */
    hide() {
      this.displaying = false;

      this.$emit("display", this.displaying);
    },
  },
};
</script>
