package debugbundle

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// defaultOutputDir is tmpfs on live media and is NOT globbed by `kairos-agent
// logs`, so writing the bundle here avoids recursion.
const defaultOutputDir = "/run/kairos"

// Log files the installer writes under /var/log/kairos (globbed by
// `kairos-agent logs` and listed in the stdlib fallback tarball).
const (
	// InstallerLog is the installer's own structured log.
	InstallerLog = "/var/log/kairos/installer.log"
	// AgentOutputLog is the full agent transcript (raw stdout + stderr)
	// captured during an install.
	AgentOutputLog = "/var/log/kairos/agent-output.log"
)

// OutputPath returns a timestamped bundle path under /run/kairos, falling back
// to the OS temp dir when /run/kairos is not writable.
func OutputPath(now time.Time) string {
	name := fmt.Sprintf("kairos-logs-%s.tar.gz", now.Format("20060102-150405"))
	if err := os.MkdirAll(defaultOutputDir, 0o755); err == nil {
		if f, err := os.CreateTemp(defaultOutputDir, ".writable"); err == nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
			return filepath.Join(defaultOutputDir, name)
		}
	}
	return filepath.Join(os.TempDir(), name)
}

// Generate builds the bundle at outputPath. When agentBin is non-empty it runs
// `kairos-agent logs --output <outputPath>` (which globs /var/log/kairos and
// gathers journald). If agentBin is empty, or that command fails, it falls back
// to a stdlib tarball of files.
func Generate(agentBin, outputPath string, files []string) error {
	if agentBin != "" {
		if err := exec.Command(agentBin, "logs", "--output", outputPath).Run(); err == nil {
			return nil
		}
	}
	return buildTarball(outputPath, files)
}

func buildTarball(outputPath string, files []string) error {
	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	for _, f := range files {
		// best-effort: skip unreadable files rather than failing the bundle.
		_ = addFile(tw, f)
	}
	return nil
}

func addFile(tw *tar.Writer, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Name:    filepath.Base(path),
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(data)
	return err
}
