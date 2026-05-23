# ShellPort Configuration

ShellPort reads its runtime settings from a `.json` file. The file may use JSONC-style `//` comments, `/* ... */` comments, and trailing commas. In the Docker image, the default path is `/config/shellport.conf.json`.

If that file does not exist, ShellPort creates a minimal writable config and then loads it. The generated file listens on `0.0.0.0:8182`, starts with no presets, and leaves `UserPassword` and `AdminPassword` blank so you can reach the UI on first boot and add presets before tightening access.

You can point ShellPort at a different file with `SHELLPORT_CONFIG`.

The checked-in `shellport.conf.example.json` is an annotated reference file and is loadable by ShellPort.

## Config File Location

The default file path is convenient for Docker volume mounts, but it is not required. Any writable `.json` config file can be used as long as ShellPort can read it at startup.

The runtime environment variables are:

- `TZ`, which sets the timezone used for timestamps in logs.
- `SHELLPORT_CONFIG`, which overrides the config file path.
- `SHELLPORT_DEBUG`, which enables debug logging when it is set to any non-empty value.
- `SHELLPORT_PRESET_SECRET_KEY`, which enables preset-password encryption and decryption.

`SHELLPORT_PRESET_SECRET_KEY` must be provided as a base64-encoded 32-byte key. Set it in the environment, not in the JSON file. When present, ShellPort migrates plaintext preset passwords to encrypted form on startup and keeps the plaintext value out of the config file.

## Preset Management

Preset updates require a writable file-backed configuration. ShellPort can read presets from an in-memory or environment-loaded config, but UI writes only persist when the active config comes from disk and the file is writable.

The UI can create, edit, and delete presets when the current access policy allows it. `AdminPassword` controls protected preset writes when you want separate user and admin access. If `AdminPassword` is blank and `UserPassword` is set, authenticated users get admin-level preset management. If both passwords are blank, anyone who can reach the UI can manage presets.

When `OnlyAllowPresetRemotes` is enabled, ShellPort only allows connections to hosts that are already present in the preset list, and preset management is disabled.

## Top-Level Settings

`HostName` restricts accepted HTTP `Host` headers. Leave it blank to accept requests for any host name. Set it when ShellPort should only answer for one public name, such as `shellport.example.com`.

`UserPassword` controls access to the web UI. Leave it blank to allow public UI access.

`AdminPassword` protects full preset updates when you want administrative control separate from user access.

`DialTimeout` limits how long ShellPort waits when opening an outbound connection. If it is omitted, ShellPort uses a 5 second default in the file loader path.

`Socks5` enables outbound SOCKS5 proxying. Use `Socks5User` and `Socks5Password` when the proxy needs credentials.

`Hooks` lets you run server-side commands during connection lifecycle events. The `before_connecting` hook runs before ShellPort dials the remote host. A non-zero exit status aborts the connection. Hook inputs are not sanitized by ShellPort, so only use trusted commands and validate the values inside your hook scripts.

`HookTimeout` limits how long each hook may run before ShellPort terminates it.

`Servers` defines one or more HTTP or HTTPS listeners.

`Presets` defines the preconfigured remotes shown in the UI.

`OnlyAllowPresetRemotes` restricts outbound connections to preset hosts only.

## Server Settings

Each entry in `Servers` defines one listener.

`ListenInterface` is the local interface to bind. The generated example file uses `0.0.0.0` so the container listens on all interfaces. If you leave it empty in a normalized configuration, ShellPort falls back to `127.0.0.1`.

`ListenPort` is the port to bind. If it is omitted, ShellPort uses `80` for plain HTTP and `443` when TLS is enabled.

`InitialTimeout` is the time allowed for the initial HTTP request and handshake.

`ReadTimeout` is the idle timeout for the connection once it is established.

`WriteTimeout` is the timeout used when the server waits for the client to receive new data.

`HeartbeatTimeout` controls the internal heartbeat interval. ShellPort keeps it below the read timeout so the heartbeat can fire before the connection is considered idle.

`ReadDelay` and `WriteDelay` add small delays to reads and writes when you need to slow traffic down intentionally.

`TLSCertificateFile` and `TLSCertificateKeyFile` enable HTTPS. Both fields must be set together; ShellPort rejects a config that sets only one of them.

`ServerTitle` appears as a custom title on the home page.

`ServerMessage` appears on the home page as short text. It supports Markdown-style links.

## Preset Fields

Each preset has `ID`, `Title`, `Type`, `Host`, `TabColor`, and a `Meta` map.

`ID` is the stable identifier ShellPort uses when presets are edited through the API. File-backed configs can start with missing IDs; ShellPort fills them in and writes them back.

`Title` is the visible name shown in the UI.

`Type` selects the command family: `SSH`, `Telnet`, `Mosh`, or `ET`.

`Host` is the remote address, with port when needed.

`TabColor` tints the preset tab in the UI.

`Meta` stores the type-specific fields. The UI exposes the common ones directly, but the file format can also carry imported or advanced values.

The shared preset metadata fields are:

- `User`
- `Authentication`
- `Encoding`
- `Password`
- `Encrypted Password`
- `Private Key`
- `Fingerprint`

SSH and Mosh presets support `Password`, `Private Key`, or `None` authentication. ET presets currently support `Private Key` authentication only.

`Password` is the legacy plaintext saved-password field. When `SHELLPORT_PRESET_SECRET_KEY` is set, ShellPort migrates it to `Encrypted Password` automatically.

`Encrypted Password` is the server-side encrypted form of a saved password. Do not hand-edit it.

`Private Key` can contain the literal private-key text or a reference such as `file://...` or `environment://...`. File references are useful when you want ShellPort to keep the key material outside the main config file.

`Fingerprint` stores the SSH host key fingerprint. You can save it from the connection-time fingerprint prompt, and the UI preserves it during preset edits.

`Encoding` is the terminal encoding for the connection. Telnet offers the full browser-supported list. Mosh and ET are locked to `utf-8`.

## SSH, Mosh, And ET Details

SSH presets use the same preset form as the other connection types, but they are the most flexible and can work with password, private key, or no authentication.

Mosh presets still use SSH for bootstrap, but the browser session is proxied through ShellPort while the remote Mosh traffic uses UDP between the backend and the host. The remote host must have `mosh-server` installed. ShellPort does not support SOCKS5 for Mosh.

The `Mosh Server` field lets you override the local command used to launch the server-side Mosh helper. Leave it empty to use `mosh-server`.

ET presets require private-key authentication. The `ET Server Port` field controls the remote `etserver` TCP port and defaults to `2022`. The `ET Command` field names the local ET client command and currently only accepts the built-in `et` value.

ET does not support password authentication or SOCKS5 proxying.

## Preset UI Workflow

The preset editor in the UI follows the same configuration model as the JSON file.

For a new preset, fill in the preset name, type, host, and any type-specific fields. Choose the authentication mode, then decide whether to save a password or a private key. If you use the existing server key option, ShellPort can point the preset at a key file managed on disk instead of embedding key text in the main config.

If a preset already has a saved password or private key, the editor lets you keep the saved value or replace it. You can also clear the saved secret during an edit if you no longer want it stored.

When you save a fingerprint from the connection flow, ShellPort writes it back to the matching preset only. That path is intentionally separate from the main preset editor because it is tied to the live fingerprint confirmation workflow.
