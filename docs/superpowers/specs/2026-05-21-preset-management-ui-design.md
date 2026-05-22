# Preset Management UI Design

Date: 2026-05-21

## Goal

Finish ShellPort's UI support for managing presets stored in the current
writable config file. Users should be able to create, edit, and delete presets
from the browser UI when policy allows it, while preserving the existing preset
refresh behavior and the existing connection-time fingerprint save flow.

## Current Context

ShellPort already exposes `GET /shellport/config/presets` and
`PUT /shellport/config/presets`. The `PUT` endpoint supports full preset-list
replacement for admin users, preserves hidden preset passwords when requested,
assigns missing preset IDs, normalizes metadata, applies preset secret handling,
persists to the file-backed config, and updates the live preset repository.

The current UI can refresh presets and save a connection fingerprint for an
existing preset. It does not yet expose full create, edit, or delete controls.
The current auth UI accepts `SharedKey`; `AdminKey` exists on the backend for
admin preset writes but has no dedicated UI prompt.

## Permission Model

Preset management is unavailable when `OnlyAllowPresetRemotes` is true. In that
mode the UI must not offer edit, create, delete, or save-as controls.

When `OnlyAllowPresetRemotes` is false and `AdminKey` is blank, anyone with UI
access can create, edit, and delete presets.

When `OnlyAllowPresetRemotes` is false and `AdminKey` is configured, the UI
shows create, edit, and delete affordances, but the first protected write opens
an AdminKey prompt. A successful AdminKey is cached only in browser memory for
the current page session. Future preset create, edit, or delete actions in that
session reuse it. A page reload clears it.

The `Refresh presets` button remains available to anyone who can access the UI.
Connection-time fingerprint saving remains governed by the existing
fingerprint-save path and is not moved into the preset editor.

## Backend Contract

Extend the existing socket verification response with preset management policy
metadata so the frontend can render the correct controls without guessing. The
metadata should distinguish:

- Preset config is writable or not writable.
- Preset management is blocked by `OnlyAllowPresetRemotes`.
- Preset management can write immediately.
- Preset management requires an AdminKey prompt.

Also expose whether each preset has a hidden saved password so the editor can
default "Keep saved password" correctly without sending the password value to
the browser.

The existing `PUT /shellport/config/presets` full-list replacement remains the
write mechanism for create, edit, and delete. The frontend sends a complete
replacement list after applying the requested change to its current preset list.

The backend remains authoritative. UI hiding is only convenience; backend tests
must continue to verify that non-admin full-list writes are rejected when an
AdminKey is configured.

## Preset List UI

The Known Presets list gets an icon-only edit control aligned to the right of
each preset name when preset management is allowed or possible through an
AdminKey prompt. The icon must have an accessible label and hover title such as
`Edit preset`.

Clicking the preset row still starts a connection. Clicking the edit icon opens
the preset editor and must not start a connection.

The refresh button stays in the Known Presets panel and keeps its current
permission behavior.

## New Remote Save-As Flow

When the user creates a New remote and preset management is allowed or possible,
the flow offers a way to save that remote as a preset. The save-as path should
reuse the same preset editor used for existing presets, prefilled from the
connection details already entered by the user.

When `OnlyAllowPresetRemotes` is true, New remote is already hidden and no
save-as path is shown.

## Preset Editor

Use one compact editor for creating and updating presets. It opens inside the
current connection overlay rather than introducing a separate management page.

The editor supports:

- Preset title.
- Preset type.
- Host.
- Tab color where supported by existing preset data.
- Type-specific metadata for SSH, Mosh, ET, and Telnet.
- Save or keep password.
- Save or keep private key.

The implementation should first support changing the preset type between SSH,
Mosh, ET, and Telnet. If that proves too coupled to the existing command
metadata, the accepted fallback is to keep the type fixed for existing presets
and allow choosing type only during create. The editor must still support
changing options for the current type.

Fingerprint is intentionally absent from this editor. Users can save the
fingerprint only from the connection-time fingerprint prompt.

## Secret Handling

For new presets, password saving defaults off. Private key saving defaults on
when a private key value is present. Users can change both choices before
saving.

For existing presets, if a password is already saved but hidden from the client,
the editor defaults to keeping the saved password. If the user clears that
choice, the next save removes the saved password from the preset. If the user
enters a replacement password and chooses to save it, the replacement is sent in
the full-list update and the backend handles encryption or plaintext persistence
according to the existing preset secret configuration.

For private keys, the editor defaults to saving or keeping the key when a value
is present. Users can clear that choice to omit the private key from the saved
preset.

Hidden secrets must not be displayed back to the browser. The UI should model
hidden saved password state as a boolean such as "saved password exists" rather
than as a value.

## AdminKey Prompt

When an AdminKey is required and no valid key is cached in memory, clicking
Save, Delete, or Save as preset opens a small prompt asking for AdminKey.

The prompt is for ShellPort administration, not for the remote host's password
or private key. A successful protected write caches the AdminKey in memory for
the current page session. A failed write caused by wrong AdminKey leaves the
prompt open, shows an error, and does not mutate local preset state.

## Delete Flow

The edit window includes a delete action for existing presets. Clicking delete
does not immediately write. The user must confirm deletion first. After
confirmation, the UI applies the removal to the current preset list and sends a
full-list `PUT`.

If AdminKey is required and not cached, the AdminKey prompt appears as part of
the confirmed delete action.

## Error Handling

Refresh failures should keep the current preset list and show the existing
alert-style error.

Save and delete failures should keep the editor open, keep user-entered values,
and show an actionable error. For stale-list or validation errors, users can
refresh presets and try again.

Wrong AdminKey errors should be shown in the AdminKey prompt rather than as a
generic browser alert.

## Testing

Backend tests should cover the new policy metadata in the verification response
for these cases:

- `OnlyAllowPresetRemotes` true.
- Writable file config with blank AdminKey.
- Writable file config with AdminKey.
- Non-writable config.

Existing backend tests for admin-only full-list writes, SharedKey-as-admin when
AdminKey is blank, anonymous-admin when both keys are blank, and anonymous-user
when only AdminKey is set should remain in place.

Frontend tests should cover:

- Edit icons are shown only when management is allowed or AdminKey-gated.
- Edit icons are hidden when `OnlyAllowPresetRemotes` blocks management.
- Refresh remains available independent of management permission.
- AdminKey prompt appears on the first protected save/delete/create and is
  skipped after a successful write in the same browser session.
- Wrong AdminKey leaves the editor/prompt open and does not update local presets.
- Existing hidden saved password defaults to keep checked.
- New password saving defaults unchecked.
- Private key saving defaults checked when a key is present.
- Fingerprint controls are absent from the editor.
- Delete requires confirmation before the write.
- Full-list save payloads preserve unaffected presets.

## Out of Scope

This design does not add a separate preset management page, bulk editing,
import/export, audit logs, or changes to the connection-time fingerprint save
permission model.
