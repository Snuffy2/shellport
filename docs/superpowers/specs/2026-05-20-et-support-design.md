# Eternal Terminal Support Design

## Goal

Add Eternal Terminal (ET) as an additive ShellPort transport, similar in shape
to the recent Mosh support. The browser-to-ShellPort connection remains the
existing WebSocket framed stream. ET support changes only the
ShellPort-backend-to-remote-host leg.

## Approach

Implementation must first check for a maintained Go ET client library with a
usable session API. If one exists, use it and keep the backend transport shaped
like the existing Mosh session abstraction. If no such library exists, use the
ET CLI with ShellPort-owned authentication and trust handling.

The current design assumes the CLI fallback because the known ET project
packages an `et` client executable rather than a Go session library.

## User-Facing Scope

ET v1 supports:

- Private-key authentication only.
- Existing ShellPort host fingerprint confirmation and saving.
- A separate ET server port field, defaulting to `2022`.
- An optional local ET client command path, defaulting to `et`.
- UTF-8 terminal encoding unless implementation research proves ET requires a
  different handling.
- Mosh-like session lifetime: when the ShellPort/browser session closes, the
  backend ET client process is terminated.

ET v1 does not support:

- Password authentication.
- SOCKS5 proxying.
- ShellPort-side reattachment to an ET process after the browser session is
  closed.

## Architecture

Add ET as a fourth command alongside Telnet, SSH, and Mosh.

Backend changes:

- Register `ET` in `application/commands/commands.go`.
- Add ET command/session code under `application/commands/et*.go`.
- Reuse existing SSH private-key credential, preset credential, and fingerprint
  confirmation/storage primitives.
- For the CLI fallback, launch `et` under a PTY and bridge PTY input, output,
  resize, close, and process lifecycle through the command stream.

Frontend changes:

- Add `ui/commands/et.js` for wizard fields, validation, request encoding,
  credential/fingerprint responses, and stream markers.
- Add `ui/control/et.js` for terminal control. If ET is exposed as a PTY byte
  stream, this can follow the Mosh control shape closely.
- Register ET command and control in `ui/app.js`.
- Extend preset direct-launch logic so ET presets can skip the wizard when all
  required metadata is present.

Docker/runtime changes:

- If the CLI fallback is used, include runtime dependencies needed to run the
  ET client in the standard Docker image. The implementation must verify and
  document the exact package set; expected dependencies are the `et` client and
  OpenSSH client support.

## Wizard And Preset Fields

ET wizard/preset metadata should include:

- `Host`: SSH bootstrap target, with default SSH port `22`.
- `User`: SSH username.
- `Authentication`: fixed to or limited to `Private Key` for v1.
- `Private Key`: existing ShellPort private-key credential handling.
- `Fingerprint`: existing ShellPort host fingerprint confirmation and storage.
- `Encoding`: fixed to `utf-8` unless implementation research changes this.
- `ET Server Port`: numeric value, default `2022`.
- `ET Command`: optional local executable path, default `et`.

The example configuration should include an `Example ET` preset with private-key
authentication metadata and ET-specific defaults.

Preset and launcher/bookmark support must round-trip ET-specific fields. The
Mosh support previously exposed a failure mode where custom metadata was lost in
direct-launch paths; ET must include tests for this from the start.

## CLI Fallback Backend Flow

When using the `et` CLI fallback, the backend command should:

1. Reject SOCKS5 immediately with a dedicated ET unsupported-proxy error.
2. Parse user, SSH host, auth method, ET metadata, and optional preset ID from
   the initial request.
3. Accept only `Private Key`; return a clear bad-auth error for `Password` or
   unsupported methods.
4. Reuse ShellPort's SSH dial/auth/fingerprint flow to validate the remote host
   and cache accepted fingerprints before launching ET.
5. Create a per-session temporary directory with restrictive permissions.
6. Write temporary SSH material required by the ET process, such as an identity
   file, `known_hosts`, and SSH config.
7. Start `et` with argv-style process execution, never shell interpolation.
8. Run `et` under a PTY and bridge stdout/stderr to terminal output.
9. Bridge terminal input and resize events to the PTY.
10. On close or error, terminate the child process and delete the temporary
    directory.

## Error Handling

ET should expose user-facing errors instead of collapsing all failures into a
generic connection failure. Important cases include:

- Missing ET binary.
- Unsupported SOCKS5 proxy.
- Unsupported authentication method.
- Invalid ET server port.
- SSH fingerprint refused.
- SSH authentication failure.
- ET process exits before the session is ready.
- Temporary file creation or permission failures.

Secrets must not be logged. Temporary key material must be written with
restrictive permissions and cleaned up when the command exits.

## Testing And Validation

Add focused tests for:

- Backend ET metadata parsing and validation.
- Unsupported SOCKS5 and unsupported password-auth error mapping.
- Private-key credential and fingerprint flow reuse.
- Temporary file cleanup and child process termination on close.
- PTY bridge behavior using a fake executable or test helper rather than a live
  ET server.
- Frontend wizard validation and request encoding.
- Preset direct-launch metadata round-trip for `ET Server Port` and
  `ET Command`.

Validation should start with targeted backend and frontend tests for changed
files, then broaden based on risk. Expected full validation for the feature is:

- `go test ./... -race -timeout 30s`
- `npm test`
- `npm run build`
- Docker build if the Dockerfile changes
