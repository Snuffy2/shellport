# ET Go Library Check

Date: 2026-05-20

Decision: Use the ET CLI fallback with ShellPort-owned auth.

Reason:

- No maintained Go ET client library with a usable session API was found.
- The CLI fallback means ShellPort owns private-key SSH auth and host-trust validation before invoking `et`.
- Branch/spec confirmation: `git status --short --branch` returned `## add-et-support...origin/add-et-support [ahead 1]`, and the spec says to use a Go library only if it exposes a maintained ET client/session API.
- `go env GOPATH`: `/Users/snuffy2/go`
- Candidate inspection for `github.com/MisterTea/EternalTerminal`: `go list -m -json github.com/MisterTea/EternalTerminal@v1.1.0` returned `Dir: /Users/snuffy2/go/pkg/mod/github.com/!mister!tea/!eternal!terminal@v1.1.0`; `rg` found no Go `Session` or `Client` API; `sed` inspection returned `# Eternal Terminal` in `README.md` and `#!/bin/bash` plus ET launcher variables in `launcher/et`.
- Inference from `README.md` and `launcher/et`: the official Eternal Terminal implementation packages an `et` client executable/launcher, not a Go session library.

Commands checked:

- `go env GOPATH`: "/Users/snuffy2/go"
- `git status --short --branch`: "## add-et-support...origin/add-et-support [ahead 1]"
- `sed -n '1,180p' docs/superpowers/specs/2026-05-20-et-support-design.md`: "Implementation must first check for a maintained Go ET client library with a usable session API."
- `go list -m -versions github.com/MisterTea/EternalTerminal`: "github.com/MisterTea/EternalTerminal v1.0.0 v1.0.1 v1.0.2 v1.0.3 v1.0.4 v1.1.0"
- `go list -m -json github.com/MisterTea/EternalTerminal@v1.1.0`: `"Path": "github.com/MisterTea/EternalTerminal"; "Version": "v1.1.0"; "Dir": "/Users/snuffy2/go/pkg/mod/github.com/!mister!tea/!eternal!terminal@v1.1.0"; "Origin.URL": "https://github.com/MisterTea/EternalTerminal"`
- `go mod download -json github.com/MisterTea/EternalTerminal@v1.1.0`: `"Dir": "/Users/snuffy2/go/pkg/mod/github.com/!mister!tea/!eternal!terminal@v1.1.0"; "Sum": "h1:QtrGCxpORQKluLN0DTpo9pWFcdM98y4M0uhUpOE1MPs="; "Origin.Ref": "refs/tags/v1.1.0"`
- `rg -n "type (Session|Client)|func .*Connect|interface.*Session|Read\\(\\[\\]byte\\)|Resize" /Users/snuffy2/go/pkg/mod/github.com/!mister!tea/!eternal!terminal@v1.1.0`: no output; exit status 1.
- `sed -n '1,2p' /Users/snuffy2/go/pkg/mod/github.com/!mister!tea/!eternal!terminal@v1.1.0/README.md`: `# Eternal Terminal`; `https://mistertea.github.io/EternalTCP/`
- `sed -n '1,16p' /Users/snuffy2/go/pkg/mod/github.com/!mister!tea/!eternal!terminal@v1.1.0/launcher/et`: `#!/bin/bash`; `PORT="2022"`; `ET_COMMAND=""`; `SSH_COMMAND=""`
- `go list -m -versions github.com/eternal-terminal/et`: "no usable Go module returned"
- `go list -m -versions github.com/MisterTea/et`: "no usable Go module returned"
