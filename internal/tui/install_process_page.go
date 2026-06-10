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
func (p *installProcessPage) View() string                   { return "install (stub)" }
func (p *installProcessPage) Title() string                  { return "Installing" }
func (p *installProcessPage) Help() string                   { return "" }
func (p *installProcessPage) ID() string                     { return "install_process" }
func (p *installProcessPage) Abort()                         {}
