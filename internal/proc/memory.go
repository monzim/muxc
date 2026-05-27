// Package proc — RSS memory reading from /proc.
// v1 uses RSS from /proc/<pid>/status VmRSS field. PSS (from smaps_rollup)
// is more accurate for shared memory but deferred to v2. See spec §13.2.
package proc

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// RSS returns the Resident Set Size of pid in bytes, parsed from the
// "VmRSS:" line in /proc/<pid>/status (value in kB × 1024).
//
// Special cases:
//   - File missing → return 0, os.ErrNotExist (process exited between tree
//     build and this call).
//   - VmRSS line absent (kernel thread) → return 0, nil (spec §13.2).
//   - Other I/O errors → returned as-is.
func (t *Tree) RSS(pid int) (uint64, error) {
	path := filepath.Join(t.procRoot, strconv.Itoa(pid), "status")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, os.ErrNotExist
		}
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		// Line format: "VmRSS:\t    4096 kB"
		fields := strings.Fields(line)
		// fields[0] = "VmRSS:", fields[1] = numeric value, fields[2] = "kB"
		if len(fields) < 2 {
			return 0, errors.New("proc: malformed VmRSS line")
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, err
		}
		return kb * 1024, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}

	// VmRSS line not present — kernel thread or zero-memory process. Treat as 0.
	return 0, nil
}

// SumRSS sums RSS for all PIDs in the pids slice. Errors (e.g. process exited)
// are silently skipped — only bytes from successfully read PIDs are summed.
func SumRSS(t *Tree, pids []int) uint64 {
	var total uint64
	for _, pid := range pids {
		rss, err := t.RSS(pid)
		if err != nil {
			continue
		}
		total += rss
	}
	return total
}
