// internal/tui/debug_bundle_page.go
package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kairos-io/kairos-installer/internal/agentrun"
	"github.com/kairos-io/kairos-installer/internal/debugbundle"
)

type bundleState int

const (
	bundleCollecting bundleState = iota
	bundleReady
	bundleUSBPrompt
	bundleFailed
)

type debugBundlePage struct {
	once       sync.Once
	state      bundleState
	resultCh   chan bundleResult
	path       string
	urls       []string
	server     *debugbundle.Server
	usbMessage string
	errMsg     string
}

type bundleResult struct {
	path   string
	server *debugbundle.Server
	err    error
}

func newDebugBundlePage() *debugBundlePage {
	return &debugBundlePage{state: bundleCollecting, resultCh: make(chan bundleResult, 1)}
}

// CheckBundleMsg polls the build result channel.
type CheckBundleMsg struct{}

func (p *debugBundlePage) Init() tea.Cmd {
	p.once.Do(func() {
		go func() {
			p.resultCh <- buildBundle()
		}()
	})
	return func() tea.Msg { return CheckBundleMsg{} }
}

// buildBundle collects extras, generates the tarball, and starts the HTTP
// server. It runs off the bubbletea goroutine.
func buildBundle() bundleResult {
	agentBin := agentrun.ResolveAgentBin()

	redacted, _ := RenderRedactedCloudConfig(&mainModel)
	cmd := agentrun.Command(agentBin, "<config>", mainModel.source, mainModel.finishAction)
	ctx := debugbundle.Context{
		AgentBin:            agentBin,
		AgentArgs:           cmd.Args[1:],
		Disk:                mainModel.disk,
		Source:              mainModel.source,
		Version:             version,
		CloudConfigRedacted: redacted,
	}

	extras, _ := debugbundle.CollectExtras(debugbundle.ExecRunner{}, ctx, "/var/log/kairos/")
	out := debugbundle.OutputPath(time.Now())

	// Fallback tarball also includes the installer log if the agent path fails.
	files := append([]string{"/var/log/kairos/installer.log"}, extras...)
	if err := debugbundle.Generate(agentBin, out, files); err != nil {
		return bundleResult{err: err}
	}
	srv, _ := debugbundle.Serve(out)
	return bundleResult{path: out, server: srv}
}

func (p *debugBundlePage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch m := msg.(type) {
	case CheckBundleMsg:
		select {
		case res := <-p.resultCh:
			if res.err != nil {
				p.state = bundleFailed
				p.errMsg = res.err.Error()
				return p, nil
			}
			p.path = res.path
			p.server = res.server
			if res.server != nil {
				p.urls = res.server.URLs()
			}
			p.state = bundleReady
			return p, nil
		default:
			return p, tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return CheckBundleMsg{} })
		}
	case tea.KeyMsg:
		return p.handleKey(m)
	}
	return p, nil
}

func (p *debugBundlePage) handleKey(m tea.KeyMsg) (Page, tea.Cmd) {
	switch p.state {
	case bundleReady:
		if m.String() == "u" {
			p.state = bundleUSBPrompt
			p.usbMessage = "Plug in a USB drive, then press Enter to copy the bundle."
			return p, nil
		}
	case bundleUSBPrompt:
		if m.Type == tea.KeyEnter {
			p.usbMessage = copyToFirstRemovable(p.path)
			p.state = bundleReady
			return p, nil
		}
	}
	return p, nil
}

// copyToFirstRemovable copies the bundle to the first mounted removable
// partition, returning a user-facing status line.
func copyToFirstRemovable(path string) string {
	mounts, err := debugbundle.RemovableMounts()
	if err != nil || len(mounts) == 0 {
		return "No USB detected. Re-plug the drive and press Enter again."
	}
	dest, err := debugbundle.CopyTo(path, mounts[0].MountPoint)
	if err != nil {
		return "Copy failed: " + err.Error()
	}
	return "Copied to " + dest + " on " + mounts[0].Device
}

// formatRetrievalText renders the HTTP URLs and local path block.
func formatRetrievalText(path string, urls []string) string {
	var b strings.Builder
	b.WriteString("Retrieve over the network:\n")
	if len(urls) == 0 {
		b.WriteString("  (no usable network address detected)\n")
	} else {
		for _, u := range urls {
			b.WriteString("  curl -O " + u + "\n")
		}
	}
	fmt.Fprintf(&b, "\nLocal path: %s\n", path)
	return b.String()
}

func (p *debugBundlePage) View() string {
	switch p.state {
	case bundleCollecting:
		return "Collecting diagnostics and building debug bundle...\n"
	case bundleFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true).
			Render("Failed to build debug bundle: "+p.errMsg) + "\n"
	case bundleUSBPrompt:
		return p.usbMessage + "\n"
	default: // bundleReady
		s := "Debug bundle ready.\n\n" + formatRetrievalText(p.path, p.urls)
		if p.usbMessage != "" {
			s += "\n" + p.usbMessage + "\n"
		}
		return s
	}
}

func (p *debugBundlePage) Title() string { return "Debug Bundle" }

func (p *debugBundlePage) Help() string {
	if p.state == bundleReady {
		return "u: copy to USB • q/ctrl+c: quit"
	}
	return "Please wait..."
}

func (p *debugBundlePage) ID() string { return DebugBundlePageID }
