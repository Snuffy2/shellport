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
      - "8182:8182"
    volumes:
      - ./config:/etc/shellport
    environment:
      # Optional: override the config file path. By default, this compose file
      # uses /etc/shellport/shellport.conf.json from the mounted ./config dir.
      # SHELLPORT_CONFIG: /etc/shellport/shellport.conf.json
      # Optional: IANA timezone used for local timestamps in logs.
      TZ: America/New_York
      # Recommended: base64-encoded 32-byte key for encrypted preset passwords.
      # Generate with: openssl rand -base64 32
      SHELLPORT_PRESET_SECRET_KEY: "replace-with-generated-key"
      # Optional: set to "1" to enable debug-level logs on Docker stdout.
      # SHELLPORT_DEBUG: "1"
```

Then open `http://localhost:8182`.

For reverse proxy deployments, publish the service only on localhost:

```yaml
ports:
  - "127.0.0.1:8182:8182"
```

## Configuration

ShellPort can be configured with a JSON configuration file or environment variables. See [CONFIGURATION.md](CONFIGURATION.md) for the full configuration reference.

The Docker Compose example above mounts `./config` as a writable configuration directory. By default, ShellPort loads `/etc/shellport/shellport.conf.json`, creating it with a minimal `0.0.0.0:8182` configuration if no default config file exists. That generated file leaves `SharedKey` and `AdminKey` empty so you can add presets from the UI first, then edit the config file later for authentication, admin protection, or other advanced settings. `SHELLPORT_CONFIG` is only needed when you want to override the config path.

Writable file-backed configuration enables preset updates from the UI, such as saving SSH/Mosh fingerprints. If `SHELLPORT_PRESET_SECRET_KEY` is set, plaintext preset `Password` values are migrated on startup to `Encrypted Password` and the plaintext value is removed from the JSON file. That key must be set through the environment, not in JSON. Without that key, plaintext password presets continue
to work as before. Full preset add/edit/remove API writes require admin access. The UI prompts for `AdminKey` before the first protected preset create, edit, or delete action and remembers a successful AdminKey only for the current browser page session. If `AdminKey` is blank, anyone with UI access can manage presets. If both `SharedKey` and `AdminKey` are blank, everyone gets admin access without authentication. Fingerprint saves remain available only from the connection-time fingerprint prompt.

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
