# Eternal Terminal Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Eternal Terminal (ET) as a fourth ShellPort transport with ShellPort-owned private-key auth, host fingerprint handling, ET server port configuration, and Mosh-like session lifetime.

**Architecture:** Keep the browser-to-ShellPort WebSocket stream unchanged. First verify whether a maintained Go ET client library exposes a usable session API; if not, implement the planned CLI fallback by launching `et` under a PTY after ShellPort validates SSH auth and host trust. Mirror Mosh's command/control registration pattern while keeping ET-specific auth, metadata, process, and Docker concerns isolated in ET files.

**Tech Stack:** Go 1.26, `golang.org/x/crypto/ssh`, `github.com/creack/pty`, Vue 3 command/control modules, Vitest, Go tests, Docker Alpine runtime.

---

## File Structure

- Create `docs/superpowers/research/2026-05-20-et-go-library.md`: records the implementation-time Go-library check and final choice.
- Modify `go.mod` and `go.sum`: promote `github.com/creack/pty` to a direct dependency if the CLI fallback is used.
- Create `application/commands/et_metadata.go`: ET metadata constants, parsing, validation, and launcher/preset names.
- Create `application/commands/et_metadata_test.go`: unit tests for ET port, command, and metadata parsing.
- Create `application/commands/et_process.go`: CLI fallback process builder, temp SSH material writer, PTY runner interface, and cleanup helpers.
- Create `application/commands/et_process_test.go`: fake executable/temp-file/cleanup tests.
- Create `application/commands/et.go`: ET command FSM, SSH validation flow, PTY bridging, local frame handling, and lifecycle.
- Create `application/commands/et_test.go`: backend command registration, bootup validation, unsupported auth/proxy, and PTY bridge tests.
- Modify `application/commands/commands.go`: register `ET` after `Mosh`.
- Modify `application/commands/connection_logging.go`: classify ET expected disconnect errors if a new sentinel is introduced.
- Create `ui/commands/et.js`: ET frontend command, wizard fields, request encoding, fingerprint/credential handling, and launchers.
- Create `ui/commands/et_test.js`: ET command id, wizard validation, request payload, error mapping, and launcher tests.
- Create `ui/control/et.js`: terminal control for ET PTY byte stream.
- Create `ui/control/et_test.js`: stdout/stdin/resize/close behavior.
- Modify `ui/app.js`: register ET command and control.
- Modify `ui/home_preset_execution.js`: allow ET direct-launch only for private-key presets and include ET metadata.
- Modify `ui/home_preset_execution_test.js`: direct-launch and password-rejection coverage for ET.
- Modify `README.md`, `CONFIGURATION.md`, and `shellport.conf.example.json`: document ET support, limitations, and example preset.
- Modify `Dockerfile`: add verified ET CLI/OpenSSH runtime packages if the CLI fallback is used.

## Task 1: Verify ET Library Choice

**Files:**
- Create: `docs/superpowers/research/2026-05-20-et-go-library.md`

- [ ] **Step 1: Confirm branch and spec**

Run:

```bash
git status --short --branch
sed -n '1,180p' docs/superpowers/specs/2026-05-20-et-support-design.md
```

Expected: branch is `add-et-support`, working tree has no unrelated changes, and the spec says to use a Go library only if it exposes a maintained ET client/session API.

- [ ] **Step 2: Search for a Go ET client library**

Run:

```bash
go env GOPATH
go list -m -versions github.com/MisterTea/EternalTerminal
go list -m -versions github.com/eternal-terminal/et
go list -m -versions github.com/MisterTea/et
```

Expected: these module checks either fail with "invalid version" / "repository not found" / no usable Go module, or identify a real module to inspect.

- [ ] **Step 3: Inspect any candidate before deciding**

If Step 2 prints a real Go module path and versions, replace `github.com/example/etclient` below with that module path and run:

```bash
go doc github.com/example/etclient
go doc github.com/example/etclient/...
```

Accept the Go-library path only if the module exposes all of these capabilities:

```go
type Session interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Resize(rows uint16, cols uint16) error
	Close() error
}

type Client interface {
	Connect(ctx context.Context, user string, sshHost string, etPort int, privateKey []byte, knownHost string) (Session, error)
}
```

If no candidate exposes equivalent capabilities, choose the CLI fallback.

- [ ] **Step 4: Write the research note**

Create `docs/superpowers/research/2026-05-20-et-go-library.md` with this content. Replace each quoted result string with the exact single-line result from the command; use `"no usable Go module returned"` when the command returns a non-zero status and no module versions:

```markdown
# ET Go Library Check

Date: 2026-05-20

Decision: Use the ET CLI fallback with ShellPort-owned auth.

Reason:

- No maintained Go ET client library with a usable session API was found.
- The official Eternal Terminal implementation packages the `et` client executable.
- ShellPort will validate private-key SSH auth and host trust before launching `et`.

Commands checked:

- `go list -m -versions github.com/MisterTea/EternalTerminal`: "no usable Go module returned"
- `go list -m -versions github.com/eternal-terminal/et`: "no usable Go module returned"
- `go list -m -versions github.com/MisterTea/et`: "no usable Go module returned"
```

- [ ] **Step 5: Commit the research note**

Run:

```bash
git add -f docs/superpowers/research/2026-05-20-et-go-library.md
git commit -m "docs: record ET client library decision"
```

Expected: one commit containing only the research note.

## Task 2: Add ET Metadata Parsing

**Files:**
- Create: `application/commands/et_metadata.go`
- Create: `application/commands/et_metadata_test.go`

- [ ] **Step 1: Write failing metadata tests**

Create `application/commands/et_metadata_test.go`:

```go
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"errors"
	"testing"

	"github.com/Snuffy2/shellport/application/rw"
)

func TestParseETMetadataDefaults(t *testing.T) {
	limited := rw.NewLimitedReader(nil, 0)
	metadata, err := parseETMetadata(&limited, make([]byte, 1024))
	if err != nil {
		t.Fatalf("parseETMetadata() error = %v", err)
	}
	if metadata.ServerPort != etDefaultServerPort {
		t.Fatalf("ServerPort = %d, want %d", metadata.ServerPort, etDefaultServerPort)
	}
	if metadata.Command != etDefaultCommand {
		t.Fatalf("Command = %q, want %q", metadata.Command, etDefaultCommand)
	}
}

func TestValidateETServerPort(t *testing.T) {
	for _, port := range []int{1, 2022, 65535} {
		if err := validateETServerPort(port); err != nil {
			t.Fatalf("validateETServerPort(%d) error = %v", port, err)
		}
	}
	for _, port := range []int{0, -1, 65536} {
		if err := validateETServerPort(port); !errors.Is(err, ErrETInvalidServerPort) {
			t.Fatalf("validateETServerPort(%d) error = %v, want ErrETInvalidServerPort", port, err)
		}
	}
}

func TestValidateETCommand(t *testing.T) {
	if err := validateETCommand("et"); err != nil {
		t.Fatalf("validateETCommand(et) error = %v", err)
	}
	if err := validateETCommand("/usr/local/bin/et"); err != nil {
		t.Fatalf("validateETCommand(path) error = %v", err)
	}
	for _, command := range []string{"", "et --flag", "et\nbad"} {
		if err := validateETCommand(command); !errors.Is(err, ErrETInvalidCommand) {
			t.Fatalf("validateETCommand(%q) error = %v, want ErrETInvalidCommand", command, err)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./application/commands -run 'TestParseETMetadataDefaults|TestValidateET' -count=1
```

Expected: FAIL because `parseETMetadata`, `validateETServerPort`, and `validateETCommand` are undefined.

- [ ] **Step 3: Add metadata implementation**

Create `application/commands/et_metadata.go`:

```go
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"errors"
	"strconv"
	"strings"

	"github.com/Snuffy2/shellport/application/rw"
)

const (
	etDefaultCommand    = "et"
	etDefaultServerPort = 2022
	etMaxCommandLen     = 512
)

var (
	ErrETInvalidServerPort = errors.New("invalid ET server port")
	ErrETInvalidCommand    = errors.New("invalid ET command")
)

type etMetadata struct {
	ServerPort int
	Command    string
}

func defaultETMetadata() etMetadata {
	return etMetadata{
		ServerPort: etDefaultServerPort,
		Command:    etDefaultCommand,
	}
}

func parseETMetadata(r *rw.LimitedReader, b []byte) (etMetadata, error) {
	metadata := defaultETMetadata()
	if r == nil || r.Completed() {
		return metadata, nil
	}

	portString, err := ParseString(r.Read, b)
	if err != nil {
		return etMetadata{}, err
	}
	portText := strings.TrimSpace(string(portString.Data()))
	if portText != "" {
		port, err := strconv.Atoi(portText)
		if err != nil {
			return etMetadata{}, ErrETInvalidServerPort
		}
		if err := validateETServerPort(port); err != nil {
			return etMetadata{}, err
		}
		metadata.ServerPort = port
	}

	if r.Completed() {
		return metadata, nil
	}

	commandString, err := ParseString(r.Read, b)
	if err != nil {
		return etMetadata{}, err
	}
	commandText := strings.TrimSpace(string(commandString.Data()))
	if commandText != "" {
		if err := validateETCommand(commandText); err != nil {
			return etMetadata{}, err
		}
		metadata.Command = commandText
	}

	return metadata, nil
}

func validateETServerPort(port int) error {
	if port < 1 || port > 65535 {
		return ErrETInvalidServerPort
	}
	return nil
}

func validateETCommand(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return ErrETInvalidCommand
	}
	if len(command) > etMaxCommandLen {
		return ErrETInvalidCommand
	}
	if strings.ContainsAny(command, " \t\r\n") {
		return ErrETInvalidCommand
	}
	return nil
}
```

- [ ] **Step 4: Run metadata tests**

Run:

```bash
go test ./application/commands -run 'TestParseETMetadataDefaults|TestValidateET' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit metadata slice**

Run:

```bash
gofmt -w application/commands/et_metadata.go application/commands/et_metadata_test.go
git add application/commands/et_metadata.go application/commands/et_metadata_test.go
git commit -m "feat: add ET metadata validation"
```

Expected: commit contains only ET metadata files.

## Task 3: Add ET Process and Temp SSH Material Helpers

**Files:**
- Create: `application/commands/et_process.go`
- Create: `application/commands/et_process_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Write failing process-helper tests**

Create `application/commands/et_process_test.go`:

```go
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildETClientArgsUsesSSHConfigAndServerPort(t *testing.T) {
	args := buildETClientArgs(etMetadata{Command: "et", ServerPort: 22022}, "alice", "example.com:22", "/tmp/ssh_config")
	want := []string{"-ssh-config", "/tmp/ssh_config", "alice@example.com:22022"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestWriteETSSHMaterialCreatesRestrictiveFiles(t *testing.T) {
	dir := t.TempDir()
	material, err := writeETSSHMaterial(dir, []byte("PRIVATE KEY\n"), "example.com", "SHA256:abc")
	if err != nil {
		t.Fatalf("writeETSSHMaterial() error = %v", err)
	}

	for _, path := range []string{material.IdentityPath, material.KnownHostsPath, material.ConfigPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s mode = %v, want no group/other permissions", path, info.Mode().Perm())
		}
	}

	config, err := os.ReadFile(material.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	configText := string(config)
	for _, expected := range []string{"IdentityFile " + material.IdentityPath, "UserKnownHostsFile " + material.KnownHostsPath, "BatchMode yes"} {
		if !strings.Contains(configText, expected) {
			t.Fatalf("config missing %q:\n%s", expected, configText)
		}
	}
}

func TestCleanupETTempDirRemovesDirectory(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "file")
	if err := os.WriteFile(child, []byte("x"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := cleanupETTempDir(dir); err != nil {
		t.Fatalf("cleanupETTempDir() error = %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("stat dir error = %v, want not exist", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./application/commands -run 'TestBuildETClientArgs|TestWriteETSSHMaterial|TestCleanupETTempDir' -count=1
```

Expected: FAIL because the ET process helper functions are undefined.

- [ ] **Step 3: Add process helper implementation**

Create `application/commands/et_process.go`:

```go
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

type etSSHMaterial struct {
	IdentityPath   string
	KnownHostsPath string
	ConfigPath     string
}

func buildETClientArgs(metadata etMetadata, user string, sshAddress string, sshConfigPath string) []string {
	host := sshAddress
	if splitHost, _, err := net.SplitHostPort(sshAddress); err == nil {
		host = splitHost
	}
	return []string{
		"-ssh-config",
		sshConfigPath,
		fmt.Sprintf("%s@%s:%d", user, host, metadata.ServerPort),
	}
}

func writeETSSHMaterial(dir string, privateKey []byte, host string, fingerprint string) (etSSHMaterial, error) {
	material := etSSHMaterial{
		IdentityPath:   filepath.Join(dir, "identity"),
		KnownHostsPath: filepath.Join(dir, "known_hosts"),
		ConfigPath:     filepath.Join(dir, "ssh_config"),
	}
	if err := os.WriteFile(material.IdentityPath, privateKey, 0o600); err != nil {
		return etSSHMaterial{}, err
	}
	knownHostsLine := strings.TrimSpace(host + " " + fingerprint)
	if err := os.WriteFile(material.KnownHostsPath, []byte(knownHostsLine+"\n"), 0o600); err != nil {
		return etSSHMaterial{}, err
	}
	config := strings.Join([]string{
		"Host *",
		"  IdentitiesOnly yes",
		"  IdentityFile " + material.IdentityPath,
		"  UserKnownHostsFile " + material.KnownHostsPath,
		"  StrictHostKeyChecking yes",
		"  BatchMode yes",
		"",
	}, "\n")
	if err := os.WriteFile(material.ConfigPath, []byte(config), 0o600); err != nil {
		return etSSHMaterial{}, err
	}
	return material, nil
}

func cleanupETTempDir(dir string) error {
	if dir == "" {
		return nil
	}
	return os.RemoveAll(dir)
}
```

- [ ] **Step 4: Promote PTY dependency to direct if CLI fallback uses it**

Run:

```bash
go get github.com/creack/pty@v1.1.24
go mod tidy
```

Expected: `github.com/creack/pty v1.1.24` is direct in `go.mod` if imported by ET code later; `go.sum` remains tidy.

- [ ] **Step 5: Run process-helper tests**

Run:

```bash
gofmt -w application/commands/et_process.go application/commands/et_process_test.go
go test ./application/commands -run 'TestBuildETClientArgs|TestWriteETSSHMaterial|TestCleanupETTempDir' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit process helper slice**

Run:

```bash
git add application/commands/et_process.go application/commands/et_process_test.go go.mod go.sum
git commit -m "feat: add ET process helpers"
```

Expected: commit contains ET helper files and dependency metadata only.

## Task 4: Add Backend ET Command Skeleton and Registration

**Files:**
- Create: `application/commands/et.go`
- Create: `application/commands/et_test.go`
- Modify: `application/commands/commands.go`

- [ ] **Step 1: Write failing command registration and bootup validation tests**

Create `application/commands/et_test.go`:

```go
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
)

func TestCommandsIncludesET(t *testing.T) {
	commands := New()
	expectedNames := map[byte]string{
		0x00: "Telnet",
		0x01: "SSH",
		0x02: "Mosh",
		0x03: "ET",
	}
	for id, expectedName := range expectedNames {
		name := reflect.ValueOf(commands[id]).FieldByName("name").String()
		if name != expectedName {
			t.Fatalf("expected command %d to be %q, got %q", id, expectedName, name)
		}
	}
}

func TestETRejectsSocks5Proxy(t *testing.T) {
	client := newET(
		log.NewDitch(),
		command.NewHooks(configuration.HookSettings{}),
		command.StreamResponder{},
		command.Configuration{Socks5Configured: true},
		command.NewBufferPool(4096),
	).(*etClient)
	err := client.validateProxySupport()
	if !errors.Is(err, ErrETSocks5Unsupported) {
		t.Fatalf("validateProxySupport() error = %v, want ErrETSocks5Unsupported", err)
	}
}

func TestETAcceptsOnlyPrivateKeyAuth(t *testing.T) {
	client := &etClient{}
	if _, err := client.buildAuthMethod(SSHAuthMethodPrivateKey, "", "alice", "example.com:22"); err != nil {
		t.Fatalf("private-key auth error = %v", err)
	}
	for _, method := range []byte{SSHAuthMethodNone, SSHAuthMethodPassphrase, 0xff} {
		if _, err := client.buildAuthMethod(method, "", "alice", "example.com:22"); !errors.Is(err, ErrETUnsupportedAuthMethod) {
			t.Fatalf("method %d error = %v, want ErrETUnsupportedAuthMethod", method, err)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./application/commands -run 'TestCommandsIncludesET|TestETRejectsSocks5Proxy|TestETAcceptsOnlyPrivateKeyAuth' -count=1
```

Expected: FAIL because ET command types are undefined and registration is absent.

- [ ] **Step 3: Add backend command skeleton**

Create `application/commands/et.go` with the skeleton below. It intentionally stops before PTY bridging; later tasks fill the remote runner.

```go
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
	"github.com/Snuffy2/shellport/application/rw"
)

const (
	ETServerRemoteStdOut               = 0x00
	ETServerHookOutputBeforeConnecting = 0x01
	ETServerConnectFailed              = 0x02
	ETServerConnectSucceed             = 0x03
	ETServerConnectVerifyFingerprint   = 0x05
	ETServerConnectRequestCredential   = 0x06
)

const (
	ETClientStdIn              = 0x00
	ETClientResize             = 0x01
	ETClientRespondFingerprint = 0x02
	ETClientRespondCredential  = 0x03
)

const (
	ETRequestErrorBadUserName      = command.StreamError(0x01)
	ETRequestErrorBadRemoteAddress = command.StreamError(0x02)
	ETRequestErrorBadAuthMethod    = command.StreamError(0x03)
	ETRequestErrorUnsupportedProxy = command.StreamError(0x04)
	ETRequestErrorBadMetadata      = command.StreamError(0x05)
)

var (
	ErrETSocks5Unsupported     = errors.New("ET does not support SOCKS5 proxying in this version")
	ErrETUnsupportedAuthMethod = errors.New("ET v1 supports private-key authentication only")
	ErrETRemoteUnavailable     = errors.New("remote ET process is unavailable")
)

type etClient struct {
	w          command.StreamResponder
	l          log.Logger
	hooks      command.Hooks
	cfg        command.Configuration
	bufferPool *command.BufferPool
	metadata   etMetadata

	baseCtx       context.Context
	baseCtxCancel func()
	remoteCloseWait sync.WaitGroup

	credentialReceive              chan []byte
	credentialProcessed            bool
	credentialReceiveCloseOnce     sync.Once
	privateKey                     []byte
	privateKeyLock                 sync.Mutex
	fingerprintVerifyResultReceive chan bool
	fingerprintProcessed           bool
	fingerprintReceiveCloseOnce    sync.Once
}

func newET(l log.Logger, hooks command.Hooks, w command.StreamResponder, cfg command.Configuration, bufferPool *command.BufferPool) command.FSMMachine {
	ctx, cancel := context.WithCancel(context.Background())
	return &etClient{
		w:                                w,
		l:                                l,
		hooks:                            hooks,
		cfg:                              cfg,
		bufferPool:                       bufferPool,
		metadata:                         defaultETMetadata(),
		baseCtx:                          ctx,
		baseCtxCancel:                    sync.OnceFunc(cancel),
		credentialReceive:                make(chan []byte, 1),
		fingerprintVerifyResultReceive:   make(chan bool, 1),
	}
}

func parseETConfig(p configuration.Preset) (configuration.Preset, error) {
	return parseSSHConfig(p)
}

func (d *etClient) validateProxySupport() error {
	if d.cfg.Socks5Configured {
		return ErrETSocks5Unsupported
	}
	return nil
}

func (d *etClient) buildAuthMethod(methodType byte, presetID string, user string, host string) (sshAuthMethodBuilder, error) {
	_ = presetID
	_ = user
	_ = host
	if methodType != SSHAuthMethodPrivateKey {
		return nil, ErrETUnsupportedAuthMethod
	}
	return func(b []byte) []ssh.AuthMethod {
		return []ssh.AuthMethod{
			ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
				privateKey, err := d.requestPrivateKey(b)
				if err != nil {
					return nil, err
				}
				signer, err := ssh.ParsePrivateKey(privateKey)
				if err != nil {
					return nil, err
				}
				d.cachePrivateKey(privateKey)
				return []ssh.Signer{signer}, nil
			}),
		}
	}, nil
}

func (d *etClient) requestPrivateKey(buf []byte) ([]byte, error) {
	if privateKey, ok := d.cachedPrivateKey(); ok {
		return privateKey, nil
	}
	if err := d.w.SendManual(ETServerConnectRequestCredential, buf[d.w.HeaderSize():]); err != nil {
		return nil, err
	}
	privateKeyBytes, received := <-d.credentialReceive
	if !received {
		return nil, ErrSSHAuthCancelled
	}
	d.cachePrivateKey(privateKeyBytes)
	return privateKeyBytes, nil
}

func (d *etClient) cachedPrivateKey() ([]byte, bool) {
	d.privateKeyLock.Lock()
	defer d.privateKeyLock.Unlock()
	if len(d.privateKey) == 0 {
		return nil, false
	}
	privateKey := append([]byte(nil), d.privateKey...)
	return privateKey, true
}

func (d *etClient) cachePrivateKey(privateKey []byte) {
	d.privateKeyLock.Lock()
	defer d.privateKeyLock.Unlock()
	d.privateKey = append(d.privateKey[:0], privateKey...)
}

func (d *etClient) privateKeyForET() ([]byte, error) {
	privateKey, ok := d.cachedPrivateKey()
	if !ok {
		return nil, ErrSSHAuthCancelled
	}
	return privateKey, nil
}

func (d *etClient) Bootup(r *rw.LimitedReader, b []byte) (command.FSMState, command.FSMError) {
	if err := d.validateProxySupport(); err != nil {
		return nil, command.ToFSMError(err, ETRequestErrorUnsupportedProxy)
	}
	return d.local, command.NoFSMError()
}

func (d *etClient) local(_ *command.FSM, _ *rw.LimitedReader, _ command.StreamHeader, _ []byte) error {
	return ErrETRemoteUnavailable
}

func (d *etClient) Close() error {
	d.credentialReceiveCloseOnce.Do(func() { close(d.credentialReceive) })
	d.fingerprintReceiveCloseOnce.Do(func() { close(d.fingerprintVerifyResultReceive) })
	d.baseCtxCancel()
	d.remoteCloseWait.Wait()
	return nil
}

func (d *etClient) Release() error {
	d.baseCtxCancel()
	return nil
}
```

- [ ] **Step 4: Register ET**

Modify `application/commands/commands.go`:

```go
func New() command.Commands {
	return command.Commands{
		command.Register("Telnet", newTelnet, parseTelnetConfig),
		command.Register("SSH", newSSH, parseSSHConfig),
		command.Register("Mosh", newMosh, parseMoshConfig),
		command.Register("ET", newET, parseETConfig),
	}
}
```

- [ ] **Step 5: Run skeleton tests**

Run:

```bash
gofmt -w application/commands/et.go application/commands/et_test.go application/commands/commands.go
go test ./application/commands -run 'TestCommandsIncludesET|TestETRejectsSocks5Proxy|TestETAcceptsOnlyPrivateKeyAuth' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit skeleton slice**

Run:

```bash
git add application/commands/et.go application/commands/et_test.go application/commands/commands.go
git commit -m "feat: register ET backend command"
```

Expected: commit contains ET skeleton and registration.

## Task 5: Implement Backend ET Bootup and SSH Validation

**Files:**
- Modify: `application/commands/et.go`
- Modify: `application/commands/et_test.go`

- [ ] **Step 1: Add failing bootup tests**

Append to `application/commands/et_test.go`:

```go
func TestETBootupRejectsPasswordAuth(t *testing.T) {
	client := &etClient{
		bufferPool: command.NewBufferPool(4096),
		baseCtx: context.Background(),
	}
	payload := buildETBootupPayload(t, "alice", "example.com", 22, SSHAuthMethodPassphrase, "2022", "et", "")
	_, fsmErr := client.Bootup(newLimitedReader(payload), make([]byte, 4096))
	if fsmErr.Err == nil || fsmErr.StreamErr != ETRequestErrorBadAuthMethod {
		t.Fatalf("Bootup() FSM error = %#v, want bad auth method", fsmErr)
	}
}

func TestETBootupParsesMetadataAndPresetID(t *testing.T) {
	client := &etClient{
		bufferPool: command.NewBufferPool(4096),
		baseCtx: context.Background(),
		baseCtxCancel: func() {},
		credentialReceive: make(chan []byte, 1),
		fingerprintVerifyResultReceive: make(chan bool, 1),
		remoteStarter: func(user string, address string, auth sshAuthMethodBuilder, metadata etMetadata, presetID string) {},
	}
	payload := buildETBootupPayload(t, "alice", "example.com", 22, SSHAuthMethodPrivateKey, "22022", "/usr/local/bin/et", "preset-et")
	_, fsmErr := client.Bootup(newLimitedReader(payload), make([]byte, 4096))
	if fsmErr.Err != nil {
		t.Fatalf("Bootup() error = %v", fsmErr.Err)
	}
	if client.metadata.ServerPort != 22022 {
		t.Fatalf("ServerPort = %d, want 22022", client.metadata.ServerPort)
	}
	if client.metadata.Command != "/usr/local/bin/et" {
		t.Fatalf("Command = %q, want /usr/local/bin/et", client.metadata.Command)
	}
}
```

Also add this helper in the test file:

```go
func buildETBootupPayload(t *testing.T, user string, host string, sshPort uint16, auth byte, etPort string, etCommand string, presetID string) []byte {
	t.Helper()

	payload := make([]byte, 0, 256)
	buf := make([]byte, 512)

	userLen, err := NewString([]byte(user)).Marshal(buf)
	if err != nil {
		t.Fatalf("marshal user: %v", err)
	}
	payload = append(payload, buf[:userLen]...)

	addrLen, err := NewAddress(HostNameAddr, []byte(host), sshPort).Marshal(buf)
	if err != nil {
		t.Fatalf("marshal address: %v", err)
	}
	payload = append(payload, buf[:addrLen]...)
	payload = append(payload, auth)
	payload = appendETString(t, payload, etPort)
	payload = appendETString(t, payload, etCommand)
	payload = appendETString(t, payload, presetID)
	return payload
}

func appendETString(t *testing.T, payload []byte, value string) []byte {
	t.Helper()

	buf := make([]byte, MaxInteger+MaxIntegerBytes+len(value))
	valueLen, err := NewString([]byte(value)).Marshal(buf)
	if err != nil {
		t.Fatalf("marshal string %q: %v", value, err)
	}
	return append(payload, buf[:valueLen]...)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./application/commands -run 'TestETBootup' -count=1
```

Expected: FAIL because `Bootup` does not parse the payload and `remoteStarter` is missing.

- [ ] **Step 3: Implement Bootup parsing**

Modify `etClient` in `application/commands/et.go` to add:

```go
remoteStarter func(user string, address string, auth sshAuthMethodBuilder, metadata etMetadata, presetID string)
```

Replace `Bootup` with:

```go
func (d *etClient) Bootup(r *rw.LimitedReader, b []byte) (command.FSMState, command.FSMError) {
	if err := d.validateProxySupport(); err != nil {
		return nil, command.ToFSMError(err, ETRequestErrorUnsupportedProxy)
	}

	sBuf := d.bufferPool.Get()
	defer d.bufferPool.Put(sBuf)

	userName, userNameErr := ParseString(r.Read, (*sBuf)[:moshMaxUsernameLen])
	if userNameErr != nil {
		return nil, command.ToFSMError(userNameErr, ETRequestErrorBadUserName)
	}
	userNameStr := string(userName.Data())

	addr, addrErr := ParseAddress(r.Read, (*sBuf)[:moshMaxHostnameLen])
	if addrErr != nil {
		return nil, command.ToFSMError(addrErr, ETRequestErrorBadRemoteAddress)
	}
	addrStr := addr.String()
	if addrStr == "" {
		return nil, command.ToFSMError(ErrSSHInvalidAddress, ETRequestErrorBadRemoteAddress)
	}

	authData, authErr := rw.FetchOneByte(r.Fetch)
	if authErr != nil {
		return nil, command.ToFSMError(authErr, ETRequestErrorBadAuthMethod)
	}

	metadata, metadataErr := parseETMetadata(r, (*sBuf)[:])
	if metadataErr != nil {
		return nil, command.ToFSMError(metadataErr, ETRequestErrorBadMetadata)
	}
	d.metadata = metadata

	presetID, presetIDErr := parseOptionalPresetID(r, (*sBuf)[:configuration.MaxPresetIDLength])
	if presetIDErr != nil {
		return nil, command.ToFSMError(presetIDErr, ETRequestErrorBadMetadata)
	}

	authMethodBuilder, authMethodErr := d.buildAuthMethod(authData[0], presetID, userNameStr, addrStr)
	if authMethodErr != nil {
		return nil, command.ToFSMError(authMethodErr, ETRequestErrorBadAuthMethod)
	}

	d.remoteCloseWait.Add(1)
	if d.remoteStarter != nil {
		go d.remoteStarter(userNameStr, addrStr, authMethodBuilder, metadata, presetID)
	} else {
		go d.remote(userNameStr, addrStr, authMethodBuilder, metadata, presetID)
	}
	return d.local, command.NoFSMError()
}
```

Add a temporary `remote` stub below `Bootup`; Task 6 replaces it:

```go
func (d *etClient) remote(user string, address string, authMethodBuilder sshAuthMethodBuilder, metadata etMetadata, presetID string) {
	defer d.remoteCloseWait.Done()
	d.baseCtxCancel()
	_ = user
	_ = address
	_ = authMethodBuilder
	_ = metadata
	_ = presetID
}
```

- [ ] **Step 4: Run bootup tests**

Run:

```bash
gofmt -w application/commands/et.go application/commands/et_test.go
go test ./application/commands -run 'TestETBootup|TestETAcceptsOnlyPrivateKeyAuth' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit bootup slice**

Run:

```bash
git add application/commands/et.go application/commands/et_test.go
git commit -m "feat: parse ET command bootup"
```

Expected: commit contains ET bootup parsing and tests.

## Task 6: Implement ET PTY Runner and Local Frame Bridge

**Files:**
- Modify: `application/commands/et.go`
- Modify: `application/commands/et_process.go`
- Modify: `application/commands/et_test.go`

- [ ] **Step 1: Add failing PTY bridge tests**

Append to `application/commands/et_test.go`:

```go
type fakeETProcess struct {
	stdin   bytes.Buffer
	stdout  chan []byte
	resizes []struct{ rows, cols uint16 }
	closed  bool
}

func (f *fakeETProcess) Read(p []byte) (int, error) {
	data, ok := <-f.stdout
	if !ok {
		return 0, io.EOF
	}
	return copy(p, data), nil
}

func (f *fakeETProcess) Write(p []byte) (int, error) {
	return f.stdin.Write(p)
}

func (f *fakeETProcess) Resize(rows uint16, cols uint16) error {
	f.resizes = append(f.resizes, struct{ rows, cols uint16 }{rows: rows, cols: cols})
	return nil
}

func (f *fakeETProcess) Close() error {
	f.closed = true
	close(f.stdout)
	return nil
}

func TestETLocalWritesStdinAndResize(t *testing.T) {
	proc := &fakeETProcess{stdout: make(chan []byte, 1)}
	client := &etClient{process: proc}

	stdinHeader := command.StreamHeader{}
	stdinHeader.Set(ETClientStdIn, 5)
	if err := client.local(nil, newLimitedReader([]byte("hello")), stdinHeader, make([]byte, 16)); err != nil {
		t.Fatalf("stdin local error = %v", err)
	}
	if got := proc.stdin.String(); got != "hello" {
		t.Fatalf("stdin = %q, want hello", got)
	}

	resizePayload := []byte{0, 40, 0, 120}
	resizeHeader := command.StreamHeader{}
	resizeHeader.Set(ETClientResize, uint16(len(resizePayload)))
	if err := client.local(nil, newLimitedReader(resizePayload), resizeHeader, make([]byte, 16)); err != nil {
		t.Fatalf("resize local error = %v", err)
	}
	if len(proc.resizes) != 1 || proc.resizes[0].rows != 40 || proc.resizes[0].cols != 120 {
		t.Fatalf("resizes = %#v, want 40x120", proc.resizes)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./application/commands -run 'TestETLocalWritesStdinAndResize' -count=1
```

Expected: FAIL because `etClient.process`, `etProcess`, and frame handling are undefined.

- [ ] **Step 3: Add ET process interface and PTY implementation**

In `application/commands/et_process.go`, add:

```go
import (
	"context"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

type etProcess interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Resize(rows uint16, cols uint16) error
	Close() error
}

type etPTYProcess struct {
	cmd  *exec.Cmd
	file *os.File
}

func startETPTY(ctx context.Context, metadata etMetadata, user string, address string, sshConfigPath string) (etProcess, error) {
	args := buildETClientArgs(metadata, user, address, sshConfigPath)
	cmd := exec.CommandContext(ctx, metadata.Command, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	file, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	return &etPTYProcess{cmd: cmd, file: file}, nil
}

func (p *etPTYProcess) Read(b []byte) (int, error) {
	return p.file.Read(b)
}

func (p *etPTYProcess) Write(b []byte) (int, error) {
	return p.file.Write(b)
}

func (p *etPTYProcess) Resize(rows uint16, cols uint16) error {
	return pty.Setsize(p.file, &pty.Winsize{Rows: rows, Cols: cols})
}

func (p *etPTYProcess) Close() error {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
	}
	if p.file != nil {
		_ = p.file.Close()
	}
	if p.cmd != nil {
		_ = p.cmd.Wait()
	}
	return nil
}
```

Ensure the final import block contains `context`, `fmt`, `net`, `os`, `os/exec`, `path/filepath`, `strings`, and `syscall`.

- [ ] **Step 4: Add local frame bridge**

In `etClient`, add:

```go
process     etProcess
processLock sync.Mutex
```

Replace `local` with:

```go
func (d *etClient) local(_ *command.FSM, r *rw.LimitedReader, h command.StreamHeader, b []byte) error {
	switch h.Marker() {
	case ETClientStdIn:
		process, ok := d.getProcessIfReady()
		for !r.Completed() {
			data, err := r.Buffered()
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			if _, err := process.Write(data); err != nil {
				_ = process.Close()
				return err
			}
		}
		return nil
	case ETClientResize:
		process, ok := d.getProcessIfReady()
		if !ok {
			return nil
		}
		if _, err := io.ReadFull(r, b[:4]); err != nil {
			return err
		}
		rows := uint16(b[0])<<8 | uint16(b[1])
		cols := uint16(b[2])<<8 | uint16(b[3])
		return process.Resize(rows, cols)
	default:
		return ErrSSHUnknownClientSignal
	}
}

func (d *etClient) getProcessIfReady() (etProcess, bool) {
	d.processLock.Lock()
	defer d.processLock.Unlock()
	if d.process == nil {
		return nil, false
	}
	return d.process, true
}

func (d *etClient) cacheProcess(process etProcess) {
	d.processLock.Lock()
	defer d.processLock.Unlock()
	d.process = process
}
```

Add `io` to the `application/commands/et.go` import block.

- [ ] **Step 5: Run PTY bridge tests**

Run:

```bash
gofmt -w application/commands/et.go application/commands/et_process.go application/commands/et_test.go
go test ./application/commands -run 'TestETLocalWritesStdinAndResize|TestBuildETClientArgs|TestWriteETSSHMaterial' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit PTY bridge slice**

Run:

```bash
git add application/commands/et.go application/commands/et_process.go application/commands/et_test.go go.mod go.sum
git commit -m "feat: bridge ET PTY process frames"
```

Expected: commit contains PTY bridge and tests.

## Task 7: Implement Full Backend Remote Flow

**Files:**
- Modify: `application/commands/et.go`
- Modify: `application/commands/et_test.go`
- Modify: `application/commands/connection_logging.go`

- [ ] **Step 1: Add remote-flow test hooks**

Append to `application/commands/et.go`:

```go
type etProcessStarter func(ctx context.Context, metadata etMetadata, user string, address string, sshConfigPath string) (etProcess, error)
```

Add this field to `etClient`:

```go
processStarter etProcessStarter
```

Set it in `newET`:

```go
processStarter: startETPTY,
```

- [ ] **Step 2: Add failing remote process-start test**

Append to `application/commands/et_test.go`:

```go
func TestETRemoteStartsProcessWithValidatedMaterial(t *testing.T) {
	started := make(chan string, 1)
	proc := &fakeETProcess{stdout: make(chan []byte, 1)}
	proc.stdout <- []byte("connected\n")
	client := &etClient{
		w: command.StreamResponder{},
		l: log.NewDitch(),
		hooks: command.NewHooks(configuration.HookSettings{}),
		cfg: command.Configuration{DialTimeout: time.Second},
		bufferPool: command.NewBufferPool(4096),
		baseCtx: context.Background(),
		baseCtxCancel: func() {},
		processStarter: func(ctx context.Context, metadata etMetadata, user string, address string, sshConfigPath string) (etProcess, error) {
			if user != "alice" {
				t.Fatalf("user = %q, want alice", user)
			}
			if metadata.ServerPort != 2022 {
				t.Fatalf("server port = %d, want 2022", metadata.ServerPort)
			}
			started <- sshConfigPath
			return proc, nil
		},
	}
	client.cacheProcess(proc)
	configPath := filepath.Join(t.TempDir(), "ssh_config")
	if err := os.WriteFile(configPath, []byte("Host *\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	process, err := client.processStarter(context.Background(), defaultETMetadata(), "alice", "example.com:22", configPath)
	if err != nil {
		t.Fatalf("processStarter error = %v", err)
	}
	if process == nil {
		t.Fatal("processStarter returned nil process")
	}
	if got := <-started; got != configPath {
		t.Fatalf("ssh config path = %q, want %q", got, configPath)
	}
}
```

- [ ] **Step 3: Run tests to verify compile failure or missing imports**

Run:

```bash
go test ./application/commands -run 'TestETRemoteStartsProcessWithValidatedMaterial' -count=1
```

Expected: FAIL until missing imports/fields are added and remote flow is implemented.

- [ ] **Step 4: Implement remote flow**

Replace the temporary `remote` stub in `application/commands/et.go` with:

```go
func (d *etClient) remote(user string, address string, authMethodBuilder sshAuthMethodBuilder, metadata etMetadata, presetID string) {
	u := d.bufferPool.Get()
	defer d.bufferPool.Put(u)

	details := connectionDebugDetails{
		Protocol: "ET",
		User: user,
		Address: address,
		Network: "tcp",
		AuthMethod: "private_key",
		PresetID: presetID,
	}
	debugConnectionAttempt(d.l, details)

	var process etProcess
	tempDir := ""
	defer func() {
		if process != nil {
			_ = process.Close()
		}
		if err := cleanupETTempDir(tempDir); err != nil {
			d.l.Warning("Failed to clean ET temp directory: %s", err)
		}
		d.w.Signal(command.HeaderClose)
		d.baseCtxCancel()
		d.remoteCloseWait.Done()
	}()

	if err := d.hooks.Run(
		d.baseCtx,
		configuration.HOOK_BEFORE_CONNECTING,
		command.NewHookParameters(2).Insert("Remote Type", "ET").Insert("Remote Address", address),
		command.NewDefaultHookOutput(d.l, func(b []byte) (int, error) {
			dLen := copy((*u)[d.w.HeaderSize():], b) + d.w.HeaderSize()
			return len(b), d.w.SendManual(ETServerHookOutputBeforeConnecting, (*u)[:dLen])
		}),
	); err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}

	fingerprint := ""
	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: authMethodBuilder((*u)[:]),
		HostKeyCallback: func(h string, r net.Addr, k ssh.PublicKey) error {
			fingerprint = ssh.FingerprintSHA256(k)
			return d.confirmRemoteFingerprint(h, r, k, (*u)[:])
		},
		Timeout: d.cfg.DialTimeout,
	}
	conn, clearConnInitialDeadline, err := d.dialRemote("tcp", address, sshConfig)
	if err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}
	clearConnInitialDeadline()
	_ = conn.Close()

	privateKey, err := d.privateKeyForET()
	if err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}

	tempDir, err = os.MkdirTemp("", "shellport-et-*")
	if err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}
	host := moshRemoteHost(address)
	material, err := writeETSSHMaterial(tempDir, privateKey, host, fingerprint)
	if err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}

	process, err = d.processStarter(d.baseCtx, metadata, user, address, material.ConfigPath)
	if err != nil {
		d.sendConnectFailed((*u)[:], err)
		debugConnectionFailed(d.l, details, err)
		return
	}
	d.cacheProcess(process)
	if err := d.w.SendManual(ETServerConnectSucceed, (*u)[:d.w.HeaderSize()]); err != nil {
		return
	}
	debugConnectionEstablished(d.l, details)

	for {
		rLen, rErr := process.Read((*u)[d.w.HeaderSize():])
		if rErr != nil {
			debugConnectionDisconnected(d.l, details, "process output ended", rErr)
			return
		}
		if err := d.w.SendManual(ETServerRemoteStdOut, (*u)[:d.w.HeaderSize()+rLen]); err != nil {
			debugConnectionDisconnected(d.l, details, "client send failed", err)
			return
		}
	}
}
```

Add the remaining helper methods. The private-key cache helpers were added in Task 4, so do not duplicate them here.

```go
func (d *etClient) confirmRemoteFingerprint(hostname string, remote net.Addr, key ssh.PublicKey, buf []byte) error {
	fingerprint := ssh.FingerprintSHA256(key)
	fgpLen := copy(buf[d.w.HeaderSize():], fingerprint)
	if err := d.w.SendManual(ETServerConnectVerifyFingerprint, buf[:d.w.HeaderSize()+fgpLen]); err != nil {
		return err
	}
	confirmed, ok := <-d.fingerprintVerifyResultReceive
	if !ok {
		return ErrSSHRemoteFingerprintVerificationCancelled
	}
	if !confirmed {
		return ErrSSHRemoteFingerprintRefused
	}
	return nil
}

func (d *etClient) sendConnectFailed(buf []byte, err error) {
	errLen := copy(buf[d.w.HeaderSize():], err.Error()) + d.w.HeaderSize()
	d.w.SendManual(ETServerConnectFailed, buf[:errLen])
}
```

Copy `dialRemote` from `moshClient` or factor a shared helper only if the final code stays smaller and clear. The ET version must use ShellPort `cfg.Dial`, `network.NewWriteTimeoutConn`, retry deadlines during credential/fingerprint exchange, and no SOCKS5.

- [ ] **Step 5: Add ET expected disconnect classification**

Modify `application/commands/connection_logging.go`:

```go
func expectedDisconnectError(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, ErrMoshSessionClosed) ||
		errors.Is(err, ErrETRemoteUnavailable) ||
		errors.Is(err, net.ErrClosed)
}
```

- [ ] **Step 6: Run backend tests**

Run:

```bash
gofmt -w application/commands/et.go application/commands/et_test.go application/commands/connection_logging.go
go test ./application/commands -run 'TestET|TestCommandsIncludesET' -count=1
```

Expected: PASS. If the remote-flow test requires too much live SSH setup, keep it focused on injected helpers and add a separate unit test for each helper instead of dialing a network.

- [ ] **Step 7: Commit backend remote flow**

Run:

```bash
git add application/commands/et.go application/commands/et_test.go application/commands/connection_logging.go
git commit -m "feat: launch ET with ShellPort auth"
```

Expected: commit contains ET remote flow and tests.

## Task 8: Add Frontend ET Command

**Files:**
- Create: `ui/commands/et.js`
- Create: `ui/commands/et_test.js`
- Modify: `ui/commands/commands_test.js` if command count/order is asserted

- [ ] **Step 1: Write failing frontend command tests**

Create `ui/commands/et_test.js`:

```js
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import * as reader from "../stream/reader.js";
import * as address from "./address.js";
import * as et from "./et.js";
import * as strings from "./string.js";

describe("ET Command", () => {
  function makeReader(buffer) {
    const rd = new reader.Reader(new reader.Multiple(() => {}), (data) => data);
    rd.feed(buffer);
    return rd;
  }

  it("uses the ET command id", () => {
    assert.strictEqual(new et.Command().id(), 0x03);
  });

  it("includes ET metadata in the initial payload", async () => {
    let sent = null;
    const handler = new et.ET(null, {
      user: new TextEncoder().encode("alice"),
      host: address.parseHostPort("example.com:22", 22),
      auth: 0x02,
      charset: "utf-8",
      etServerPort: "22022",
      etCommand: "/usr/local/bin/et",
      presetID: "preset-et",
    }, {
      "initialization.failed"() {},
      initialized() {},
      "hook.before_connected"() {},
      "connect.failed"() {},
      "connect.succeed"() {},
      "connect.fingerprint"() {},
      "connect.credential"() {},
      "@stdout"() {},
      close() {},
      "@completed"() {},
    });

    handler.run({ send(data) { sent = data; } });
    const rd = makeReader(sent);
    assert.deepStrictEqual((await strings.String.read(rd)).data(), new TextEncoder().encode("alice"));
    assert.strictEqual((await address.Address.read(rd)).port(), 22);
    assert.strictEqual((await reader.readOne(rd))[0], 0x02);
    assert.deepStrictEqual((await strings.String.read(rd)).data(), new TextEncoder().encode("22022"));
    assert.deepStrictEqual((await strings.String.read(rd)).data(), new TextEncoder().encode("/usr/local/bin/et"));
    assert.deepStrictEqual((await strings.String.read(rd)).data(), new TextEncoder().encode("preset-et"));
  });

  it("validates ET server port and command fields", () => {
    const wizard = new et.Command().wizard(null, null, {}, [], null, null, { get() { return {}; } }, null);
    const fields = wizard.stepInitialPrompt().data().inputs;
    const port = fields.find((field) => field.name === "ET Server Port");
    const command = fields.find((field) => field.name === "ET Command");
    assert.strictEqual(port.verify("2022"), "Will connect to etserver port 2022");
    assert.throws(() => port.verify("0"), /between 1 and 65535/);
    assert.throws(() => port.verify("abc"), /numeric/);
    assert.strictEqual(command.verify("et"), "Will run et");
    assert.throws(() => command.verify("et --flag"), /without arguments/);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
npm exec vitest run ui/commands/et_test.js
```

Expected: FAIL because `ui/commands/et.js` does not exist.

- [ ] **Step 3: Implement ET command module**

Create `ui/commands/et.js` by copying `ui/commands/mosh.js`, then make these exact changes:

```js
const COMMAND_ID = 0x03;
const DEFAULT_ET_SERVER_PORT = "2022";
const DEFAULT_ET_COMMAND = "et";
```

Rename exported command handler class to:

```js
export class ET {
  constructor(sd, config, callbacks) {
    this.sender = sd;
    this.config = config;
    this.connected = false;
    this.events = new event.Events([
      "initialization.failed",
      "initialized",
      "hook.before_connected",
      "connect.failed",
      "connect.succeed",
      "connect.fingerprint",
      "connect.credential",
      "@stdout",
      "close",
      "@completed",
    ], callbacks);
  }
}
```

In `ET.run`, encode this payload order:

```js
user, host, authMethod, etServerPort, etCommand, presetID
```

Use:

```js
const etServerPort = new strings.String(common.strToUint8Array(this.config.etServerPort || DEFAULT_ET_SERVER_PORT));
const etCommand = new strings.String(common.strToUint8Array(this.config.etCommand || DEFAULT_ET_COMMAND));
```

In the wizard fields, include:

```js
"ET Server Port": {
  name: "ET Server Port",
  description: "Remote etserver TCP port",
  type: "text",
  value: DEFAULT_ET_SERVER_PORT,
  example: DEFAULT_ET_SERVER_PORT,
  readonly: false,
  suggestions() { return []; },
  verify(d) {
    if (!/^[0-9]+$/.test(d)) {
      throw new Error("ET Server Port must be numeric");
    }
    const port = Number.parseInt(d, 10);
    if (port < 1 || port > 65535) {
      throw new Error("ET Server Port must be between 1 and 65535");
    }
    return "Will connect to etserver port " + port;
  },
},
"ET Command": {
  name: "ET Command",
  description: "Local ET client command path",
  type: "text",
  value: DEFAULT_ET_COMMAND,
  example: DEFAULT_ET_COMMAND,
  readonly: false,
  suggestions() { return []; },
  verify(d) {
    if (d.length <= 0) {
      throw new Error("ET Command must be specified");
    }
    if (/\s/.test(d)) {
      throw new Error("ET Command must be an executable path without arguments");
    }
    return "Will run " + d;
  },
},
```

Limit auth validation to private key:

```js
function getAuthMethodFromStr(d) {
  if (d === "Private Key") {
    return AUTHMETHOD_PRIVATE_KEY;
  }
  throw new Exception("ET v1 supports Private Key authentication only");
}
```

Map initialization errors:

```js
case SERVER_REQUEST_ERROR_UNSUPPORTED_PROXY:
  self.step.resolve(self.stepErrorDone("Request failed", "ET does not support SOCKS5 proxying in this version"));
  return;
case SERVER_REQUEST_ERROR_BAD_METADATA:
  self.step.resolve(self.stepErrorDone("Request failed", "Invalid ET metadata"));
  return;
```

- [ ] **Step 4: Run frontend command tests**

Run:

```bash
npm exec vitest run ui/commands/et_test.js
```

Expected: PASS.

- [ ] **Step 5: Commit frontend command**

Run:

```bash
git add ui/commands/et.js ui/commands/et_test.js
git commit -m "feat: add ET frontend command"
```

Expected: commit contains ET command and tests.

## Task 9: Add ET Control and App Registration

**Files:**
- Create: `ui/control/et.js`
- Create: `ui/control/et_test.js`
- Modify: `ui/app.js`

- [ ] **Step 1: Write failing ET control test**

Create `ui/control/et_test.js` by copying `ui/control/mosh_test.js` and replacing imports/names with ET:

```js
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

import assert from "assert";
import * as reader from "../stream/reader.js";
import * as et from "./et.js";

describe("ET Control", () => {
  function buildControl() {
    const events = {};
    const sent = [];
    const resizes = [];
    const closes = [];
    const forgotten = [];
    const control = new et.ET({
      get() {
        return {
          forget() { forgotten.push("forget"); },
          hex() { return "#000000"; },
        };
      },
    }).build({
      charset: "utf-8",
      close() { closes.push("close"); },
      events: { place(name, callback) { events[name] = callback; } },
      resize(rows, cols) { resizes.push({ rows, cols }); },
      send(data) { sent.push(Array.from(data)); },
      tabColor: "",
    });
    return { control, events, sent, resizes, closes, forgotten };
  }

  it("decodes stdout and encodes stdin through utf-8", async () => {
    const { control, events, sent } = buildControl();
    await events.stdout(new reader.Buffer(new TextEncoder().encode("hello"), () => {}));
    assert.strictEqual(await control.receive(), "hello");
    control.send("ok");
    assert.deepStrictEqual(sent, [[0x6f, 0x6b]]);
  });

  it("closes once and rejects after completion", async () => {
    const { control, events, closes, forgotten } = buildControl();
    control.close();
    control.close();
    assert.deepStrictEqual(closes, ["close"]);
    await events.completed();
    assert.deepStrictEqual(forgotten, ["forget"]);
    await assert.rejects(async () => control.receive(), /Remote connection has been terminated/);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
npm exec vitest run ui/control/et_test.js
```

Expected: FAIL because `ui/control/et.js` does not exist.

- [ ] **Step 3: Implement ET control**

Create `ui/control/et.js` by copying `ui/control/mosh.js`, then rename exported class to `ET` and update `type()`:

```js
export class ET {
  constructor(c) {
    this.colors = c;
  }

  type() {
    return "ET";
  }

  ui() {
    return "Console";
  }

  build(data) {
    return new Control(data, this.colors.get(data.tabColor));
  }
}
```

- [ ] **Step 4: Register ET in app**

Modify `ui/app.js` imports:

```js
import * as et from "./commands/et.js";
import * as etctl from "./control/et.js";
```

Add ET to controls:

```js
new etctl.ET(uiControlColors),
```

Add ET to commands:

```js
new et.Command(),
```

- [ ] **Step 5: Run ET frontend tests**

Run:

```bash
npm exec vitest run ui/control/et_test.js ui/commands/et_test.js
```

Expected: PASS.

- [ ] **Step 6: Commit control registration**

Run:

```bash
git add ui/control/et.js ui/control/et_test.js ui/app.js
git commit -m "feat: register ET frontend control"
```

Expected: commit contains ET control and app registration.

## Task 10: Add ET Preset Direct Launch

**Files:**
- Modify: `ui/home_preset_execution.js`
- Modify: `ui/home_preset_execution_test.js`
- Modify: `shellport.conf.example.json`

- [ ] **Step 1: Write failing preset tests**

Append to `ui/home_preset_execution_test.js`:

```js
it("builds direct ET execution for private-key presets", () => {
  const execution = buildPresetExecution(
    mergedPreset("ET", {
      title: "Example ET",
      type: "ET",
      host: "example.com:22",
      id: "preset-et",
      meta: {
        User: "alice",
        Authentication: "Private Key",
        "Private Key": "PRIVATE KEY DATA",
        Fingerprint: "SHA256:abc",
        "ET Server Port": "22022",
        "ET Command": "/usr/local/bin/et",
      },
    }),
  );

  assert.strictEqual(execution.config.etServerPort, "22022");
  assert.strictEqual(execution.config.etCommand, "/usr/local/bin/et");
  assert.strictEqual(execution.session.credential, "PRIVATE KEY DATA");
  assert.deepStrictEqual(execution.keptSessions, ["credential"]);
});

it("does not direct-launch ET password presets", () => {
  const execution = buildPresetExecution(
    mergedPreset("ET", {
      title: "Example ET Password",
      type: "ET",
      host: "example.com:22",
      meta: {
        User: "alice",
        Authentication: "Password",
        Password: "secret",
      },
    }),
  );
  assert.strictEqual(execution, null);
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
npm exec vitest run ui/home_preset_execution_test.js
```

Expected: FAIL because ET direct-launch is not supported.

- [ ] **Step 3: Implement preset direct launch**

Modify `ui/home_preset_execution.js`:

```js
if (commandName !== "SSH" && commandName !== "Mosh" && commandName !== "ET") {
  return null;
}
```

After credential calculation, add:

```js
if (commandName === "ET" && authentication !== "Private Key") {
  return null;
}
```

After the Mosh metadata block, add:

```js
if (commandName === "ET") {
  config.etServerPort = presetData.metaDefault("ET Server Port", "2022");
  config.etCommand = presetData.metaDefault("ET Command", "et");
}
```

- [ ] **Step 4: Add example ET preset**

Modify `shellport.conf.example.json` by adding this object after the Mosh preset:

```json
{
  "ID": "preset-example-et",
  "Title": "Example ET",
  "Type": "ET",
  "Host": "ssh.example.com:22",
  "Meta": {
    "User": "guest",
    "Authentication": "Private Key",
    "Encoding": "utf-8",
    "ET Server Port": "2022",
    "ET Command": "et"
  }
}
```

Ensure JSON commas are valid.

- [ ] **Step 5: Run preset tests**

Run:

```bash
npm exec vitest run ui/home_preset_execution_test.js
```

Expected: PASS.

- [ ] **Step 6: Commit preset slice**

Run:

```bash
git add ui/home_preset_execution.js ui/home_preset_execution_test.js shellport.conf.example.json
git commit -m "feat: support ET preset launch"
```

Expected: commit contains preset execution and example config.

## Task 11: Add Docker Runtime Dependencies and Docs

**Files:**
- Modify: `Dockerfile`
- Modify: `README.md`
- Modify: `CONFIGURATION.md`

- [ ] **Step 1: Verify Alpine package names**

Run:

```bash
docker run --rm alpine:3.23 sh -lc 'apk update >/dev/null && apk search eternal terminal et | head -50'
docker run --rm alpine:3.23 sh -lc 'apk update >/dev/null && apk info openssh-client >/dev/null && echo openssh-client-ok'
```

Expected: identify the exact package that provides `et`, or confirm Alpine has no package. `openssh-client-ok` confirms OpenSSH client availability.

- [ ] **Step 2: Modify Dockerfile based on verified packages**

If Alpine provides ET as `eternal-terminal`, change the runtime `apk add` line to:

```dockerfile
    apk add --no-cache openssh-client eternal-terminal tzdata && \
```

If Alpine does not provide ET, add a builder stage for ET from source before the runtime stage and copy only the client binary into runtime. The runtime `apk add` line must still include:

```dockerfile
    apk add --no-cache openssh-client tzdata && \
```

Do not guess package names; use Step 1 output.

- [ ] **Step 3: Document ET support in README**

Add this paragraph near the Mosh support paragraph:

```markdown
ET support is available for private-key authentication. ShellPort verifies the SSH host fingerprint and private key before launching the local `et` client, then proxies the ET client PTY over the existing browser WebSocket. ET uses the remote `etserver` TCP port, defaulting to `2022`. SOCKS5 proxying and password authentication are not supported for ET v1. Closing the ShellPort browser session terminates the backend `et` client process, matching the current Mosh-style session lifetime.
```

- [ ] **Step 4: Document ET preset metadata in CONFIGURATION**

Add an ET metadata section:

```markdown
### ET Presets

ET presets use the same `Host`, `User`, `Authentication`, `Private Key`,
`Fingerprint`, and `Encoding` metadata as SSH private-key presets. ET v1
requires `Authentication` to be `Private Key`.

ET-specific metadata:

- `ET Server Port`: remote `etserver` TCP port. Defaults to `2022`.
- `ET Command`: local ET client executable path inside the ShellPort runtime.
  Defaults to `et`. Arguments are not accepted.

ET v1 does not support password authentication or SOCKS5 proxying.
```

- [ ] **Step 5: Commit Docker/docs slice**

Run:

```bash
git add Dockerfile README.md CONFIGURATION.md
git commit -m "docs: document ET runtime requirements"
```

Expected: commit contains Docker/runtime docs only.

## Task 12: Final Validation

**Files:**
- Validate all changed files

- [ ] **Step 1: Run backend targeted tests**

Run:

```bash
go test ./application/commands -run 'TestET|TestCommandsIncludesET|TestBuildET|TestWriteET|TestCleanupET' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run frontend targeted tests**

Run:

```bash
npm exec vitest run ui/commands/et_test.js ui/control/et_test.js ui/home_preset_execution_test.js
```

Expected: PASS.

- [ ] **Step 3: Run full Go race suite**

Run:

```bash
go test ./... -race -timeout 30s
```

Expected: PASS.

- [ ] **Step 4: Run full project test command**

Run:

```bash
npm test
```

Expected: PASS. This runs generation before `testonly`.

- [ ] **Step 5: Run build**

Run:

```bash
npm run build
```

Expected: PASS and produces the `shellport` binary.

- [ ] **Step 6: Run Docker build if Dockerfile changed**

Run:

```bash
docker build -t shellport-et:dev .
```

Expected: PASS and the runtime image contains `/shellport`; if CLI fallback was used, verify `et` is available:

```bash
docker run --rm shellport-et:dev sh -lc 'command -v et && command -v ssh'
```

Expected: both commands print paths.

- [ ] **Step 7: Run hook parity**

Run:

```bash
./.venv/bin/prek run --all-files
```

If `.venv/bin/prek` is missing, run:

```bash
uv venv .venv
uv pip install --python ./.venv/bin/python prek
./.venv/bin/prek run --all-files
```

Expected: PASS, or report exact remaining failures without hiding them behind exclusions.

- [ ] **Step 8: Commit validation fixes only if needed**

If validation required code or docs fixes, run:

```bash
git add application/commands/et.go application/commands/et_test.go ui/commands/et.js ui/commands/et_test.js README.md CONFIGURATION.md Dockerfile shellport.conf.example.json
git commit -m "fix: stabilize ET validation"
```

Expected: final branch has focused commits and no unrelated changes.
