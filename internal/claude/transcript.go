// Package claude — JSONL transcript reading.
// Claude Code transcripts are append-only JSONL files; muxc reads but never
// writes them (spec §2). The schema is not stable across Claude versions (§14.2).
package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

// Entry is a single transcript message extracted for display by `muxc info`.
// tool_use/tool_result entries are skipped for tail purposes (spec §11.6).
type Entry struct {
	Role    string
	Content string
}

// rawEntry mirrors the Claude JSONL line format. The schema is intentionally
// loose because it is not stable across Claude versions (spec §14.2).
type rawEntry struct {
	Type    string `json:"type"`
	Role    string `json:"role,omitempty"`
	Message struct {
		Role    string `json:"role,omitempty"`
		Content any    `json:"content,omitempty"`
	} `json:"message,omitempty"`
	Content string `json:"content,omitempty"`
}

// maxScannerBuf is 1 MB — transcripts often have very long lines (base64
// images, large code blocks). The default 64 KB bufio.Scanner buffer is
// too small for real-world transcripts.
const maxScannerBuf = 1 << 20 // 1 MB

// maxContentRunes is the maximum length of an Entry.Content string. Content
// longer than this is truncated and the Unicode ellipsis "…" is appended.
// Uses runes (not bytes) for correct Unicode handling.
const maxContentRunes = 200

// maxTailEntries is the hard cap on n for TailTranscript (spec §11.6).
const maxTailEntries = 100

// ExtractSessionName reads up to the first 50 lines of the JSONL at path
// and returns the session name by probing known name-bearing fields in order:
// "name", "sessionName", "session_name", "title" (spec §14.2).
//
// Returns ("", nil) if no name field is found in the first 50 lines.
// Corrupt JSON lines are skipped silently (spec §14.3).
func ExtractSessionName(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, maxScannerBuf)
	scanner.Buffer(buf, maxScannerBuf)

	// Probe fields in priority order (spec §14.2).
	nameFields := []string{"name", "sessionName", "session_name", "title"}

	lineCount := 0
	for scanner.Scan() {
		if lineCount >= 50 {
			break
		}
		lineCount++

		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			// Corrupt line — skip silently.
			continue
		}

		for _, field := range nameFields {
			if v, ok := obj[field]; ok {
				if s, ok := v.(string); ok && s != "" {
					return s, nil
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", nil
}

// TailTranscript reads the last n entries from the JSONL transcript at path.
// n is clamped to maxTailEntries (100). tool_use/tool_result entries are
// always skipped (spec §11.6). Returns nil, nil when n <= 0.
//
// Content is Unicode-truncated to maxContentRunes (200) and "…" appended if
// truncated. Corrupt JSON lines are skipped silently (spec §14.3).
func TailTranscript(path string, n int) ([]Entry, error) {
	if n <= 0 {
		return nil, nil
	}
	if n > maxTailEntries {
		n = maxTailEntries
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, maxScannerBuf)
	scanner.Buffer(buf, maxScannerBuf)

	// Rolling ring buffer of size n.
	ring := make([]Entry, n)
	head := 0  // index where the next entry will be written
	count := 0 // total valid entries seen

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var raw rawEntry
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			// Corrupt line — skip silently.
			continue
		}

		// Skip tool_use and tool_result entirely (spec §11.6).
		if raw.Type == "tool_use" || raw.Type == "tool_result" {
			continue
		}

		role := deriveRole(&raw)
		content := deriveContent(&raw)
		content = truncateRunes(content, maxContentRunes)

		ring[head] = Entry{Role: role, Content: content}
		head = (head + 1) % n
		count++
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if count == 0 {
		return nil, nil
	}

	// Reconstruct in order from the ring buffer.
	// If we have seen fewer than n entries, they are contiguous starting at 0.
	var result []Entry
	if count <= n {
		result = make([]Entry, count)
		copy(result, ring[:count])
	} else {
		// Ring wrapped: oldest entry is at `head`.
		result = make([]Entry, n)
		for i := 0; i < n; i++ {
			result[i] = ring[(head+i)%n]
		}
	}

	return result, nil
}

// deriveRole extracts the best-effort role string from a raw entry.
// Priority: top-level Role → Message.Role → Type.
func deriveRole(raw *rawEntry) string {
	if raw.Role != "" {
		return raw.Role
	}
	if raw.Message.Role != "" {
		return raw.Message.Role
	}
	return raw.Type
}

// deriveContent extracts the best-effort content string from a raw entry.
//
// Priority:
//  1. Message.Content as string
//  2. Message.Content as []any (structured content blocks — Claude wraps text
//     in {"type":"text","text":"..."} objects); concatenate text blocks.
//  3. Top-level Content string.
func deriveContent(raw *rawEntry) string {
	if raw.Message.Content != nil {
		switch v := raw.Message.Content.(type) {
		case string:
			if v != "" {
				return v
			}
		case []any:
			return extractTextBlocks(v)
		}
	}
	return raw.Content
}

// extractTextBlocks concatenates the "text" field from every block whose
// "type" is "text" in a Claude structured content array.
func extractTextBlocks(blocks []any) string {
	var sb strings.Builder
	for _, b := range blocks {
		m, ok := b.(map[string]any)
		if !ok {
			continue
		}
		t, _ := m["type"].(string)
		if t != "text" {
			continue
		}
		text, _ := m["text"].(string)
		sb.WriteString(text)
	}
	return sb.String()
}

// truncateRunes truncates s to at most max runes, appending "…" if truncated.
// This is Unicode-safe — it counts runes, not bytes.
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
