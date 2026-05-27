package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- EncodeProjectPath ------------------------------------------------------

func TestEncodeProjectPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "typical absolute path",
			input: "/home/user/proj",
			want:  "-home-user-proj",
		},
		{
			name:  "root path",
			input: "/",
			want:  "-",
		},
		{
			name:  "trailing slash trimmed",
			input: "/home/user/proj/",
			want:  "-home-user-proj",
		},
		{
			name:  "multiple trailing slashes trimmed",
			input: "/home/user/proj///",
			want:  "-home-user-proj",
		},
		{
			name:  "single level path",
			input: "/tmp",
			want:  "-tmp",
		},
		{
			name:  "deep path",
			input: "/home/monzim/code/my145",
			want:  "-home-monzim-code-my145",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EncodeProjectPath(tc.input)
			if got != tc.want {
				t.Errorf("EncodeProjectPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---- LatestSession ----------------------------------------------------------

func TestLatestSession_ReturnsLatest(t *testing.T) {
	projectsRoot := filepath.Join("testdata", "projects")
	projDir := filepath.Join(projectsRoot, "-home-user-proj")

	// Git doesn't preserve mtimes — on a fresh checkout (CI) all testdata
	// files share the same timestamp, so LatestSession's "most recent" pick
	// is arbitrary. Stamp them explicitly so the assertion is robust.
	mtimes := map[string]time.Time{
		"session-aaaa1111.jsonl": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		"session-bbbb2222.jsonl": time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		"session-cccc3333.jsonl": time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
	}
	for name, mtime := range mtimes {
		if err := os.Chtimes(filepath.Join(projDir, name), mtime, mtime); err != nil {
			t.Fatalf("set mtime for %s: %v", name, err)
		}
	}

	session, err := LatestSession(projectsRoot, "/home/user/proj")
	if err != nil {
		t.Fatalf("LatestSession: %v", err)
	}
	if session == nil {
		t.Fatal("expected a session, got nil")
	}

	// session-cccc3333.jsonl has the most recent mtime (2026-03-01).
	if session.ID != "session-cccc3333" {
		t.Errorf("expected session-cccc3333 (most recent), got %q", session.ID)
	}

	if session.TranscriptPath == "" {
		t.Error("TranscriptPath should be set")
	}
	if session.ModTime.IsZero() {
		t.Error("ModTime should be set")
	}
	if session.Size <= 0 {
		t.Error("Size should be positive")
	}
}

func TestLatestSession_MissingDir_ReturnsNil(t *testing.T) {
	session, err := LatestSession("/nonexistent/projects", "/home/user/proj")
	if err != nil {
		t.Fatalf("LatestSession on missing dir: want nil error, got %v", err)
	}
	if session != nil {
		t.Errorf("expected nil session for missing dir, got %+v", session)
	}
}

func TestLatestSession_EmptyDir_ReturnsNil(t *testing.T) {
	projectsRoot := filepath.Join("testdata", "projects")
	// -home-user-empty exists but contains no .jsonl files.
	session, err := LatestSession(projectsRoot, "/home/user/empty")
	if err != nil {
		t.Fatalf("LatestSession on empty dir: want nil error, got %v", err)
	}
	if session != nil {
		t.Errorf("expected nil session for dir with no jsonl files, got %+v", session)
	}
}

func TestLatestSession_SessionFields(t *testing.T) {
	projectsRoot := filepath.Join("testdata", "projects")
	// session-aaaa1111.jsonl (oldest) has a `name` field: "auth-refactor".
	// We need to specifically request the oldest file. Create a tmp dir with only that one file.
	tmpRoot := t.TempDir()
	projDir := filepath.Join(tmpRoot, "-home-user-testonly")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Copy session-aaaa1111.jsonl content.
	src := filepath.Join("testdata", "projects", "-home-user-proj", "session-aaaa1111.jsonl")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(projDir, "session-aaaa1111.jsonl")
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
	// Set a known mtime.
	mtime := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(dst, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	session, err := LatestSession(tmpRoot, "/home/user/testonly")
	if err != nil {
		t.Fatalf("LatestSession: %v", err)
	}
	if session == nil {
		t.Fatal("expected session, got nil")
	}
	if session.ID != "session-aaaa1111" {
		t.Errorf("ID: want session-aaaa1111, got %q", session.ID)
	}
	if session.Name != "auth-refactor" {
		t.Errorf("Name: want auth-refactor, got %q", session.Name)
	}
	_ = projectsRoot
}

func TestLatestSession_FallbackName(t *testing.T) {
	// session-cccc3333.jsonl has no name in first 50 lines → fallback to truncated ID.
	tmpRoot := t.TempDir()
	projDir := filepath.Join(tmpRoot, "-home-user-fallback")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join("testdata", "projects", "-home-user-proj", "session-cccc3333.jsonl")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(projDir, "session-cccc3333.jsonl")
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}

	session, err := LatestSession(tmpRoot, "/home/user/fallback")
	if err != nil {
		t.Fatalf("LatestSession: %v", err)
	}
	if session == nil {
		t.Fatal("expected session, got nil")
	}
	// Name should fall back to first 8 chars of ID + "…"
	if !strings.HasPrefix(session.Name, "session-") {
		t.Errorf("fallback name should start with id prefix, got %q", session.Name)
	}
	if !strings.Contains(session.Name, "…") {
		t.Errorf("fallback name should contain ellipsis, got %q", session.Name)
	}
}

// ---- ExtractSessionName -----------------------------------------------------

func TestExtractSessionName_NameFieldOnLine3(t *testing.T) {
	path := filepath.Join("testdata", "projects", "-home-user-proj", "session-aaaa1111.jsonl")
	name, err := ExtractSessionName(path)
	if err != nil {
		t.Fatalf("ExtractSessionName: %v", err)
	}
	if name != "auth-refactor" {
		t.Errorf("want auth-refactor, got %q", name)
	}
}

func TestExtractSessionName_CorruptLinesThenSessionName(t *testing.T) {
	// session-bbbb2222.jsonl: first 5 lines corrupt/no name, line 6 has sessionName.
	path := filepath.Join("testdata", "projects", "-home-user-proj", "session-bbbb2222.jsonl")
	name, err := ExtractSessionName(path)
	if err != nil {
		t.Fatalf("ExtractSessionName: %v", err)
	}
	if name != "code-review-session" {
		t.Errorf("want code-review-session, got %q", name)
	}
}

func TestExtractSessionName_NameOnlyAfter50Lines(t *testing.T) {
	// session-cccc3333.jsonl: 60 lines total, name field only on line 60 → not found.
	path := filepath.Join("testdata", "projects", "-home-user-proj", "session-cccc3333.jsonl")
	name, err := ExtractSessionName(path)
	if err != nil {
		t.Fatalf("ExtractSessionName: %v", err)
	}
	if name != "" {
		t.Errorf("expected empty name (name field beyond 50 lines), got %q", name)
	}
}

func TestExtractSessionName_MissingFile(t *testing.T) {
	_, err := ExtractSessionName("/nonexistent/path/file.jsonl")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestExtractSessionName_TitleField(t *testing.T) {
	// Test with a file that has "title" field.
	f, err := os.CreateTemp(t.TempDir(), "*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_, _ = f.WriteString(`{"type":"meta","title":"my-title-session"}` + "\n")

	name, err := ExtractSessionName(f.Name())
	if err != nil {
		t.Fatalf("ExtractSessionName: %v", err)
	}
	if name != "my-title-session" {
		t.Errorf("want my-title-session, got %q", name)
	}
}

func TestExtractSessionName_SessionNameUnderscore(t *testing.T) {
	// Test with "session_name" (underscore variant).
	f, err := os.CreateTemp(t.TempDir(), "*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_, _ = f.WriteString(`{"type":"meta","session_name":"underscore-session"}` + "\n")

	name, err := ExtractSessionName(f.Name())
	if err != nil {
		t.Fatalf("ExtractSessionName: %v", err)
	}
	if name != "underscore-session" {
		t.Errorf("want underscore-session, got %q", name)
	}
}

// ---- TailTranscript ---------------------------------------------------------

func TestTailTranscript_ZeroN(t *testing.T) {
	entries, err := TailTranscript("testdata/tail_basic.jsonl", 0)
	if err != nil {
		t.Fatalf("TailTranscript(n=0): %v", err)
	}
	if entries != nil {
		t.Errorf("TailTranscript(n=0): want nil, got %v", entries)
	}
}

func TestTailTranscript_NegativeN(t *testing.T) {
	entries, err := TailTranscript("testdata/tail_basic.jsonl", -5)
	if err != nil {
		t.Fatalf("TailTranscript(n=-5): %v", err)
	}
	if entries != nil {
		t.Errorf("TailTranscript(n=-5): want nil, got %v", entries)
	}
}

func TestTailTranscript_NCappedAt100(t *testing.T) {
	// Create a file with 110 entries; tail with n=200 should return at most 100.
	f, err := os.CreateTemp(t.TempDir(), "*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	for i := 0; i < 110; i++ {
		line := `{"type":"user","role":"user","message":{"role":"user","content":"msg"}}` + "\n"
		if _, err := f.WriteString(line); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	entries, err := TailTranscript(f.Name(), 200)
	if err != nil {
		t.Fatalf("TailTranscript(n=200): %v", err)
	}
	if len(entries) != 100 {
		t.Errorf("n=200 should be capped at 100; got %d entries", len(entries))
	}
}

func TestTailTranscript_BasicExchange(t *testing.T) {
	// tail_basic.jsonl has 5 entries; request last 2.
	entries, err := TailTranscript("testdata/tail_basic.jsonl", 2)
	if err != nil {
		t.Fatalf("TailTranscript: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d: %v", len(entries), entries)
	}

	// Last two entries: "How are you?" assistant reply, then "Last message".
	// The rolling buffer returns the last 2 in file order.
	if entries[0].Content != "I am doing well, thanks for asking." {
		t.Errorf("entries[0].Content: want %q, got %q",
			"I am doing well, thanks for asking.", entries[0].Content)
	}
	if entries[1].Content != "Last message" {
		t.Errorf("entries[1].Content: want %q, got %q", "Last message", entries[1].Content)
	}
}

func TestTailTranscript_ToolEntriesSkipped(t *testing.T) {
	// tail_with_tools.jsonl: user, tool_use (skip), tool_result (skip), assistant.
	entries, err := TailTranscript("testdata/tail_with_tools.jsonl", 10)
	if err != nil {
		t.Fatalf("TailTranscript: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries (tool lines skipped), got %d: %v", len(entries), entries)
	}

	for _, e := range entries {
		if e.Role == "tool_use" || e.Role == "tool_result" {
			t.Errorf("tool entries should be skipped, found role=%q", e.Role)
		}
	}
}

func TestTailTranscript_StructuredContentBlocks(t *testing.T) {
	entries, err := TailTranscript("testdata/tail_structured.jsonl", 10)
	if err != nil {
		t.Fatalf("TailTranscript: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}

	// First entry: two text blocks concatenated → "hello world"
	if entries[0].Content != "hello world" {
		t.Errorf("structured blocks: want %q, got %q", "hello world", entries[0].Content)
	}
	// Second entry: single text block → "got it"
	if entries[1].Content != "got it" {
		t.Errorf("structured blocks: want %q, got %q", "got it", entries[1].Content)
	}
}

func TestTailTranscript_LongContentTruncated(t *testing.T) {
	entries, err := TailTranscript("testdata/tail_long.jsonl", 1)
	if err != nil {
		t.Fatalf("TailTranscript: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}

	content := entries[0].Content
	runes := []rune(content)
	// Should be maxContentRunes+1 (200 runes + "…" which is 1 rune).
	if len(runes) != maxContentRunes+1 {
		t.Errorf("truncated content: want %d runes, got %d", maxContentRunes+1, len(runes))
	}
	if !strings.HasSuffix(content, "…") {
		t.Errorf("truncated content must end with ellipsis, got %q", content)
	}
}

func TestTailTranscript_CorruptLinesSkipped(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	content := `not json at all
{"type":"user","role":"user","message":{"role":"user","content":"valid"}}
{broken json}
{"type":"assistant","role":"assistant","message":{"role":"assistant","content":"also valid"}}
`
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	entries, err := TailTranscript(f.Name(), 10)
	if err != nil {
		t.Fatalf("TailTranscript with corrupt lines: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 valid entries, got %d: %v", len(entries), entries)
	}
}

func TestTailTranscript_RoleDerivation(t *testing.T) {
	// Test that role is derived from the correct field.
	f, err := os.CreateTemp(t.TempDir(), "*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Top-level role takes priority.
	_, _ = f.WriteString(`{"type":"user","role":"user","message":{"role":"user","content":"hello"}}` + "\n")
	// No top-level role, use message.role.
	_, _ = f.WriteString(`{"type":"message","message":{"role":"assistant","content":"hi"}}` + "\n")
	// Neither top-level nor message role — fall back to type.
	_, _ = f.WriteString(`{"type":"notification","message":{"content":"ping"}}` + "\n")
	f.Close()

	entries, err := TailTranscript(f.Name(), 10)
	if err != nil {
		t.Fatalf("TailTranscript: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	if entries[0].Role != "user" {
		t.Errorf("[0] role: want user, got %q", entries[0].Role)
	}
	if entries[1].Role != "assistant" {
		t.Errorf("[1] role: want assistant, got %q", entries[1].Role)
	}
	if entries[2].Role != "notification" {
		t.Errorf("[2] role: want notification (fallback to type), got %q", entries[2].Role)
	}
}

func TestTailTranscript_AllN(t *testing.T) {
	// Request more than available — should return all entries.
	entries, err := TailTranscript("testdata/tail_basic.jsonl", 50)
	if err != nil {
		t.Fatalf("TailTranscript: %v", err)
	}
	// tail_basic.jsonl has 5 entries.
	if len(entries) != 5 {
		t.Errorf("want 5 entries, got %d", len(entries))
	}
}
