package cli

import (
	"testing"
	"time"
)

// ---- SortRows tests --------------------------------------------------------

func makeRow(name string, rss uint64, idle int64, created time.Time) SessionRow {
	return SessionRow{
		Name:                 name,
		RSSPlusChildrenBytes: rss,
		IdleSeconds:          idle,
		CreatedAt:            created,
	}
}

func TestSortRows_Name(t *testing.T) {
	rows := []SessionRow{
		makeRow("muxc-z", 100, 10, time.Now()),
		makeRow("muxc-a", 200, 20, time.Now()),
		makeRow("muxc-m", 300, 30, time.Now()),
	}

	SortRows(rows, "name")

	want := []string{"muxc-a", "muxc-m", "muxc-z"}
	for i, r := range rows {
		if r.Name != want[i] {
			t.Errorf("sort name [%d]: got %q, want %q", i, r.Name, want[i])
		}
	}
}

func TestSortRows_NameDefault(t *testing.T) {
	// Unknown key should fall back to name sort.
	rows := []SessionRow{
		makeRow("gamma", 0, 0, time.Now()),
		makeRow("alpha", 0, 0, time.Now()),
		makeRow("beta", 0, 0, time.Now()),
	}

	SortRows(rows, "unknown-key")

	want := []string{"alpha", "beta", "gamma"}
	for i, r := range rows {
		if r.Name != want[i] {
			t.Errorf("sort default [%d]: got %q, want %q", i, r.Name, want[i])
		}
	}
}

func TestSortRows_Mem(t *testing.T) {
	rows := []SessionRow{
		makeRow("small", 1024, 0, time.Now()),
		makeRow("large", 1<<30, 0, time.Now()),
		makeRow("medium", 1<<20, 0, time.Now()),
	}

	SortRows(rows, "mem")

	// Descending: largest first.
	want := []string{"large", "medium", "small"}
	for i, r := range rows {
		if r.Name != want[i] {
			t.Errorf("sort mem [%d]: got %q, want %q", i, r.Name, want[i])
		}
	}
}

func TestSortRows_Idle(t *testing.T) {
	rows := []SessionRow{
		makeRow("active", 0, 60, time.Now()),
		makeRow("idle", 0, 7200, time.Now()),
		makeRow("medium", 0, 300, time.Now()),
	}

	SortRows(rows, "idle")

	// Descending: most idle first.
	want := []string{"idle", "medium", "active"}
	for i, r := range rows {
		if r.Name != want[i] {
			t.Errorf("sort idle [%d]: got %q, want %q", i, r.Name, want[i])
		}
	}
}

func TestSortRows_Created(t *testing.T) {
	now := time.Now()
	rows := []SessionRow{
		makeRow("newest", 0, 0, now),
		makeRow("oldest", 0, 0, now.Add(-24*time.Hour)),
		makeRow("middle", 0, 0, now.Add(-12*time.Hour)),
	}

	SortRows(rows, "created")

	// Ascending: oldest first.
	want := []string{"oldest", "middle", "newest"}
	for i, r := range rows {
		if r.Name != want[i] {
			t.Errorf("sort created [%d]: got %q, want %q", i, r.Name, want[i])
		}
	}
}

func TestSortRows_Empty(t *testing.T) {
	// Should not panic on empty slice.
	var rows []SessionRow
	SortRows(rows, "name")
	SortRows(rows, "mem")
	SortRows(rows, "idle")
	SortRows(rows, "created")
}

func TestSortRows_SingleRow(t *testing.T) {
	rows := []SessionRow{makeRow("only", 100, 10, time.Now())}
	SortRows(rows, "mem")
	if rows[0].Name != "only" {
		t.Errorf("single row sort: got %q", rows[0].Name)
	}
}

// ---- FilterByGlob tests ----------------------------------------------------

func makeNamedRows(names ...string) []SessionRow {
	rows := make([]SessionRow, len(names))
	for i, n := range names {
		rows[i] = SessionRow{Name: n}
	}
	return rows
}

func TestFilterByGlob_EmptyPattern(t *testing.T) {
	rows := makeNamedRows("muxc-a", "muxc-b", "other")
	got := FilterByGlob(rows, "")
	if len(got) != 3 {
		t.Errorf("empty pattern: expected 3 rows, got %d", len(got))
	}
}

func TestFilterByGlob_LiteralMatch(t *testing.T) {
	rows := makeNamedRows("muxc-alpha", "muxc-beta", "muxc-gamma")
	got := FilterByGlob(rows, "muxc-alpha")
	if len(got) != 1 {
		t.Fatalf("literal match: expected 1, got %d", len(got))
	}
	if got[0].Name != "muxc-alpha" {
		t.Errorf("literal match: got %q", got[0].Name)
	}
}

func TestFilterByGlob_StarPattern(t *testing.T) {
	rows := makeNamedRows("muxc-alpha", "muxc-beta", "external")
	got := FilterByGlob(rows, "muxc-*")
	if len(got) != 2 {
		t.Fatalf("star pattern: expected 2, got %d", len(got))
	}
	for _, r := range got {
		if r.Name == "external" {
			t.Errorf("star pattern: unexpectedly kept %q", r.Name)
		}
	}
}

func TestFilterByGlob_QuestionMarkPattern(t *testing.T) {
	rows := makeNamedRows("muxc-a", "muxc-bb", "muxc-c")
	got := FilterByGlob(rows, "muxc-?")
	if len(got) != 2 {
		t.Fatalf("? pattern: expected 2 single-char suffix rows, got %d", len(got))
	}
}

func TestFilterByGlob_NoMatch(t *testing.T) {
	rows := makeNamedRows("muxc-alpha", "muxc-beta")
	got := FilterByGlob(rows, "other-*")
	if len(got) != 0 {
		t.Errorf("no match: expected 0, got %d", len(got))
	}
}

func TestFilterByGlob_InvalidPattern(t *testing.T) {
	// An invalid glob pattern should not panic; it just drops all rows.
	rows := makeNamedRows("muxc-alpha")
	// "[z-a]" is an invalid character class range → filepath.Match returns error.
	got := FilterByGlob(rows, "[z-a]")
	// All rows are dropped because the pattern itself errors.
	if len(got) != 0 {
		t.Errorf("invalid pattern: expected 0 rows (error drops), got %d", len(got))
	}
}

func TestFilterByGlob_Empty(t *testing.T) {
	var rows []SessionRow
	got := FilterByGlob(rows, "muxc-*")
	if len(got) != 0 {
		t.Errorf("empty rows: expected 0, got %d", len(got))
	}
}

func TestFilterByGlob_AllMatch(t *testing.T) {
	rows := makeNamedRows("muxc-a", "muxc-b", "muxc-c")
	got := FilterByGlob(rows, "*")
	if len(got) != 3 {
		t.Errorf("wildcard: expected 3, got %d", len(got))
	}
}
