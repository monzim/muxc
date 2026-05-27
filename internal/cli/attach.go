// Package cli — attach subcommand.
// See spec §11.3 for the full behavioral specification.
//
// Implementation notes (spec §25 open Q3): the numbered prompt uses bufio.Scanner
// (no extra dep). fzf picker pipes tab-separated "name\tproject\tidle" lines per
// spec §25 open Q2. syscall.Exec is wrapped behind tmuxAttacher so tests can
// inject a fake without actually replacing the process.
package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/monzim/muxc/internal/render"
	"github.com/monzim/muxc/internal/tmux"
)

// tmuxAttacher is the interface wrapping tmux.Attach so tests can substitute a
// fake that records calls without invoking syscall.Exec (CLAUDE.md invariant).
type tmuxAttacher interface {
	Attach(name string, detach bool) error
}

// realAttacher is the production implementation: delegates directly to tmux.Attach
// which calls syscall.Exec and never returns on success.
type realAttacher struct{}

func (realAttacher) Attach(name string, detach bool) error {
	return tmux.Attach(name, detach)
}

// attacher is the package-level attacher used by runAttach. Tests replace this.
var attacher tmuxAttacher = realAttacher{}

// attachCmd attaches to an existing muxc-managed tmux session.
var attachCmd = &cobra.Command{
	Use:   "attach [name]",
	Short: "Attach to a Claude Code session",
	Long: `Attach to a running muxc-managed tmux session.

If no name is given, presents a picker (fzf if available and enabled in config,
else a numbered prompt). Uses syscall.Exec to replace muxc with tmux so the
user lands directly in tmux without an extra shell layer.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAttach,
}

func init() {
	attachCmd.Flags().Bool("no-detach", false, "do not pass -d to tmux attach (share with other clients)")
}

// runAttach is the RunE implementation for attachCmd. Separated for testability.
func runAttach(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, st, err := LoadContext(cmd)
	if err != nil {
		return exitErr(3, err.Error())
	}

	noDetach, _ := cmd.Flags().GetBool("no-detach")
	// detach = true means we pass -d to tmux (default behaviour per spec §9 attach.force_detach).
	detach := !noDetach

	// ── Case A: explicit name argument ───────────────────────────────────────
	if len(args) == 1 {
		fullName, err := ResolveSessionName(ctx, cfg, args[0])
		if err != nil {
			// §19-formatted message; exit 2 per spec §16.
			return exitErrf(2, "muxc: tmux session %q not found\n  list available sessions with `muxc ls`", args[0])
		}

		// Update last_attached_at BEFORE exec (exec never returns on success).
		if entry, ok := st.Sessions[fullName]; ok {
			entry.LastAttachedAt = time.Now().UTC()
			st.Upsert(fullName, entry)
			_ = st.Write(cfg.Paths.StateFile) // best-effort; ignore write errors
		}

		return attacher.Attach(fullName, detach)
	}

	// ── Case B: no argument — list sessions and present picker ───────────────
	// Picker shows muxc-managed sessions plus external sessions running Claude.
	rows, err := Gather(ctx, cfg, st, ExternalWithClaude)
	if err != nil {
		return exitErr(1, fmt.Sprintf("muxc: gather sessions: %s", err))
	}

	if len(rows) == 0 {
		return exitErrf(2, "muxc: no muxc sessions found, create one with `muxc new <path>`")
	}

	SortRows(rows, "name")

	var chosen string

	switch {
	case len(rows) == 1:
		// Single session → attach directly without prompting.
		chosen = rows[0].Name

	case cfg.Attach.FzfPicker:
		// Try fzf; fall through to numbered prompt if fzf not on PATH.
		fzfPath, fzfErr := exec.LookPath("fzf")
		if fzfErr == nil {
			selected, cancelled, err := runFzfPicker(fzfPath, rows)
			if err != nil {
				return exitErr(1, fmt.Sprintf("muxc: fzf: %s", err))
			}
			if cancelled {
				// User pressed Escape / Ctrl-C in fzf → exit 0 silently.
				return nil
			}
			chosen = selected
			break
		}
		// fzf not found — fall through to numbered prompt.
		fallthrough

	default:
		// Numbered prompt.
		idx, err := pickFromRows(rows, os.Stdin, os.Stderr)
		if err != nil {
			return exitErr(1, fmt.Sprintf("muxc: prompt: %s", err))
		}
		if idx < 0 {
			// User cancelled (empty input or out-of-range) → exit 0 silently.
			return nil
		}
		chosen = rows[idx].Name
	}

	// Update last_attached_at BEFORE exec.
	if entry, ok := st.Sessions[chosen]; ok {
		entry.LastAttachedAt = time.Now().UTC()
		st.Upsert(chosen, entry)
		_ = st.Write(cfg.Paths.StateFile)
	}

	return attacher.Attach(chosen, detach)
}

// pickFromRows prints a numbered list of sessions to out and reads a choice from
// in. It returns the 0-based index of the chosen row.
//
// Returns -1 (no error) if the user cancels: empty line, non-numeric input, or
// an out-of-range number. EOF is treated as cancel.
//
// Spec §11.3 numbered prompt format:
//
//  1. muxc-my145    ~/code/my145         idle 5m
//  2. muxc-iar      ~/code/iar-website   idle 2h11m
//     Select [1-N]:
func pickFromRows(rows []SessionRow, in io.Reader, out io.Writer) (int, error) {
	for i, r := range rows {
		fmt.Fprintf(out, "%d) %s\n", i+1, formatPickerLine(r))
	}
	fmt.Fprintf(out, "Select [1-%d]: ", len(rows))

	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		// EOF → cancel.
		return -1, nil
	}
	line := strings.TrimSpace(sc.Text())
	if line == "" {
		return -1, nil
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(rows) {
		return -1, nil
	}
	return n - 1, nil
}

// formatPickerLine returns one human-readable line for a session row used in the
// numbered prompt and fzf input. Format:
//
//	<name padded to 20>  <project padded to 30>  idle <idleDuration>
//
// Spec §11.3 / §25 open Q2: "name\tproject\tidle" for fzf; we use a padded
// human-readable variant for the numbered prompt and the same tab-separated
// fields for fzf.
func formatPickerLine(row SessionRow) string {
	idleDur := time.Duration(row.IdleSeconds) * time.Second
	idleStr := "idle " + render.Duration(idleDur)

	project := row.ProjectPath
	if project == "" {
		project = "(unknown)"
	}
	// Left-truncate long paths.
	if len([]rune(project)) > 30 {
		project = render.LeftTruncate(project, 30)
	}

	return fmt.Sprintf("%-20s  %-30s  %s", row.Name, project, idleStr)
}

// formatFzfInput builds the multi-line tab-separated string piped to fzf stdin.
// Each line is: "<name>\t<project>\t<idle>".
// Lines end with \n. Spec §11.3 / §25 open Q2.
func formatFzfInput(rows []SessionRow) string {
	var b strings.Builder
	for _, r := range rows {
		idleDur := time.Duration(r.IdleSeconds) * time.Second
		project := r.ProjectPath
		if project == "" {
			project = "(unknown)"
		}
		b.WriteString(r.Name)
		b.WriteByte('\t')
		b.WriteString(project)
		b.WriteByte('\t')
		b.WriteString("idle ")
		b.WriteString(render.Duration(idleDur))
		b.WriteByte('\n')
	}
	return b.String()
}

// runFzfPicker spawns fzf with the session list piped to its stdin.
// Returns (selectedName, cancelled, err).
// cancelled is true when fzf exits non-zero (user pressed Escape / Ctrl-C).
func runFzfPicker(fzfPath string, rows []SessionRow) (string, bool, error) {
	input := formatFzfInput(rows)

	fzfCmd := exec.Command(fzfPath, "--with-nth=1,2,3", "--delimiter=\t")
	fzfCmd.Stdin = strings.NewReader(input)
	fzfCmd.Stderr = os.Stderr

	var stdout bytes.Buffer
	fzfCmd.Stdout = &stdout

	if err := fzfCmd.Run(); err != nil {
		// fzf exits with code 130 (or 1) on cancel — treat as cancel, not error.
		return "", true, nil
	}

	// The selected line is the full tab-separated line; the first field is the name.
	line := strings.TrimRight(stdout.String(), "\n")
	if line == "" {
		return "", true, nil
	}
	parts := strings.SplitN(line, "\t", 2)
	name := strings.TrimSpace(parts[0])
	if name == "" {
		return "", true, nil
	}
	return name, false, nil
}
