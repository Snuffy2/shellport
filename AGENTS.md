# AGENTS.md

## Project Overview

ShellPort is browser-based remote shell access over SSH, Telnet, ET, and Mosh. The
repository combines:

- A Go backend in `shellport.go` and `application/`.
- A Vue 3 frontend in `ui/`.
- A Vite build pipeline in `vite.config.js`.
- Docker packaging in `Dockerfile` and `docker-compose.example.yaml`.
- GitHub Actions automation under `.github/`.

The module path is `github.com/Snuffy2/shellport`. The project is licensed under
AGPL-3.0-only; preserve existing license headers when editing source files.

## Source Layout

- `application/configuration/`: config file and environment loading.
- `application/controller/`: HTTP routes, static page serving, socket handling.
- `application/command/` and `application/commands/`: command protocol and
  typed command values shared with connection flows.
- `application/network/`: TCP/SOCKS dialing and connection wrappers.
- `application/server/`: HTTP server setup.
- `application/log/`: project logging helpers.
- `ui/`: Vue components, browser command protocol, stream handling, styles,
  static files, and frontend tests.
- `application/controller/static_page_generater/`: generator used by
  `go generate` during frontend/static asset generation.
- `.tmp/`: generated build output. Treat it as disposable unless a task is
  explicitly about generated output.

## Development Commands

Install Node dependencies with:

```sh
npm ci
```

Use the scripts in `package.json` as the source of truth:

```sh
npm run generate
npm run build
npm run lint
npm run lint:fix
npm run testonly
npm test
```

Important behavior:

- `npm run generate` cleans `.tmp/`, builds frontend assets with Vite, and
  runs Go static page generation.
- `npm run build` runs generation and then builds the `shellport` binary.
- `npm run testonly` runs Vitest frontend tests and `go test ./... -race`.
- `npm test` runs generation first, then `testonly`.
- `npm run dev` starts the Go backend with `scripts/shellport.dev.conf.yml` and
  runs a Vite dev server with HMR and backend proxying.

For Go-only checks, use:

```sh
go test ./...
go test ./... -race -timeout 30s
go vet ./...
go mod tidy
```

For hook parity with CI, use the repo-local `prek.toml`:

```sh
prek run --all-files
```

## Validation Expectations

Run the narrowest validation that proves the change, then broaden based on
risk:

- Go backend change: run targeted `go test` for the touched package, then
  consider `go test ./... -race -timeout 30s`.
- Frontend logic change: run the relevant Vitest tests or `npm run testonly`.
- Build pipeline, generated assets, or static serving change: run
  `npm run generate` and, when practical, `npm run build`.
- Lint/style-only change: run `npm run lint` or
  `prek run --all-files`.
- GitHub Actions change: run `prek run --all-files` so `actionlint`
  runs through the configured hook.

CI runs `npm ci`, `npm run generate`, and `prek` on pushes to `main`, pull
requests, and manual dispatch.

## Coding Conventions

- Keep imports at the top of files and preserve existing comments.
- New source files under `application/`, `scripts/`, and `ui/` should use the concise AGPL SPDX
  header style at the top of the file: `Copyright (C) 2026 Snuffy2`, then
  `SPDX-License-Identifier: AGPL-3.0-only`, using the file's native comment
  syntax. Do not add the full AGPL boilerplate to new files.
- Prefer small, root-cause fixes over broad rewrites.
- Match existing Go package structure and frontend component patterns.
- Add or update tests for changed behavior.
- Keep Go code formatted with `gofmt`.
- Keep frontend code compatible with Vue 3 and the current Vite setup.
- Use existing command, stream, and connector abstractions instead of
  duplicating protocol logic.
- Treat hook commands and connection inputs as untrusted; avoid command-line
  injection and keep error handling explicit.
- Do not commit generated `.tmp/` output unless the user explicitly asks.

## Frontend Notes

The UI uses Vue 3 single-file components and plain CSS under `ui/`. Tests live
beside frontend modules as `*_test.js` and run under Vitest.

### When changing UI behavior

- Keep terminal behavior, stream handling, and keyboard handling stable.
- Check mobile and desktop layout assumptions for visible UI changes.
- Prefer existing widgets under `ui/widgets/` over new one-off controls.
- Preserve accessible, inspectable text and avoid unnecessary visual churn.

## Backend Notes

The backend serves the web application and proxies SSH/Telnet sessions over the
project command protocol.

### When changing backend behavior

- Keep configuration compatibility with both file and environment loaders.
- Preserve timeout semantics for dialing, hooks, HTTP reads, and writes.
- Keep hook execution bounded by configured deadlines and sanitize any new
  external-process inputs.
- Avoid logging secrets such as shared keys, SOCKS credentials, TLS key
  material, or preset credentials.

## Docker And Release Notes

`Dockerfile` is the canonical reference for container builds. It builds the
application in Debian-based stages, then copies the final binary into an Alpine
runtime image.

GitHub release publishing is configured in `.github/workflows/release.yml` for
GHCR image `ghcr.io/snuffy2/shellport`.

Do not push branches, publish images, or open pull requests unless the user
explicitly asks.

## Git And File Safety

- Do not revert user changes unless explicitly instructed.
- Before editing a file that already has uncommitted changes, inspect it and
  work with the current contents.
- Keep changes scoped to the requested task.
