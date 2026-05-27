// Command muxc is the entry point for the muxc CLI — a tmux session manager
// for Claude Code sessions. Build-time variables (Version, Commit, Date) are
// injected via -ldflags; see the Makefile build target.
package main

import (
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/monzim/muxc/internal/cli"
)

// Version, Commit, and Date are injected at build time via:
//
//	-X main.Version=$(VERSION) -X main.Commit=<git-sha> -X main.Date=<rfc3339>
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func main() {
	// Default logger: WARN+ to stderr (spec §17). Commands that load config
	// may replace this with a file-backed handler per [logging] settings.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})))

	// Propagate build info into the cli package so version/doctor commands
	// can display it without importing cmd/muxc (import cycle).
	cli.SetBuildInfo(Version, Commit, Date)

	// Install SIGINT handler: exit 130 (128 + signal number 2) per spec §16.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	go func() {
		<-sigCh
		os.Exit(130)
	}()

	err := cli.Execute()
	if err == nil {
		os.Exit(0)
	}

	// Map typed errors to exit codes per spec §16.
	var exitErr *cli.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.Code)
	}

	// Unclassified errors → exit 1.
	os.Exit(1)
}
