# ShellPort

ShellPort is a browser-based remote shell for SSH, Telnet, Mosh, and Eternal Terminal (ET).

![Screenshot](screenshot.png)

## Table Of Contents

- [Quick Start With Docker Compose](#quick-start-with-docker-compose)
- [Docker Run](#docker-run)
- [First Launch Checklist](#first-launch-checklist)
- [Create Presets In The UI](#create-presets-in-the-ui)
- [Configuration](#configuration)
- [Browser Support](#browser-support)
- [Running From Source](#running-from-source)
- [Fork](#fork)
- [License](#license)

## Quick Start With Docker Compose

The easiest way to run ShellPort is with Docker Compose and a writable config
directory.

1. Create a working directory and a config directory:

```sh
mkdir shellport
cd shellport
mkdir -p config
```

2. Create `docker-compose.yaml`:

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
      TZ: America/New_York
      SHELLPORT_PRESET_SECRET_KEY: "replace-with-generated-key"
```

Before first start, replace `replace-with-generated-key` with the output of
`openssl rand -base64 32`. Keep the same value for every restart so ShellPort
can read encrypted saved preset passwords later.

3. Start ShellPort:

```sh
docker compose up -d
```

4. Open `http://localhost:8182`.

If `config/shellport.conf.json` does not exist, ShellPort creates it on first
boot. Use the repository's `shellport.conf.example.json` as an annotated
reference while you edit the live file.

The example only publishes ShellPort on `127.0.0.1`. Keep it there until you
have added the passwords or access controls you want. To expose it on your LAN
or behind a reverse proxy, change the port mapping after hardening the config.

The repository also includes `docker-compose.example.yaml` with the same layout.

## Docker Run

```sh
docker run -d \
  --name shellport \
  --restart unless-stopped \
  -p 127.0.0.1:8182:8182 \
  -v "$PWD/config:/config" \
  -e TZ=America/New_York \
  -e SHELLPORT_PRESET_SECRET_KEY="<base64-encoded-32-byte-key>" \
  ghcr.io/snuffy2/shellport:latest
```

## First Launch Checklist

1. Open the UI while it is still bound to `127.0.0.1`.
2. Create one or more presets from the Connector view.
3. Edit `config/shellport.conf.json` and set `UserPassword` before exposing the
   service to other machines.
4. Set `AdminPassword` if preset create, edit, and delete actions should require
   a separate admin password.
5. Restart the container after changing top-level config values such as
   passwords, listeners, TLS, SOCKS5, hooks, or preset-only restrictions.

## Create Presets In The UI

Presets are the normal way to connect to hosts. Open the Connector view and use the preset editor to create a new preset or edit an existing one.

The main fields match the UI:

- `Preset name`
- `Type`
- `Host`
- `Tab color`
- `User`
- `Authentication`
- `Password`
- `Private key source`
- `Encoding`
- `Mosh Server`
- `ET Server Port`
- `ET Command`

For SSH and Mosh presets, you can choose password or private key authentication. ET presets currently use private key authentication only.

If the preset already has a saved password or private key, the editor lets you keep it, replace it, or clear it. Fingerprints can be saved from the connection-time fingerprint prompt.

Preset create, edit, and delete actions require a writable file-backed
configuration. If `AdminPassword` is set, the UI prompts for it before protected
preset changes. If `AdminPassword` is blank and `UserPassword` is set, any
authenticated user can manage presets. If both passwords are blank, preset
management is open to anyone who can reach the UI.

## Configuration

See [CONFIGURATION.md](CONFIGURATION.md) for the full configuration reference.

The important setup choices are:

- `UserPassword` controls access to the web UI.
- `AdminPassword` protects preset writes when you want separate admin access.
- `SHELLPORT_PRESET_SECRET_KEY` lets ShellPort encrypt saved preset passwords before writing them back to disk.
- `TLSCertificateFile` and `TLSCertificateKeyFile` enable HTTPS for a server listener.
- `Socks5` routes outbound connections through a SOCKS5 proxy.
- `OnlyAllowPresetRemotes` limits outbound connections to hosts that are already defined as presets and disables preset management.

Mosh uses SSH only to start the remote session; the Mosh data path uses UDP
between the ShellPort container and the remote host. ET uses the local `et`
client in the container and the remote `etserver` TCP port.

## Browser Support

ShellPort works with recent versions of Chrome, Edge, Firefox, and Safari.

## Running From Source

Use this path if you want to develop ShellPort locally.

Prerequisites:

- `git`
- `go`
- `node` 24 or newer
- `npm`

Clone the repo and build the app:

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

`npm run dev` starts the Go backend with a writable local config copied from `scripts/shellport.dev.conf.json` into `.tmp/dev/shellport.conf.json`, then serves the frontend through Vite with HMR and backend proxying.

Useful checks while developing:

```sh
npm run generate
npm run testonly
npm run lint
go test ./...
```

`npm run generate` rebuilds the frontend assets and refreshes the embedded static assets used by the Go backend.

## Fork

This repository is a fork of [nirui/sshwifty](https://github.com/nirui/sshwifty). The original project and design are the work of [@nirui](https://github.com/nirui).

## License

Code in this project is licensed under AGPL-3.0-only. See [LICENSE](LICENSE.md) for details.

Third-party components are licensed under their respective licenses. See [DEPENDENCIES](DEPENDENCIES.md) for dependency copyright and license details.
