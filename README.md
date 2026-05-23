# ShellPort

ShellPort is a browser-based remote shell for SSH, Telnet, Mosh, and Eternal Terminal (ET).

## Quick Start

The easiest way to run ShellPort is with Docker and a writable config directory.

![Screenshot](screenshot.png)

1. Create a config directory:

```sh
mkdir -p config
```

2. Start the container:

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

3. Open `http://localhost:8182`.

If `config/shellport.conf.json` does not exist, ShellPort creates it on first boot. The file keeps the `.json` extension, but ShellPort accepts JSONC-style comments and trailing commas. Use `shellport.conf.example.json` as a loadable annotated reference while you edit the live file.

Keep the service on `127.0.0.1` until you have added the passwords or access controls you want.

Generate `SHELLPORT_PRESET_SECRET_KEY` once with `openssl rand -base64 32` and reuse the same value for every restart if you want encrypted preset passwords to remain readable.

If you prefer Docker Compose, the repository also includes `docker-compose.example.yaml` with the same layout.

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

Preset create, edit, and delete actions require a writable file-backed configuration. If `AdminPassword` is set, the UI will prompt for it before protected preset changes. If `AdminPassword` is blank and `UserPassword` is set, any authenticated user can manage presets. If both passwords are blank, preset management is open to anyone who can reach the UI.

## Configuration

See [CONFIGURATION.md](CONFIGURATION.md) for the full configuration reference.

The important setup choices are:

- `UserPassword` controls access to the web UI.
- `AdminPassword` protects preset writes when you want separate admin access.
- `SHELLPORT_PRESET_SECRET_KEY` lets ShellPort encrypt saved preset passwords before writing them back to disk.
- `TLSCertificateFile` and `TLSCertificateKeyFile` enable HTTPS for a server listener.
- `Socks5` routes outbound connections through a SOCKS5 proxy.
- `OnlyAllowPresetRemotes` limits outbound connections to hosts that are already defined as presets.

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

`npm run dev` starts the Go backend with `shellport.conf.example.json` and serves the frontend through Vite with HMR and backend proxying.

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
