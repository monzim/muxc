// Package proc — Claude process identification heuristic.
// See spec §13.3 for the full identification algorithm.
package proc

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// IdentifyClaude walks the pane's process subtree (rootPID and its
// descendants) and applies the heuristic from spec §13.3 to find the
// Claude Code process:
//
//  1. argv[0] basename == "claude"
//  2. argv[0] basename == "node" AND any argv[i] (i>0) contains "/claude/"
//     as a substring, OR ends with "claude.js" / "cli.js" in a path that
//     also contains "claude"
//  3. /proc/<pid>/exe symlink basename matches claudeBin
//
// Among all matching PIDs, the lowest PID is returned (closest to pane PID
// by spawn order — spec §13.3). Returns (0, false) if nothing matches.
//
// Note: rootPID itself is considered as a candidate (post-v1.0 extension).
// Some tmux launch patterns — particularly `tmux new-session -d <cmd>`
// without a wrapping shell — produce a pane whose root process IS the
// command. Without including rootPID, those panes would report no Claude
// process even when the pane's only process is claude.
func (t *Tree) IdentifyClaude(rootPID int, claudeBin string) (claudePID int, found bool) {
	if claudeBin == "" {
		claudeBin = "claude"
	}

	// Include rootPID itself plus all its descendants in the candidate set.
	candidates := append([]int{rootPID}, t.Descendants(rootPID)...)

	var best int // 0 means no match yet

	for _, pid := range candidates {
		if matchesClaude(t, pid, claudeBin) {
			if best == 0 || pid < best {
				best = pid
			}
		}
	}

	if best == 0 {
		return 0, false
	}
	return best, true
}

// matchesClaude applies the three-rule heuristic to a single PID.
func matchesClaude(t *Tree, pid int, claudeBin string) bool {
	argv, err := t.Cmdline(pid)
	if err != nil || len(argv) == 0 {
		// Process exited or has no cmdline (kernel thread); skip.
		goto tryExe
	}

	// Rule 1: argv[0] basename is "claude".
	if filepath.Base(argv[0]) == "claude" {
		return true
	}

	// Rule 2: argv[0] is "node" and a later arg points into a claude path.
	if filepath.Base(argv[0]) == "node" {
		for i := 1; i < len(argv); i++ {
			arg := argv[i]
			if isClaudeNodeArg(arg) {
				return true
			}
		}
	}

tryExe:
	// Rule 3: /proc/<pid>/exe basename matches claudeBin.
	exePath := filepath.Join(t.procRoot, strconv.Itoa(pid), "exe")
	target, err := os.Readlink(exePath)
	if err == nil {
		if filepath.Base(target) == claudeBin {
			return true
		}
	}

	return false
}

// isClaudeNodeArg returns true if the argument looks like the Claude entry
// script, matching spec §13.3 rule 2:
//   - contains "/claude/" as a substring, OR
//   - ends with "claude.js", OR
//   - ends with "cli.js" and the path also contains "claude"
func isClaudeNodeArg(arg string) bool {
	if strings.Contains(arg, "/claude/") {
		return true
	}
	base := filepath.Base(arg)
	if base == "claude.js" {
		return true
	}
	if base == "cli.js" && strings.Contains(arg, "claude") {
		return true
	}
	return false
}
