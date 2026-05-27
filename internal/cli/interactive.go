// Package cli — interactive REPL menu (post-v1.0 extension).
//
// When `muxc` is invoked with no subcommand and no flags that require
// non-interactive output, runInteractive is the RunE wired on rootCmd. It
// prints a numbered list of available commands, reads the user's pick, runs
// it (prompting for any required arguments), then redraws the menu. The loop
// exits cleanly on 'q', 'quit', 'exit', or EOF.
//
// This is a deliberate deviation from spec §3 (which rejected "TUI dashboard
// (ncurses)") — see CLAUDE.md "Post-v1.0 additions". It is intentionally
// minimal: no ncurses, no ANSI cursor control, no live refresh. Just a plain
// numbered menu so it works equally well in a real terminal, a CI log, and
// piped fixture input from tests.
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/monzim/muxc/internal/tui"
)

// menuItem describes one entry in the REPL menu.
type menuItem struct {
	key  string // selection key, e.g. "1"
	name string // muxc subcommand name shown in the menu
	desc string // one-line description
	run  func(*replEnv) error
}

// replEnv carries the IO streams the REPL uses. A struct so tests can drive
// the loop with fake readers/writers without touching os.Stdin/Stdout.
type replEnv struct {
	cmd    *cobra.Command
	reader *bufio.Reader
	out    io.Writer
	err    io.Writer
}

// defaultMenuItems returns the standard interactive menu. Subcommands that
// inherently require an interactive terminal (completion shells, etc.) are
// not included.
func defaultMenuItems() []menuItem {
	return []menuItem{
		{"1", "ls", "list sessions", runMenuSimple("ls")},
		{"2", "new", "create a session in a project directory", runMenuNew},
		{"3", "attach", "attach to a session (one-way: detach returns to shell, not menu)", runMenuAttach},
		{"4", "kill", "kill a session", runMenuKill},
		{"5", "mem", "memory usage (sorted by RSS)", runMenuSimple("mem")},
		{"6", "info", "detailed session info", runMenuInfo},
		{"7", "doctor", "environment health check", runMenuSimple("doctor")},
		{"8", "version", "print version", runMenuSimple("version")},
	}
}

// runInteractive is the RunE for rootCmd when no subcommand was provided.
// Hooked up by init() in root.go.
//
// Routing:
//  1. If stdin and stdout are real terminals (and TERM != dumb), launch the
//     Bubble Tea TUI via tui.Run.
//  2. Otherwise (piped input, CI, dumb terminal), or if tui.Run returns
//     tui.ErrNoTTY, fall back to the plain-text REPL (runPlainREPL).
//
// The plain REPL stays exported-via-package so integration tests that pipe
// input to `muxc` keep working unchanged.
func runInteractive(cmd *cobra.Command, _ []string) error {
	if shouldUseTUI(cmd) {
		cfg, st, err := LoadContext(cmd)
		if err != nil {
			return err
		}
		err = tui.Run(cfg, st)
		if err == nil {
			return nil
		}
		if !errors.Is(err, tui.ErrNoTTY) {
			return err
		}
		// Fall through to plain REPL on ErrNoTTY (defensive — should not
		// usually happen since we already checked shouldUseTUI).
	}
	return runPlainREPL(cmd)
}

// shouldUseTUI returns true when the TUI is appropriate for the current
// invocation: both stdin and stdout must be terminals, and TERM must not be
// "dumb". A MUXC_NO_TUI env var lets users force the plain REPL.
func shouldUseTUI(cmd *cobra.Command) bool {
	if os.Getenv("MUXC_NO_TUI") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	in, okIn := cmd.InOrStdin().(*os.File)
	out, okOut := cmd.OutOrStdout().(*os.File)
	if !okIn || !okOut {
		return false
	}
	return isatty.IsTerminal(in.Fd()) && isatty.IsTerminal(out.Fd())
}

// runPlainREPL is the original numbered-menu REPL used as a fallback when no
// interactive terminal is available. Kept intact for piped-input tests and
// scripted use.
func runPlainREPL(cmd *cobra.Command) error {
	env := &replEnv{
		cmd:    cmd,
		reader: bufio.NewReader(cmd.InOrStdin()),
		out:    cmd.OutOrStdout(),
		err:    cmd.ErrOrStderr(),
	}
	items := defaultMenuItems()

	fmt.Fprintf(env.out, "muxc %s — interactive mode (type 'q' to quit)\n", buildInfo.version)

	for {
		fmt.Fprintln(env.out)
		fmt.Fprint(env.out, formatMenu(items))
		fmt.Fprintf(env.out, "Select (1-%d, q): ", len(items))

		line, err := env.reader.ReadString('\n')
		if err == io.EOF {
			fmt.Fprintln(env.out) // newline so the shell prompt lands cleanly
			return nil
		}
		if err != nil {
			return fmt.Errorf("interactive: read input: %w", err)
		}
		sel := parseInteractiveSelection(line)

		if sel == "" {
			// Empty input — just redraw the menu without complaint.
			continue
		}
		if isQuitWord(sel) {
			return nil
		}

		item := findMenuItem(items, sel)
		if item == nil {
			fmt.Fprintf(env.err, "invalid choice %q — pick 1-%d or 'q' to quit\n", sel, len(items))
			continue
		}

		if err := item.run(env); err != nil {
			fmt.Fprintf(env.err, "error: %s\n", err)
		}

		if !pauseForEnter(env) {
			return nil
		}
	}
}

// formatMenu renders the numbered menu as a single string. Pure function for
// easy unit testing.
func formatMenu(items []menuItem) string {
	var b strings.Builder
	// Pad command names to a consistent width for vertical alignment.
	maxName := 0
	for _, it := range items {
		if len(it.name) > maxName {
			maxName = len(it.name)
		}
	}
	for _, it := range items {
		fmt.Fprintf(&b, "  %s) %-*s  %s\n", it.key, maxName, it.name, it.desc)
	}
	return b.String()
}

// parseInteractiveSelection normalises a user-typed line into a comparable
// selection token. Returns "" on empty / whitespace-only input.
func parseInteractiveSelection(line string) string {
	return strings.ToLower(strings.TrimSpace(line))
}

// isQuitWord reports whether sel asks the REPL to exit.
func isQuitWord(sel string) bool {
	switch sel {
	case "q", "quit", "exit":
		return true
	}
	return false
}

// findMenuItem returns the matching menuItem, or nil if none matches. The
// selection can be either the numeric key ("1") or the command name ("ls").
func findMenuItem(items []menuItem, sel string) *menuItem {
	for i := range items {
		if items[i].key == sel || items[i].name == sel {
			return &items[i]
		}
	}
	return nil
}

// pauseForEnter blocks until the user presses Enter so output isn't immediately
// scrolled off-screen by the next menu redraw. Returns false on EOF (REPL exit).
func pauseForEnter(env *replEnv) bool {
	fmt.Fprint(env.out, "\nPress Enter to continue (or 'q' to quit)...")
	line, err := env.reader.ReadString('\n')
	if err == io.EOF {
		fmt.Fprintln(env.out)
		return false
	}
	if isQuitWord(parseInteractiveSelection(line)) {
		return false
	}
	return true
}

// ---- Command runners --------------------------------------------------------

// runMenuSimple builds a runner for commands that need no extra arguments.
// It invokes the subcommand by name through the root command so cobra's
// normal flag-and-context plumbing applies.
func runMenuSimple(subcommand string) func(*replEnv) error {
	return func(env *replEnv) error {
		return invokeSubcommand(env, subcommand)
	}
}

// runMenuNew prompts for a project path (default: current directory) and an
// optional --no-launch toggle, then invokes `muxc new`.
func runMenuNew(env *replEnv) error {
	cwd, _ := os.Getwd()
	defaultPath := cwd
	if defaultPath == "" {
		defaultPath = "."
	}
	path := promptLine(env, fmt.Sprintf("project path [%s]: ", defaultPath), defaultPath)
	if path == "" {
		fmt.Fprintln(env.out, "cancelled")
		return nil
	}

	noLaunch := promptYesNo(env, "skip launching Claude? [y/N]: ", false)

	args := []string{"new", path}
	if noLaunch {
		args = append(args, "--no-launch")
	}
	return invokeSubcommand(env, args...)
}

// runMenuAttach picks an attach target via the existing pickFromRows helper
// in attach.go, or runs `attach` directly when zero/one sessions exist.
//
// Attach is a one-way trip: tmux.Attach uses syscall.Exec which replaces the
// muxc process. The REPL will not return from this call when attach succeeds.
func runMenuAttach(env *replEnv) error {
	fmt.Fprintln(env.out, "Note: detaching from tmux returns you to the shell, not this menu.")
	return invokeSubcommand(env, "attach")
}

// runMenuKill prompts for a kill mode (by name / --idle / --all / --stale)
// and, where applicable, the per-mode argument. Confirmation is always on
// in the REPL — `--yes` is not auto-applied.
func runMenuKill(env *replEnv) error {
	fmt.Fprintln(env.out, "kill mode:")
	fmt.Fprintln(env.out, "  1) by name")
	fmt.Fprintln(env.out, "  2) --idle <duration> (e.g. 2h, 30m)")
	fmt.Fprintln(env.out, "  3) --all (every muxc session)")
	fmt.Fprintln(env.out, "  4) --stale (prune state entries for dead sessions)")
	mode := promptLine(env, "Mode [1-4]: ", "")
	switch mode {
	case "1":
		name := promptLine(env, "session name: ", "")
		if name == "" {
			fmt.Fprintln(env.out, "cancelled")
			return nil
		}
		return invokeSubcommand(env, "kill", name)
	case "2":
		dur := promptLine(env, "idle threshold (e.g. 2h): ", "")
		if dur == "" {
			fmt.Fprintln(env.out, "cancelled")
			return nil
		}
		return invokeSubcommand(env, "kill", "--idle", dur)
	case "3":
		return invokeSubcommand(env, "kill", "--all")
	case "4":
		return invokeSubcommand(env, "kill", "--stale")
	default:
		fmt.Fprintln(env.out, "invalid mode — cancelled")
		return nil
	}
}

// runMenuInfo prompts for a session name and optional transcript-lines count,
// then invokes `muxc info`.
func runMenuInfo(env *replEnv) error {
	name := promptLine(env, "session name: ", "")
	if name == "" {
		fmt.Fprintln(env.out, "cancelled")
		return nil
	}
	linesStr := promptLine(env, "transcript lines [0]: ", "0")
	args := []string{"info", name}
	if n, err := strconv.Atoi(linesStr); err == nil && n > 0 {
		args = append(args, "--transcript-lines", strconv.Itoa(n))
	}
	return invokeSubcommand(env, args...)
}

// invokeSubcommand runs a muxc subcommand by re-entering the root command
// with fresh args. The persistent --json flag is reset before each invocation
// so REPL iterations don't carry sticky state.
func invokeSubcommand(env *replEnv, args ...string) error {
	root := env.cmd.Root()

	// Reset persistent flags that may have been mutated in a prior iteration.
	if jsonF := root.PersistentFlags().Lookup("json"); jsonF != nil {
		_ = jsonF.Value.Set("false")
		jsonF.Changed = false
	}

	root.SetArgs(args)
	return root.Execute()
}

// promptLine asks for a single line of input. Returns defaultVal if the user
// presses Enter without typing. Returns "" on EOF.
func promptLine(env *replEnv, label, defaultVal string) string {
	fmt.Fprint(env.out, label)
	line, err := env.reader.ReadString('\n')
	if err == io.EOF {
		return ""
	}
	val := strings.TrimSpace(line)
	if val == "" {
		return defaultVal
	}
	return val
}

// promptYesNo prompts for a y/N answer. Empty input returns defaultVal.
func promptYesNo(env *replEnv, label string, defaultVal bool) bool {
	v := strings.ToLower(promptLine(env, label, ""))
	switch v {
	case "":
		return defaultVal
	case "y", "yes":
		return true
	}
	return false
}
