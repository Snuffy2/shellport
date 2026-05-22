# ShellPort

Browser-based remote shell access over SSH, Telnet, Mosh, and Eternal Terminal (ET)

![Screenshot](screenshot.png)

## Supported browsers

Most browsers over the past few years, including Chrome 80+, Edge 80+, Firefox 78+, and Safari 14+.

## Docker

Run ShellPort with Docker Compose:

```yaml
services:
  shellport:
    image: ghcr.io/snuffy2/shellport:latest
    container_name: shellport
    restart: unless-stopped
    ports:
      - "127.0.0.1:8182:8182"
    volumes:
      - ./config:/config
    environment:
      # Optional: override the config file path. By default, this compose file
      # uses /config/shellport.conf.json from the mounted ./config dir.
      # SHELLPORT_CONFIG: /config/shellport.conf.json
      # Optional: IANA timezone used for local timestamps in logs.
      TZ: America/New_York
      # Recommended: base64-encoded 32-byte key for encrypted preset passwords.
      # Generate with: openssl rand -base64 32
      SHELLPORT_PRESET_SECRET_KEY: "replace-with-generated-key"
      # Optional: set to "1" to enable debug-level logs on Docker stdout.
      # SHELLPORT_DEBUG: "1"
```

Then open `http://localhost:8182`.

The example publishes ShellPort only on localhost so a first-run instance is not
exposed on the network before you add authentication to the generated config
file. For direct LAN access, change the port mapping after setting
`UserPassword` or `AdminPassword`.

## Configuration

ShellPort is configured with a JSON configuration file. See [CONFIGURATION.md](CONFIGURATION.md) for the full configuration reference.

The Docker Compose example above mounts `./config` as the writable `/config` directory. By default, ShellPort loads `/config/shellport.conf.json`, creating it with a minimal `0.0.0.0:8182` configuration if the file does not exist. That generated file leaves `UserPassword` and `AdminPassword` empty so you can add presets from the UI first, then edit the config file later for authentication, admin protection, or other advanced settings. `SHELLPORT_CONFIG` is only needed when you want to override the config file path.

Writable file-backed configuration enables preset updates from the UI, such as saving SSH/Mosh fingerprints. If `SHELLPORT_PRESET_SECRET_KEY` is set, plaintext preset `Password` values are migrated on startup to `Encrypted Password` and the plaintext value is removed from the JSON file. That key must be set through the environment, not in JSON. Without that key, plaintext password presets continue
to work as before. Full preset add/edit/remove API writes require admin access. The UI prompts for `AdminPassword` before the first protected preset create, edit, or delete action and remembers a successful AdminPassword only for the current browser page session. If `AdminPassword` is blank, anyone with UI access can manage presets. If both `UserPassword` and `AdminPassword` are blank, everyone gets admin access without authentication. Fingerprint saves remain available only from the connection-time fingerprint prompt.

Generate a preset secret key with one of these commands:

#### macOS/Linux

```sh
openssl rand -base64 32
```

#### Windows PowerShell

```powershell
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
$bytes = New-Object byte[] 32
$rng.GetBytes($bytes)
[Convert]::ToBase64String($bytes)
```

## Mosh and Eternal Terminal (ET)

Mosh support is available with SSH used for bootstrap only. The browser connection to ShellPort still uses WebSocket, while Mosh data flows over UDP between the backend container and the remote host. Remote hosts need `mosh-server` installed, SOCKS5 is not supported for Mosh, the backend-to-host Mosh leg is IPv4-only, and terminal encoding is fixed to UTF-8.

ET support is available for private-key authentication. ShellPort verifies the SSH host fingerprint and private key before launching the local `et` client, then proxies the ET client PTY over the existing browser WebSocket. ET uses the remote `etserver` TCP port, defaulting to `2022`. SOCKS5 proxying and password authentication are not supported for ET v1. Closing the ShellPort browser session terminates the backend `et` client process, matching the current Mosh-style session lifetime.

<details>
<summary><h2>Running From Source</h2></summary>

Use this path for development.

Prerequisites:

- `git`
- `go`
- `node` 24 or newer
- `npm`

Build the frontend assets and backend binary:

```sh
git clone https://github.com/Snuffy2/shellport.git
cd shellport
npm ci
npm run build
```

Run the development server:

```sh
npm run dev
```

The development command starts the Go backend with `shellport.conf.example.json` and serves the frontend through Vite with HMR. Vite proxies backend routes such as `/shellport/socket` to the Go process.

The generated production binary is written to `./shellport` by `npm run build`.

Useful development checks:

```sh
npm run generate
npm run testonly
npm run lint
go test ./...
```

`npm run generate` produces the Vite assets and then refreshes the embedded Go static assets.

</details>

## Fork

This repository is a fork of [nirui/sshwifty](https://github.com/nirui/sshwifty). The original project and its design are the work of [@nirui](https://github.com/nirui), whose excellent work made this possible.

## License

Code in this project is licensed under AGPL-3.0-only. See [LICENSE.md](LICENSE.md) for details.

Third-party components are licensed under their respective licenses. See [DEPENDENCIES.md](DEPENDENCIES.md) for dependency copyright and license details.
