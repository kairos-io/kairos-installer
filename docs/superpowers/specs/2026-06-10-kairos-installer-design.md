# kairos-installer — Design

**Date:** 2026-06-10
**Status:** Draft for review
**Repo:** `github.com/kairos-io/kairos-installer` (new, currently empty)
**Companion:** kairos-agent already ships the agent-side contract (dispatcher + `manual-install` JSON-Lines progress) on PR branch `feat/installer-dispatch`.

## Problem & Motivation

The interactive installer UX currently lives *inside* kairos-agent
(`internal/agent/TUI*.go` + config shaping). We are extracting it into this
separate, separately-owned binary so a product team can own and evolve the UX
independently, while kairos-agent owns only partition/configure/install.

kairos-agent already does its half: `interactive-install` is now a dispatcher
that resolves an installer binary (`KAIROS_INSTALLER` env → `kairos-installer`
on `$PATH` → embedded fallback) and execs it; `manual-install` is the
drive-back command and emits machine-readable progress as JSON Lines on stdout
when `KAIROS_AGENT_PROGRESS` is set.

This document designs the `kairos-installer` binary that consumes that contract.

## Goals

- A standalone Go binary named `kairos-installer` that renders the interactive
  install UX (the existing bubbletea TUI) and drives the install by exec'ing
  `kairos-agent manual-install`.
- **Fully decoupled** from the kairos-agent Go module: depends only on
  kairos-sdk, the TUI libraries, and go-pluggable. The only coupling to the
  agent is the documented CLI contract.
- Preserve existing behavior: disk/user/ssh/finish-action selection, the
  summary/userdata pages, and the `EventInteractiveInstall` plugin mechanism
  (providers injecting extra fields).
- Render live progress from the agent's JSON-Lines events.

## Non-Goals

- No change to the install engine or the contract itself (already shipped in the
  agent PR). If a gap is found, fix it in the agent, not by re-implementing
  install logic here.
- No `go:embed`/agent-slimming work — that lands in the kairos-agent PR
  (follow-on, see Sequencing).
- No new UX features beyond what exists today; this is an extraction.

## Architecture

### Module & layout

```
github.com/kairos-io/kairos-installer   (go.mod, Go 1.26)
  main.go                 # cmd entrypoint: flags (--source), launch TUI
  internal/tui/           # moved TUI*.go (model + pages), lightly adapted
  internal/config/        # config shaping: Model -> #cloud-config
  internal/bus/           # copied minimal provider bus (EventInteractiveInstall)
  internal/branding/      # copied BrandingFile + DefaultTitle helpers
  internal/agentrun/      # exec kairos-agent manual-install + parse JSON progress
```

(Package boundaries may be refined during planning; the point is small, focused
packages rather than one flat `agent`-style package.)

### What moves from kairos-agent (verbatim-ish)

- `internal/agent/TUImodel.go`, `TUIconstants.go`, and all page files
  (`TUIdiskPage.go`, `TUIuserPasswordPage.go`, `TUIsshKeysPage.go`,
  `TUIinstallOptionsPage.go`, `TUIcustomizationPage.go`, `TUIgenericPage.go`,
  `TUIsummaryPage.go`, `TUIuserdataPage.go`) → `internal/tui/`.
- The bubbletea program launch from `interactive_install.go` → `main.go` (the
  dispatcher half stays in the agent).
- `NewInteractiveInstallConfig` + `ExtraFields` + `setValueForSectionInMainModel`
  from `internal/agent/config.go` → `internal/config/`.

### What is copied (small, because `internal/` can't be imported cross-module)

- `internal/kairos/branding.go` (`BrandingFile`, `DefaultTitleInteractiveInstaller`)
  → `internal/branding/` (a few lines).
- `internal/bus/bus.go` (~70 lines: a `*pluggable.Manager` wrapper that
  Autoloads `agent-provider` binaries from `/system/providers`,
  `/usr/local/system/providers`, `$PWD`, and registers event handlers) →
  `internal/bus/`. Uses `kairos-sdk/bus` + `go-pluggable` directly.
- `pkg/constants.GetUserConfigDirs()` value (the default config dirs list) →
  inlined in `internal/config/`.
- `pkg/config.MergeYAML` + `pkg/config.AddHeader` (tiny pure helpers) →
  `internal/config/`. `pkg/config.ScanNoLogs` is replaced by calling
  `kairos-sdk/collector` directly with the `NoLogs` option + `Directories(...)`.
  The yip stage constants (`NetworkStage`, `InitramfsStage`) are inlined as
  string constants.

### What is rewritten

`TUIinstallProcessPage.go` (the install-execution page) changes from
"call `RunInstall(cc)` in-process and tail a buffer logger for log strings" to:

1. Build the `#cloud-config` via `internal/config` and write it to a temp file.
2. Resolve the agent binary: `KAIROS_AGENT_BIN` env → `kairos-agent` on `$PATH`.
3. Exec `kairos-agent manual-install --source <source> [--reboot|--poweroff]
   <config.yaml>` with `KAIROS_AGENT_PROGRESS=1` in the child env, capturing
   stdout.
4. Scan stdout line-by-line; for each line that parses as JSON with an `event`
   field, drive the progress UI:
   - `{"event":"step","step":"<step>"}` → advance the progress bar / step list,
     mapping the step enum to the existing display strings
     (`InstallPartitionStep`, …, `InstallCompleteStep`).
   - `{"event":"error","message":"<msg>"}` → show the error and stop.
   - Non-JSON lines are ordinary agent logs (optionally shown in a log pane).
5. On process exit: success → completion page; non-zero exit / error event →
   failure state surfacing the message and the exit code.

This lives in `internal/agentrun/` (the exec + JSON parsing) with the page in
`internal/tui/` consuming it via a channel of progress messages, mirroring the
current goroutine+channel structure.

### Runtime contract consumed

- Launched by the agent dispatcher as `kairos-installer [--source <source>]`
  with the tty inherited.
- Drives `kairos-agent manual-install --source <source> [--reboot|--poweroff]
  <config.yaml>` with `KAIROS_AGENT_PROGRESS=1`.
- Parses JSON-Lines progress per `docs/installer-contract.md` in kairos-agent;
  tolerates unknown `event`s and extra fields (forward-compat).

## Data Flow

```
agent dispatcher: exec kairos-installer --source <s>   (tty inherited)
        │
        ▼
main.go  → bubbletea program (InitialModel)
        │   disk / user / ssh / finish-action pages
        │   EventInteractiveInstall → YAMLPrompt[] from agent-provider-* (internal/bus)
        ▼
internal/config: Model → #cloud-config  → /tmp/install-config.yaml
        │
        ▼
internal/agentrun: exec `kairos-agent manual-install --source <s> [flags] cfg.yaml`
        │   env KAIROS_AGENT_PROGRESS=1 ; resolve agent via KAIROS_AGENT_BIN→PATH
        │   stdout: {"event":"step",...} / {"event":"error",...} + log noise
        ▼
internal/tui install page: render progress bar from events; exit code → done/fail
```

## Error Handling

- **Agent binary not found** (`KAIROS_AGENT_BIN` unset and no `kairos-agent` on
  PATH): fail the install page with a clear message; do not proceed.
- **`manual-install` exits non-zero / emits `error` event**: show the message,
  stop the progress bar, surface the exit code; offer to drop to a shell or
  reboot per existing UX.
- **No provider plugins / bus errors**: the plugin query degrades to "no extra
  fields" (same as today); a provider error path mirrors the copied bus
  behavior.
- **Malformed progress lines**: ignored as log noise (never crash the parser).

## Testing

- **Ginkgo/Gomega** throughout, consistent with the kairos-agent codebase
  (suite bootstrap per package, `Describe`/`It`, `GinkgoT().Setenv`/`TempDir`,
  `MatchJSON`).
- `internal/agentrun`: unit-test the JSON-Lines parser (step/error/unknown/log
  lines) and the agent-binary resolution (env → PATH → not found) with a fake
  `kairos-agent` script that emits a scripted event stream and a chosen exit
  code.
- `internal/config`: assert `Model` → expected `#cloud-config` (user stage, ssh
  keys, extra fields merged).
- Migrated TUI tests come across with their files (e.g. the disk-pagination
  test).
- **E2E (hadron, later):** build an ISO shipping `kairos-installer`, boot it,
  and verify dispatch → TUI → `manual-install` → progress → installed system,
  plus the agent's embedded-fallback path. Tracked with the agent PR's e2e.

## Sequencing (relative to the kairos-agent PR)

1. **This repo** → working `kairos-installer` that drives `manual-install` and
   renders progress (this spec).
2. **kairos-agent PR (`feat/installer-dispatch`)** → add `go:embed` of the
   prebuilt `kairos-installer` as the dispatcher's fallback, and delete the
   agent's TUI code + `NewInteractiveInstallConfig`/`ExtraFields` +
   bubbletea/lipgloss deps. (Folded into the same PR per decision.)
3. **kairos-sdk** → move `EventInteractiveInstall` + `YAMLPrompt` into the SDK;
   update both consumers. (Separate, can land independently.)
4. **hadron e2e** on the assembled ISO.

## Open Questions / Risks

- **`pkg/config` parity:** the copied `MergeYAML`/`AddHeader` + direct
  `collector` call must reproduce `NewInteractiveInstallConfig`'s output exactly.
  Mitigation: a golden `#cloud-config` test comparing against the agent's
  current output for a fixed `Model`.
- **Provider bus drift:** the copied `internal/bus` could drift from the agent's.
  Acceptable short-term; the longer-term fix is the kairos-sdk move (step 3),
  after which both consume the SDK definition.
