package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/render"
)

// ---- ComputeMemTotals tests ------------------------------------------------

func TestComputeMemTotals_Empty(t *testing.T) {
	totals := ComputeMemTotals(nil)
	if totals.Count != 0 {
		t.Errorf("empty totals count: got %d", totals.Count)
	}
	if totals.RSSBytes != 0 {
		t.Errorf("empty totals rss: got %d", totals.RSSBytes)
	}
}

func TestComputeMemTotals_ThreeRows(t *testing.T) {
	rows := []SessionRow{
		{RSSPlusChildrenBytes: 100 * 1024 * 1024},
		{RSSPlusChildrenBytes: 200 * 1024 * 1024},
		{RSSPlusChildrenBytes: 50 * 1024 * 1024},
	}
	totals := ComputeMemTotals(rows)
	if totals.Count != 3 {
		t.Errorf("three-row count: got %d, want 3", totals.Count)
	}
	wantRSS := uint64(350 * 1024 * 1024)
	if totals.RSSBytes != wantRSS {
		t.Errorf("three-row rss: got %d, want %d", totals.RSSBytes, wantRSS)
	}
}

func TestComputeMemTotals_ZeroRSS(t *testing.T) {
	rows := []SessionRow{
		{Name: "a", RSSPlusChildrenBytes: 0},
		{Name: "b", RSSPlusChildrenBytes: 0},
	}
	totals := ComputeMemTotals(rows)
	if totals.Count != 2 {
		t.Errorf("zero rss count: got %d", totals.Count)
	}
	if totals.RSSBytes != 0 {
		t.Errorf("zero rss total: got %d", totals.RSSBytes)
	}
}

// ---- RenderMemTable tests --------------------------------------------------

func renderMemOutput(t *testing.T, cfg *config.Config, rows []SessionRow, totals memTotals) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()

	orig := os.Stdout
	os.Stdout = w

	renderErr := RenderMemTable(os.Stdout, cfg, rows, totals)

	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if renderErr != nil {
		t.Fatalf("RenderMemTable: %v", renderErr)
	}
	return buf.String()
}

func TestRenderMemTable_TotalsFooter(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Display.Color = "never"

	rows := []SessionRow{
		{Name: "muxc-a", RSSPlusChildrenBytes: 100 * 1024 * 1024, CreatedAt: time.Now(), ActivityAt: time.Now()},
		{Name: "muxc-b", RSSPlusChildrenBytes: 200 * 1024 * 1024, CreatedAt: time.Now(), ActivityAt: time.Now()},
		{Name: "muxc-c", RSSPlusChildrenBytes: 50 * 1024 * 1024, CreatedAt: time.Now(), ActivityAt: time.Now()},
	}
	totals := ComputeMemTotals(rows)

	out := renderMemOutput(t, cfg, rows, totals)

	if !strings.Contains(out, "TOTAL:") {
		t.Errorf("totals footer missing TOTAL: in output:\n%s", out)
	}
	if !strings.Contains(out, "3 session") {
		t.Errorf("totals count missing in output:\n%s", out)
	}
	if !strings.Contains(out, "RSS (claude+children)") {
		t.Errorf("totals label missing in output:\n%s", out)
	}
}

func TestRenderMemTable_EmptyRows(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Display.Color = "never"

	totals := ComputeMemTotals(nil)
	out := renderMemOutput(t, cfg, nil, totals)

	// Header should still appear.
	if !strings.Contains(out, "NAME") {
		t.Errorf("header missing in empty mem table:\n%s", out)
	}
	// Totals footer should show 0 sessions.
	if !strings.Contains(out, "0 session") {
		t.Errorf("zero session total missing:\n%s", out)
	}
}

func TestRenderMemTable_SingleSession(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Display.Color = "never"

	rows := []SessionRow{
		{Name: "muxc-x", RSSPlusChildrenBytes: 512 * 1024 * 1024, CreatedAt: time.Now(), ActivityAt: time.Now()},
	}
	totals := ComputeMemTotals(rows)
	out := renderMemOutput(t, cfg, rows, totals)

	// Should say "1 session" not "1 sessions".
	if !strings.Contains(out, "1 session") {
		t.Errorf("singular 'session' missing in output:\n%s", out)
	}
	if strings.Contains(out, "1 sessions") {
		t.Errorf("incorrect plural '1 sessions' in output:\n%s", out)
	}
}

// ---- JSON output tests -----------------------------------------------------

func TestMemJSON_Structure(t *testing.T) {
	rows := []SessionRow{
		{Name: "muxc-a", RSSPlusChildrenBytes: 1024},
	}
	totals := ComputeMemTotals(rows)
	out := memOutput{Sessions: rows, Totals: totals}

	var buf bytes.Buffer
	if err := render.JSON(&buf, out); err != nil {
		t.Fatalf("render.JSON: %v", err)
	}
	s := buf.String()

	for _, want := range []string{`"sessions"`, `"totals"`, `"count"`, `"rss_bytes"`} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON structure missing %q:\n%s", want, s)
		}
	}
}

func TestMemJSON_TotalsCorrect(t *testing.T) {
	rows := []SessionRow{
		{Name: "muxc-a", RSSPlusChildrenBytes: 1024},
		{Name: "muxc-b", RSSPlusChildrenBytes: 2048},
	}
	totals := ComputeMemTotals(rows)
	if totals.Count != 2 {
		t.Errorf("totals count: got %d", totals.Count)
	}
	if totals.RSSBytes != 3072 {
		t.Errorf("totals rss: got %d, want 3072", totals.RSSBytes)
	}
}

// ---- sessionPlural tests ---------------------------------------------------

func TestSessionPlural(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "s"},
		{1, ""},
		{2, "s"},
		{100, "s"},
	}
	for _, c := range cases {
		got := sessionPlural(c.n)
		if got != c.want {
			t.Errorf("sessionPlural(%d): got %q, want %q", c.n, got, c.want)
		}
	}
}
