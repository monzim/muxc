// Package proc reads Linux /proc to walk process trees, measure memory,
// and identify the Claude Code process within a tmux session's process tree.
//
// Architecture invariant (CLAUDE.md): BuildTree is called ONCE per muxc
// invocation and the resulting *Tree is shared across all sessions. Do not
// re-scan /proc per session.
package proc

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Tree is a snapshot of the Linux process tree built from /proc at one point
// in time. It caches the PPID map and per-PID metadata so callers can query
// descendants, cmdlines, and RSS without repeated /proc reads.
type Tree struct {
	// children maps parent PID → slice of child PIDs.
	children map[int][]int
	// ppid maps PID → PPID.
	ppid map[int]int
	// procRoot is the root of the proc filesystem (normally "/proc").
	// Tests override this via BuildTreeFromRoot to use synthetic fixtures.
	procRoot string
}

// BuildTree walks /proc once, builds the full PPID child map, and returns
// a *Tree. If a PID disappears between listing and reading its stat file,
// it is silently skipped (processes may exit at any time).
//
// BuildTree calls BuildTreeFromRoot("/proc").
func BuildTree() (*Tree, error) {
	return BuildTreeFromRoot("/proc")
}

// BuildTreeFromRoot builds a Tree by reading the proc filesystem rooted at
// root. This is exposed so tests can provide a synthetic fixture directory.
func BuildTreeFromRoot(root string) (*Tree, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	t := &Tree{
		children: make(map[int][]int),
		ppid:     make(map[int]int),
		procRoot: root,
	}

	for _, e := range entries {
		// Only process entries whose names are all digits.
		name := e.Name()
		if !isAllDigits(name) {
			continue
		}
		pid, err := strconv.Atoi(name)
		if err != nil {
			continue
		}

		ppid, err := readPPID(root, pid)
		if err != nil {
			// Process may have exited; skip silently.
			continue
		}

		t.ppid[pid] = ppid
		t.children[ppid] = append(t.children[ppid], pid)
	}

	return t, nil
}

// readPPID reads /proc/<pid>/stat and returns the parent PID.
// The stat format is: "<pid> (<comm>) <state> <ppid> ..."
// The comm field can contain spaces and parentheses, so we find the LAST ')'
// in the line, then parse fields from there.
func readPPID(procRoot string, pid int) (int, error) {
	data, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}

	line := strings.TrimRight(string(data), "\n")

	// Find the last ')' — the end of the comm field.
	idx := strings.LastIndex(line, ")")
	if idx < 0 || idx+2 >= len(line) {
		return 0, os.ErrInvalid
	}

	// After the last ')' we have: " <state> <ppid> ..."
	rest := line[idx+1:]
	fields := strings.Fields(rest)
	// fields[0] = state, fields[1] = ppid
	if len(fields) < 2 {
		return 0, os.ErrInvalid
	}

	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, err
	}
	return ppid, nil
}

// Descendants returns all PIDs that are transitive children of rootPID in
// BFS order. rootPID itself is NOT included — the caller should add it if
// needed (e.g., to include the root process in an RSS sum).
//
// Returns nil if rootPID has no children in the tree.
func (t *Tree) Descendants(rootPID int) []int {
	var result []int
	queue := []int{rootPID}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		for _, child := range t.children[cur] {
			result = append(result, child)
			queue = append(queue, child)
		}
	}

	return result
}

// Cmdline returns the argv slice for pid as read from /proc/<pid>/cmdline
// (null-byte separated). The trailing empty entry produced by the terminal
// null byte is dropped.
//
// Returns nil, os.ErrNotExist if the file does not exist, so callers can
// detect dead processes.
func (t *Tree) Cmdline(pid int) ([]string, error) {
	path := filepath.Join(t.procRoot, strconv.Itoa(pid), "cmdline")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}

	if len(data) == 0 {
		return nil, nil
	}

	// Split on null byte.
	parts := bytes.Split(data, []byte{0})

	// Drop trailing empty part caused by the terminating null.
	if len(parts) > 0 && len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}

	args := make([]string, len(parts))
	for i, p := range parts {
		args[i] = string(p)
	}
	return args, nil
}

// isAllDigits returns true if s is non-empty and consists entirely of ASCII digits.
func isAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
