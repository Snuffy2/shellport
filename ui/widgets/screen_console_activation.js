// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Applies active-state changes to a console terminal.
 *
 * @param {{active: boolean, nextTick: function(): Promise<void>, isStillActive: function(): boolean, activate: function(): void, refresh: function(): void, deactivate: function(): void}} options
 *   Activation callbacks supplied by the Vue component.
 * @returns {Promise<void>} Resolves after the requested state transition runs.
 */
export async function triggerConsoleActive(options) {
  if (options.active) {
    await options.nextTick();
    if (!options.isStillActive()) {
      return;
    }
    options.activate();
    options.refresh();
    return;
  }

  options.deactivate();
}
