# kairos-installer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A standalone `kairos-installer` binary that renders the existing interactive TUI and drives installation by exec'ing `kairos-agent manual-install`, rendering progress from the agent's JSON-Lines events.

**Architecture:** One TUI package (`internal/tui`) holds the moved bubbletea code (shared `mainModel`/`Page`/colors), config-shaping (Model → `#cloud-config` string), branding helpers, and the install page. A decoupled `internal/agentrun` package owns the subprocess exec + JSON-Lines parsing (no TUI deps). `internal/bus` is the copied provider event bus. `main.go` parses `--source` and launches the program. The module imports only kairos-sdk, the charmbracelet libs, go-pluggable, yip, ghw, and yaml — never kairos-agent.

**Tech Stack:** Go 1.26, bubbletea/lipgloss/bubbles, kairos-sdk, mudler/go-pluggable, mudler/yip, jaypipes/ghw, ginkgo/gomega (tests).

**Refinements vs the spec (`docs/superpowers/specs/2026-06-10-kairos-installer-design.md`):**
- Branding + config helpers are files in `internal/tui`, not separate packages (the TUI code is one tightly-coupled package).
- Config-shaping emits a `#cloud-config` **string** and the install drives `manual-install --use-default-dirs`, so the agent merges system userdata — dropping the `collector`/`GetUserConfigDirs` dependency.

**Source reference:** all "move/copy from agent" steps refer to `/home/mudler/_git/kairos-agent/internal/agent/` (and `pkg/`, `internal/kairos`) at commit `01d7564`.

---

## File Structure

| File | Responsibility | New/Moved/Copied |
|---|---|---|
| `go.mod`, `go.sum` | module `github.com/kairos-io/kairos-installer`, Go 1.26 | New |
| `.gitignore` | ignore binary + test artifacts | New |
| `main.go` | flags (`--source`, `--shell`), launch tea program | New |
| `internal/agentrun/agentrun.go` | resolve agent bin, build `manual-install` cmd, parse JSON-Lines | New |
| `internal/agentrun/agentrun_suite_test.go` + `_test.go` | ginkgo tests | New |
| `internal/bus/bus.go` | copied provider event bus | Copied |
| `internal/bus/bus_suite_test.go` + `_test.go` | ginkgo tests | New |
| `internal/tui/*.go` (TUI files) | model, constants, pages | Moved from agent |
| `internal/tui/branding.go` | `BrandingFile`, `DefaultTitleInteractiveInstaller` | Copied |
| `internal/tui/cloudconfig.go` | `RenderCloudConfig(*Model)`, `mergeYAML`, `addHeader`, stage consts | New (from `config.go`) |
| `internal/tui/install_process_page.go` | rewritten to use `agentrun` | Rewritten |
| `internal/tui/*_suite_test.go` + golden test | ginkgo | New + moved |

---

## Task 1: Scaffold the module

**Files:** Create `go.mod`, `.gitignore`, `main.go` (stub)

- [ ] **Step 1: Init the module**

Run (in `/home/mudler/_git/kairos-installer`):
```bash
go mod init github.com/kairos-io/kairos-installer
go mod edit -go=1.26.4
```

- [ ] **Step 2: Create `.gitignore`**

```
/kairos-installer
*.iso
coverage.out
.DS_Store
```

- [ ] **Step 3: Create a buildable `main.go` stub**

```go
package main

import "fmt"

func main() {
	fmt.Println("kairos-installer")
}
```

- [ ] **Step 4: Verify**

Run: `go build ./... && echo BUILD_OK`
Expected: `BUILD_OK`.

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitignore main.go
git commit -m "chore: scaffold kairos-installer module

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Copy the provider event bus (`internal/bus`)

**Files:** Create `internal/bus/bus.go`, `internal/bus/bus_suite_test.go`, `internal/bus/bus_test.go`

- [ ] **Step 1: Create `internal/bus/bus.go`**

This is `kairos-agent/internal/bus/bus.go` verbatim except the comment header. Content:

```go
package bus

import (
	"fmt"
	"os"

	"github.com/kairos-io/kairos-sdk/bus"
	"github.com/mudler/go-pluggable"
)

// Manager is the bus instance manager, which subscribes plugins to events emitted.
var Manager = NewBus()

func NewBus() *Bus {
	return &Bus{
		Manager: pluggable.NewManager(
			bus.AllEvents,
		),
	}
}

func Reload() {
	Manager = NewBus()
	Manager.Initialize()
}

type Bus struct {
	*pluggable.Manager
	registered bool
}

func (b *Bus) LoadProviders() {
	wd, _ := os.Getwd()
	b.Manager.Autoload("agent-provider", "/system/providers", "/usr/local/system/providers", wd).Register()
}

func (b *Bus) HasRegisteredPlugins() bool {
	return len(b.Plugins) > 0
}

func (b *Bus) Initialize() {
	if b.registered {
		return
	}

	b.LoadProviders()
	for i := range b.Manager.Events {
		e := b.Manager.Events[i]
		b.Manager.Response(e, func(p *pluggable.Plugin, r *pluggable.EventResponse) {
			if os.Getenv("BUS_DEBUG") == "true" {
				fmt.Println(
					fmt.Sprintf("[provider event: %s]", e),
					"received from",
					p.Name,
					"at",
					p.Executable,
					r,
				)
			}
			if r.Errored() {
				err := fmt.Sprintf("Provider %s at %s had an error: %s", p.Name, p.Executable, r.Error)
				fmt.Println(err)
				os.Exit(1)
			}

			if r.State != "" {
				fmt.Println(fmt.Sprintf("[provider event: %s]", e), r.State)
			}
		})
	}
	b.registered = true
}
```

- [ ] **Step 2: Create suite bootstrap `internal/bus/bus_suite_test.go`**

```go
package bus_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBus(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bus Suite")
}
```

- [ ] **Step 3: Create `internal/bus/bus_test.go`**

```go
package bus_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/kairos-installer/internal/bus"
)

var _ = Describe("Bus", func() {
	It("creates a fresh manager with no registered plugins", func() {
		b := bus.NewBus()
		Expect(b.HasRegisteredPlugins()).To(BeFalse())
	})
})
```

- [ ] **Step 4: Verify**

Run: `go mod tidy && go build ./... && go test ./internal/bus/...`
Expected: build OK; 1 spec passes.

- [ ] **Step 5: Commit**

```bash
git add internal/bus go.mod go.sum
git commit -m "feat(bus): copy provider event bus from kairos-agent

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Agent runner + JSON-Lines parser (`internal/agentrun`)

**Files:** Create `internal/agentrun/agentrun.go`, `internal/agentrun/agentrun_suite_test.go`, `internal/agentrun/agentrun_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/agentrun/agentrun_suite_test.go`:

```go
package agentrun_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAgentRun(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AgentRun Suite")
}
```

Create `internal/agentrun/agentrun_test.go`:

```go
package agentrun_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/kairos-installer/internal/agentrun"
)

var _ = Describe("agentrun", func() {
	Describe("ResolveAgentBin", func() {
		It("prefers KAIROS_AGENT_BIN when the file exists", func() {
			dir := GinkgoT().TempDir()
			bin := filepath.Join(dir, "fake-agent")
			Expect(os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)).To(Succeed())
			GinkgoT().Setenv(agentrun.EnvAgentBin, bin)
			Expect(agentrun.ResolveAgentBin()).To(Equal(bin))
		})

		It("falls back to kairos-agent on PATH", func() {
			GinkgoT().Setenv(agentrun.EnvAgentBin, "")
			dir := GinkgoT().TempDir()
			bin := filepath.Join(dir, "kairos-agent")
			Expect(os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)).To(Succeed())
			GinkgoT().Setenv("PATH", dir)
			Expect(agentrun.ResolveAgentBin()).To(Equal(bin))
		})

		It("returns empty when nothing is found", func() {
			GinkgoT().Setenv(agentrun.EnvAgentBin, "")
			GinkgoT().Setenv("PATH", GinkgoT().TempDir())
			Expect(agentrun.ResolveAgentBin()).To(BeEmpty())
		})
	})

	Describe("Command", func() {
		It("builds manual-install with source, finish flag, default dirs and progress env", func() {
			cmd := agentrun.Command("/usr/bin/kairos-agent", "/tmp/cc.yaml", "oci://x:y", "reboot")
			Expect(cmd.Args).To(Equal([]string{
				"/usr/bin/kairos-agent", "manual-install",
				"--source", "oci://x:y",
				"--use-default-dirs",
				"--reboot",
				"/tmp/cc.yaml",
			}))
			Expect(cmd.Env).To(ContainElement("KAIROS_AGENT_PROGRESS=1"))
		})

		It("omits the finish flag when action is nothing", func() {
			cmd := agentrun.Command("/usr/bin/kairos-agent", "/tmp/cc.yaml", "", "nothing")
			Expect(cmd.Args).To(Equal([]string{
				"/usr/bin/kairos-agent", "manual-install",
				"--use-default-dirs",
				"/tmp/cc.yaml",
			}))
		})

		It("uses --poweroff for the poweroff action", func() {
			cmd := agentrun.Command("/usr/bin/kairos-agent", "/tmp/cc.yaml", "", "poweroff")
			Expect(cmd.Args).To(ContainElement("--poweroff"))
		})
	})

	Describe("ParseLine", func() {
		It("parses a step event", func() {
			ev, ok := agentrun.ParseLine([]byte(`{"event":"step","step":"partition"}`))
			Expect(ok).To(BeTrue())
			Expect(ev.Event).To(Equal("step"))
			Expect(ev.Step).To(Equal("partition"))
		})

		It("parses an error event", func() {
			ev, ok := agentrun.ParseLine([]byte(`{"event":"error","message":"boom"}`))
			Expect(ok).To(BeTrue())
			Expect(ev.Event).To(Equal("error"))
			Expect(ev.Message).To(Equal("boom"))
		})

		It("rejects a plain log line", func() {
			_, ok := agentrun.ParseLine([]byte(`time=... level=info msg=hello`))
			Expect(ok).To(BeFalse())
		})

		It("rejects JSON without an event field", func() {
			_, ok := agentrun.ParseLine([]byte(`{"level":"info","message":"hi"}`))
			Expect(ok).To(BeFalse())
		})
	})

	Describe("Run", func() {
		It("streams events and reports a clean exit", func() {
			dir := GinkgoT().TempDir()
			bin := filepath.Join(dir, "kairos-agent")
			script := "#!/bin/sh\n" +
				`echo '{"event":"step","step":"partition"}'` + "\n" +
				`echo 'some plain log line'` + "\n" +
				`echo '{"event":"step","step":"done"}'` + "\n" +
				"exit 0\n"
			Expect(os.WriteFile(bin, []byte(script), 0o755)).To(Succeed())

			var steps []string
			err := agentrun.Run(bin, "/tmp/cc.yaml", "", "nothing",
				func(ev agentrun.ProgressEvent) {
					if ev.Event == "step" {
						steps = append(steps, ev.Step)
					}
				},
				func(string) {}, // log sink
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(steps).To(Equal([]string{"partition", "done"}))
		})

		It("returns the exit error when the agent fails", func() {
			dir := GinkgoT().TempDir()
			bin := filepath.Join(dir, "kairos-agent")
			Expect(os.WriteFile(bin, []byte("#!/bin/sh\nexit 5\n"), 0o755)).To(Succeed())
			err := agentrun.Run(bin, "/tmp/cc.yaml", "", "nothing",
				func(agentrun.ProgressEvent) {}, func(string) {})
			Expect(err).To(HaveOccurred())
		})
	})
})
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/agentrun/...`
Expected: FAIL (undefined symbols).

- [ ] **Step 3: Implement `internal/agentrun/agentrun.go`**

```go
// Package agentrun drives kairos-agent's manual-install and parses its
// JSON-Lines progress stream. It has no TUI dependencies so it can be tested
// in isolation.
package agentrun

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
)

// EnvAgentBin overrides agent discovery with an explicit path.
const EnvAgentBin = "KAIROS_AGENT_BIN"

// agentBinName is the fixed name looked up on PATH.
const agentBinName = "kairos-agent"

// ProgressEvent is one parsed JSON-Lines progress line from the agent.
type ProgressEvent struct {
	Event   string `json:"event"`
	Step    string `json:"step"`
	Message string `json:"message"`
}

// ResolveAgentBin returns the kairos-agent path: KAIROS_AGENT_BIN (must exist)
// then kairos-agent on PATH, else "".
func ResolveAgentBin() string {
	if p := os.Getenv(EnvAgentBin); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath(agentBinName); err == nil {
		return p
	}
	return ""
}

// Command builds the manual-install invocation. finishAction is one of
// "reboot", "poweroff", or anything else (no finish flag). It sets
// KAIROS_AGENT_PROGRESS=1 so the agent emits progress events.
func Command(agentBin, cfgPath, source, finishAction string) *exec.Cmd {
	args := []string{"manual-install"}
	if source != "" {
		args = append(args, "--source", source)
	}
	args = append(args, "--use-default-dirs")
	switch finishAction {
	case "reboot":
		args = append(args, "--reboot")
	case "poweroff":
		args = append(args, "--poweroff")
	}
	args = append(args, cfgPath)

	cmd := exec.Command(agentBin, args...)
	cmd.Env = append(os.Environ(), "KAIROS_AGENT_PROGRESS=1")
	return cmd
}

// ParseLine parses one stdout line. ok is true only for a JSON object carrying
// a non-empty "event" field; everything else (plain logs, eventless JSON) is
// reported as ok=false.
func ParseLine(line []byte) (ProgressEvent, bool) {
	var ev ProgressEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return ProgressEvent{}, false
	}
	if ev.Event == "" {
		return ProgressEvent{}, false
	}
	return ev, true
}

// Run execs the agent, calling onEvent for each progress event and onLog for
// each non-event stdout line. It returns the process exit error, if any.
func Run(agentBin, cfgPath, source, finishAction string, onEvent func(ProgressEvent), onLog func(string)) error {
	cmd := Command(agentBin, cfgPath, source, finishAction)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if ev, ok := ParseLine(line); ok {
			onEvent(ev)
		} else if len(line) > 0 {
			onLog(string(line))
		}
	}
	return cmd.Wait()
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/agentrun/... -v`
Expected: PASS (all specs).

- [ ] **Step 5: Commit**

```bash
git add internal/agentrun
git commit -m "feat(agentrun): exec manual-install and parse JSON-Lines progress

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Move the TUI into `internal/tui`

**Files:** Create `internal/tui/` from the agent's TUI files + new `branding.go` and `cloudconfig.go`; provide a temporary install-page stub (rewritten in Task 5).

This is a mechanical migration. The agent's TUI files are all `package agent` and reference each other plus a few agent-internal symbols. Move them into one package `tui` and rewire those few references.

- [ ] **Step 1: Copy the TUI files**

Copy these files from `/home/mudler/_git/kairos-agent/internal/agent/` into `/home/mudler/_git/kairos-installer/internal/tui/` (keep names, lowercase for style is optional — keep as-is to minimize diff):

```
TUImodel.go TUIconstants.go TUIdiskPage.go TUIuserPasswordPage.go
TUIsshKeysPage.go TUIinstallOptionsPage.go TUIcustomizationPage.go
TUIgenericPage.go TUIsummaryPage.go TUIuserdataPage.go
TUIdiskPage_internal_test.go
```

Do NOT copy `TUIinstallProcessPage.go` (replaced by a stub now, rewritten in Task 5) and do NOT copy `config.go` (replaced by `cloudconfig.go`).

- [ ] **Step 2: Rewrite the package declaration**

In every copied file, change `package agent` → `package tui`. (In the test file, keep `package tui` too — it is an internal test.)

- [ ] **Step 3: Replace the branding helper references**

Create `internal/tui/branding.go`:

```go
package tui

import (
	"os"
	"path"
)

// BrandingFile returns the path to a branding text file under /etc/kairos/branding.
func BrandingFile(s string) string {
	return path.Join("/etc", "kairos", "branding", s)
}

// DefaultTitleInteractiveInstaller returns the installer title from branding, or a default.
func DefaultTitleInteractiveInstaller() string {
	branding, err := os.ReadFile(BrandingFile("interactive_install_text"))
	if err == nil {
		return string(branding)
	}
	return "Kairos Interactive Installer"
}
```

Then in the copied files, remove the `github.com/kairos-io/kairos-agent/v2/internal/kairos` import and replace `kairos.BrandingFile(` → `BrandingFile(` and `kairos.DefaultTitleInteractiveInstaller(` → `DefaultTitleInteractiveInstaller(`. Affected files: `TUImodel.go` (DefaultTitleInteractiveInstaller), `TUIconstants.go`, `TUIinstallOptionsPage.go`, `TUIsummaryPage.go` (BrandingFile).

- [ ] **Step 4: Repoint the bus import**

In `TUIcustomizationPage.go`, change the import `github.com/kairos-io/kairos-agent/v2/internal/bus` → `github.com/kairos-io/kairos-installer/internal/bus`. The `bus.Manager` usage is unchanged.

- [ ] **Step 5: Create `internal/tui/cloudconfig.go`**

This replaces `NewInteractiveInstallConfig`. It emits a `#cloud-config` string (no collector — the agent merges system dirs via `--use-default-dirs`). Copies `MergeYAML`/`AddHeader` and inlines the stage strings.

```go
package tui

import (
	"fmt"

	"github.com/mudler/yip/pkg/schema"
	sdkConfig "github.com/kairos-io/kairos-sdk/types/config"
	sdkInstall "github.com/kairos-io/kairos-sdk/types/install"
	"gopkg.in/yaml.v3"
)

// yip stage names (copied from kairos-agent pkg/config).
const (
	networkStage   = "network"
	initramfsStage = "initramfs"
)

// extraFields wraps the dynamic plugin fields so they inline at the top level.
type extraFields struct {
	Extrafields map[string]any `yaml:",inline,omitempty"`
}

// RenderCloudConfig turns the collected Model into a #cloud-config document.
func RenderCloudConfig(m *Model) (string, error) {
	extras := extraFields{m.extraFields}

	cc := &sdkConfig.Config{
		Install: &sdkInstall.Install{
			Device: m.disk,
		},
	}
	if m.source != "" {
		cc.Install.Source = m.source
	}
	switch m.finishAction {
	case "reboot":
		cc.Install.Reboot = true
	case "poweroff":
		cc.Install.Poweroff = true
	}

	var cloudConfig schema.YipConfig
	if m.username != "" {
		user := schema.User{
			Name:              m.username,
			PasswordHash:      m.password,
			Groups:            []string{"admin"},
			SSHAuthorizedKeys: m.sshKeys,
		}
		stage := networkStage
		if len(m.sshKeys) == 0 {
			stage = initramfsStage
		}
		cloudConfig = schema.YipConfig{
			Name: "Config generated by the installer",
			Stages: map[string][]schema.Stage{stage: {
				{Users: map[string]schema.User{m.username: user}},
			}},
		}
	} else {
		cc.Install.NoUsers = true
	}

	dat, err := mergeYAML(cloudConfig, cc, extras)
	if err != nil {
		return "", err
	}
	return addHeader("#cloud-config", string(dat)), nil
}

// mergeYAML merges objects into a single YAML document (copied from kairos-agent).
func mergeYAML(objs ...interface{}) ([]byte, error) {
	finalData := make(map[string]interface{})
	for _, o := range objs {
		dat, err := yaml.Marshal(o)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(dat, &finalData); err != nil {
			return nil, err
		}
	}
	return yaml.Marshal(finalData)
}

func addHeader(header, data string) string {
	return fmt.Sprintf("%s\n%s", header, data)
}
```

- [ ] **Step 6: Add a temporary install-page stub `internal/tui/install_process_page.go`**

So the package compiles and `InitialModel` keeps its page list. The stub deliberately exposes every member `TUImodel.go` references (`progress`, `step`, `steps`, `errorMsg`, `Abort()`, and the `CheckInstallerMsg` type) so the moved `TUImodel.go` compiles **verbatim** — no edits to the model file. (The behavior is filled in by Task 5.)

```go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// CheckInstallerMsg triggers a poll of the installer output channel.
type CheckInstallerMsg struct{}

type installProcessPage struct {
	progress int
	step     string
	steps    []string
	errorMsg string
}

func newInstallProcessPage() *installProcessPage {
	return &installProcessPage{
		progress: 0,
		step:     InstallDefaultStep,
		steps: []string{
			InstallDefaultStep, InstallPartitionStep, InstallBeforeInstallStep,
			InstallActiveStep, InstallBootloaderStep, InstallRecoveryStep,
			InstallPassiveStep, InstallAfterInstallStep, InstallCompleteStep,
		},
	}
}

func (p *installProcessPage) Init() tea.Cmd                  { return nil }
func (p *installProcessPage) Update(tea.Msg) (Page, tea.Cmd) { return p, nil }
func (p *installProcessPage) View() string                  { return "install (stub)" }
func (p *installProcessPage) Title() string                 { return "Installing" }
func (p *installProcessPage) Help() string                  { return "" }
func (p *installProcessPage) ID() string                    { return "install_process" }
func (p *installProcessPage) Abort()                        {}
```

Because the stub matches every reference, `TUImodel.go` moves **unchanged** (package rename only). Do NOT edit `TUImodel.go` beyond the `package` line and the branding substitution from Step 3.

- [ ] **Step 7: Export the entrypoint**

Add to `TUImodel.go` (the existing `InitialModel` is already exported and returns `Model`; just confirm it and that `Model` is exported). No change needed if already exported.

- [ ] **Step 8: Verify build + migrated test**

Run: `go mod tidy && go build ./... && go test ./internal/tui/...`
Expected: build OK; the migrated disk-pagination test passes.

- [ ] **Step 9: Commit**

```bash
git add internal/tui go.mod go.sum
git commit -m "feat(tui): move interactive installer TUI from kairos-agent

Branding + cloud-config shaping reimplemented locally; install page
stubbed pending the agentrun rewrite. Fully decoupled from the agent module.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Rewrite the install process page on top of `agentrun`

**Files:** Replace `internal/tui/install_process_page.go`; restore the install-page handling in `internal/tui/TUImodel.go`.

- [ ] **Step 1: Write the real install page**

Replace `internal/tui/install_process_page.go` with the version below. It keeps the existing bubbletea channel/`CheckInstallerMsg` structure (so `TUImodel.go` key-handling works) but drives it from `agentrun.Run` instead of in-process `RunInstall`. The step enum from the agent maps to the display steps.

```go
package tui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kairos-io/kairos-installer/internal/agentrun"
)

// stepDisplay maps an agent progress step to the UI step label.
var stepDisplay = map[string]string{
	"partition":      InstallPartitionStep,
	"before-install": InstallBeforeInstallStep,
	"active":         InstallActiveStep,
	"bootloader":     InstallBootloaderStep,
	"recovery":       InstallRecoveryStep,
	"passive":        InstallPassiveStep,
	"after-install":  InstallAfterInstallStep,
	"done":           InstallCompleteStep,
}

type installProcessPage struct {
	progress int
	step     string
	steps    []string
	output   chan string
	done     chan bool
	once     sync.Once
	errorMsg string
}

func newInstallProcessPage() *installProcessPage {
	return &installProcessPage{
		progress: 0,
		step:     InstallDefaultStep,
		steps: []string{
			InstallDefaultStep, InstallPartitionStep, InstallBeforeInstallStep,
			InstallActiveStep, InstallBootloaderStep, InstallRecoveryStep,
			InstallPassiveStep, InstallAfterInstallStep, InstallCompleteStep,
		},
		output: make(chan string, 16),
		done:   make(chan bool),
	}
}

func (p *installProcessPage) Init() tea.Cmd {
	p.once.Do(func() {
		ccString, err := RenderCloudConfig(&mainModel)
		if err != nil {
			p.errorMsg = "Failed to generate install configuration: " + err.Error()
			return
		}
		f, err := os.CreateTemp("", "kairos-install-*.yaml")
		if err != nil {
			p.errorMsg = "Failed to write install configuration: " + err.Error()
			return
		}
		_, _ = f.WriteString(ccString)
		_ = f.Close()
		cfgPath := f.Name()

		agentBin := agentrun.ResolveAgentBin()
		if agentBin == "" {
			p.errorMsg = "kairos-agent not found (set KAIROS_AGENT_BIN or add it to PATH)"
			return
		}

		go func() {
			defer close(p.done)
			defer os.Remove(cfgPath)
			err := agentrun.Run(agentBin, cfgPath, mainModel.source, mainModel.finishAction,
				func(ev agentrun.ProgressEvent) {
					switch ev.Event {
					case "step":
						if disp, ok := stepDisplay[ev.Step]; ok {
							p.output <- StepPrefix + disp
						}
					case "error":
						p.output <- ErrorPrefix + ev.Message
					}
				},
				func(line string) { mainModel.log.Print(line) },
			)
			if err != nil && p.errorMsg == "" {
				p.output <- ErrorPrefix + err.Error()
			}
		}()
	})
	return func() tea.Msg { return CheckInstallerMsg{} }
}

// CheckInstallerMsg triggers a poll of the installer output channel.
type CheckInstallerMsg struct{}

func (p *installProcessPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg.(type) {
	case CheckInstallerMsg:
		select {
		case output, ok := <-p.output:
			if !ok {
				return p, nil
			}
			if strings.HasPrefix(output, StepPrefix) {
				stepName := strings.TrimPrefix(output, StepPrefix)
				for i, s := range p.steps {
					if s == stepName {
						p.progress = i
						p.step = stepName
						break
					}
				}
			} else if strings.HasPrefix(output, ErrorPrefix) {
				p.errorMsg = strings.TrimPrefix(output, ErrorPrefix)
				p.step = "Error: " + p.errorMsg
				return p, nil
			}
			return p, func() tea.Msg { return CheckInstallerMsg{} }
		case <-p.done:
			if p.errorMsg == "" {
				p.progress = len(p.steps) - 1
				p.step = p.steps[len(p.steps)-1]
			}
			return p, nil
		default:
			return p, tea.Tick(100*time.Millisecond, func(_ time.Time) tea.Msg {
				return CheckInstallerMsg{}
			})
		}
	}
	return p, nil
}

func (p *installProcessPage) View() string {
	if p.errorMsg != "" {
		s := "Installation encountered an error.\n\n"
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true).
			Render("[!] Installation error: "+p.errorMsg) + "\n\n"
		return s
	}
	s := "Installation in Progress\n\n"
	totalSteps := len(p.steps)
	progressPercent := (p.progress * 100) / (totalSteps - 1)
	barWidth := 40
	filled := barWidth * progressPercent / 100
	progressBar := lipgloss.NewStyle().Foreground(kairosHighlight2).Background(kairosBg).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(kairosBorder).Background(kairosBg).Render(strings.Repeat("░", barWidth-filled))
	s += "Progress:" + progressBar + lipgloss.NewStyle().Background(kairosBg).Render(" ")
	s += lipgloss.NewStyle().Foreground(kairosText).Background(kairosBg).Bold(true).Render(fmt.Sprintf("%d%%", progressPercent))
	s += "\n\n"
	s += fmt.Sprintf("Current step: %s\n\n", p.step)
	s += "Completed steps:\n"
	tick := lipgloss.NewStyle().Foreground(kairosAccent).Render(checkMark)
	for i := 0; i < p.progress; i++ {
		s += fmt.Sprintf("%s %s\n", tick, p.steps[i])
	}
	if p.progress < len(p.steps)-1 {
		warning := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true).Render("[!]  Do not power off the system during installation!")
		s += "\n" + warning
	} else {
		text := "Installation completed successfully!\n"
		if mainModel.finishAction == "nothing" {
			text += "You can now reboot or shut down your system."
		}
		s += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Render(text)
	}
	return s
}

func (p *installProcessPage) Title() string { return "Installing" }

func (p *installProcessPage) Help() string {
	if p.progress >= len(p.steps)-1 || p.errorMsg != "" {
		if mainModel.finishAction == "nothing" {
			return "Press any key to exit"
		}
		return "System will " + mainModel.finishAction + " shortly"
	}
	return "Installation in progress - Use ctrl+c to abort"
}

func (p *installProcessPage) ID() string { return "install_process" }

// Abort is a no-op, matching the agent's prior behavior (its install ran
// in-process, so there was no child process to kill). Cancelling the running
// agent mid-install can be added later via context cancellation in agentrun.
func (p *installProcessPage) Abort() {}
```

- [ ] **Step 2: No `TUImodel.go` changes**

`TUImodel.go` was moved verbatim in Task 4 and already references `installPage.errorMsg`, `installPage.progress`, `installPage.steps`, `installPage.Abort()`, and `CheckInstallerMsg`. The rewritten page below re-declares all of them (replacing the stub file wholesale), so `TUImodel.go` needs no edits. Confirm you are only replacing `internal/tui/install_process_page.go`.

- [ ] **Step 3: Verify**

Run: `go build ./... && go vet ./... && go test ./internal/...`
Expected: build/vet OK; tui + agentrun + bus tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): drive install via agentrun + JSON progress

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Wire `main.go`

**Files:** Replace `main.go`

- [ ] **Step 1: Implement the entrypoint**

```go
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	sdkLogger "github.com/kairos-io/kairos-sdk/types/logger"

	"github.com/kairos-io/kairos-installer/internal/tui"
)

func main() {
	source := flag.String("source", "", "installation source (passed through to kairos-agent)")
	flag.Parse()

	logger := sdkLogger.NewKairosLogger("installer", "info", true)
	p := tea.NewProgram(tui.InitialModel(&logger, *source), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify**

Run: `go build -o kairos-installer . && ./kairos-installer --help 2>&1 | head -5 ; echo OK`
Expected: builds; `--source` flag shown; `OK`.

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: wire main entrypoint (flags + launch TUI)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Golden cloud-config test + end-to-end smoke

**Files:** Create `internal/tui/cloudconfig_test.go` (+ suite bootstrap if `internal/tui` has none yet)

- [ ] **Step 1: Add a suite bootstrap if missing**

If `internal/tui` has no `*_suite_test.go` calling `RunSpecs`, create `internal/tui/tui_suite_test.go`:

```go
package tui

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTUI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TUI Suite")
}
```

(The migrated `TUIdiskPage_internal_test.go` uses plain `testing` and coexists fine.)

- [ ] **Step 2: Golden cloud-config test**

Create `internal/tui/cloudconfig_test.go` (package `tui`, internal, so it can build a `Model`):

```go
package tui

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RenderCloudConfig", func() {
	It("renders a user stage with ssh keys at the network stage", func() {
		m := &Model{
			disk:         "/dev/sda",
			username:     "kairos",
			password:     "hashhash",
			sshKeys:      []string{"ssh-ed25519 AAAA"},
			finishAction: "reboot",
		}
		out, err := RenderCloudConfig(m)
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(HavePrefix("#cloud-config\n"))
		Expect(out).To(ContainSubstring("network:"))
		Expect(out).To(ContainSubstring("kairos"))
		Expect(out).To(ContainSubstring("ssh-ed25519 AAAA"))
		Expect(out).To(ContainSubstring("reboot: true"))
	})

	It("sets no_users and no user stage when no username is given", func() {
		m := &Model{disk: "/dev/sda", finishAction: "nothing"}
		out, err := RenderCloudConfig(m)
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(ContainSubstring("no_users: true"))
		Expect(out).ToNot(ContainSubstring("network:"))
	})

	It("uses the initramfs stage when there are no ssh keys", func() {
		m := &Model{disk: "/dev/sda", username: "kairos", password: "x"}
		out, err := RenderCloudConfig(m)
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(ContainSubstring("initramfs:"))
	})
})
```

NOTE: the exact YAML key for `NoUsers`/`Reboot` depends on the kairos-sdk `Install` struct tags. Before finalizing, run the test once; if a substring assertion fails, inspect the actual `out` (add a temporary `GinkgoWriter.Println(out)`) and correct the expected substring to match the real sdk tag (e.g. `no_users` vs `nousers`). Do not change production code to match the test — match the test to the real sdk output.

- [ ] **Step 3: Verify the whole module**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all green.

- [ ] **Step 4: End-to-end smoke with a fake agent**

This exercises the install page's runner without a real install. Run:
```bash
go build -o /tmp/kairos-installer .
mkdir -p /tmp/fakebin
cat > /tmp/fakebin/kairos-agent <<'EOF'
#!/bin/sh
echo '{"event":"step","step":"partition"}'
echo 'plain agent log line'
echo '{"event":"step","step":"active"}'
echo '{"event":"step","step":"done"}'
exit 0
EOF
chmod +x /tmp/fakebin/kairos-agent
echo "Smoke: agentrun against the fake agent is covered by internal/agentrun tests;"
echo "the TUI cannot be asserted headlessly here. Confirm the binary launches:"
KAIROS_AGENT_BIN=/tmp/fakebin/kairos-agent KAIROS_TUI_WIDTH=80 KAIROS_TUI_HEIGHT=24 \
  timeout 2 /tmp/kairos-installer --source dir:///tmp >/dev/null 2>&1; echo "launch exit: $?"
```
Expected: the binary launches (timeout kills the TUI; a non-zero timeout exit is fine). The real progress path is covered by `internal/agentrun` tests and, later, the hadron e2e.

- [ ] **Step 5: Commit**

```bash
git add internal/tui
git commit -m "test(tui): golden cloud-config assertions

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Push the branch

- [ ] **Step 1: Final gate**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all green.

- [ ] **Step 2: Push**

The repo currently has only `main` (the spec commit). Create a feature branch and push for a PR:
```bash
git checkout -b feat/initial-installer
git push -u origin feat/initial-installer
```
(Leave PR creation to the user.)

---

## Self-review checklist (run before starting)

- Every moved file's agent-internal references (`internal/kairos`, `internal/bus`, `pkg/config`, `RunInstall`, `NewInteractiveInstallConfig`) are accounted for in Tasks 4–5.
- `agentrun` has zero TUI imports; `tui` imports `agentrun` and `bus`, never kairos-agent.
- The step enum mapping covers all 8 contract steps.
- Tests are ginkgo/gomega with per-package suites.

## Follow-on (separate efforts)
- kairos-agent PR: `go:embed` the built `kairos-installer` as the dispatcher fallback; delete the agent's TUI + bubbletea/lipgloss deps.
- kairos-sdk: move `EventInteractiveInstall` + `YAMLPrompt`; both repos then drop the copied bus/types.
- hadron ISO e2e.
