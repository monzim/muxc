package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/monzim/muxc/internal/config"
)

// ---- IsJSON tests ----------------------------------------------------------

// makeCmd builds a cobra Command with a --json persistent flag, mimicking root.go.
func makeCmd() *cobra.Command {
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().Bool("json", false, "json output")

	sub := &cobra.Command{Use: "sub"}
	root.AddCommand(sub)
	return sub
}

func TestIsJSON_FlagNotSet(t *testing.T) {
	sub := makeCmd()
	if IsJSON(sub) {
		t.Error("IsJSON: expected false when flag not set")
	}
}

func TestIsJSON_FlagSet(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().Bool("json", false, "json output")
	sub := &cobra.Command{Use: "sub"}
	root.AddCommand(sub)

	// Set the flag on the parent (as cobra does when parsing).
	if err := root.PersistentFlags().Set("json", "true"); err != nil {
		t.Fatalf("Set json flag: %v", err)
	}

	if !IsJSON(sub) {
		t.Error("IsJSON: expected true when parent --json flag is set")
	}
}

func TestIsJSON_FlagOnSelf(t *testing.T) {
	cmd := &cobra.Command{Use: "cmd"}
	cmd.Flags().Bool("json", false, "json output")
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatalf("Set json flag: %v", err)
	}
	if !IsJSON(cmd) {
		t.Error("IsJSON: expected true when own --json flag is set")
	}
}

// ---- PathExpand tests ------------------------------------------------------

func TestPathExpand_NoTilde(t *testing.T) {
	p := "/home/user/projects/foo"
	got := PathExpand(p)
	if got != p {
		t.Errorf("PathExpand no tilde: got %q, want %q", got, p)
	}
}

func TestPathExpand_Tilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	p := "~/projects/foo"
	got := PathExpand(p)
	want := filepath.Join(home, "projects/foo")
	if got != want {
		t.Errorf("PathExpand tilde: got %q, want %q", got, want)
	}
}

func TestPathExpand_TildeOnly(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	got := PathExpand("~")
	if got != home {
		t.Errorf("PathExpand tilde-only: got %q, want %q", got, home)
	}
}

func TestPathExpand_Empty(t *testing.T) {
	got := PathExpand("")
	if got != "" {
		t.Errorf("PathExpand empty: got %q", got)
	}
}

// ---- ResolveSessionName tests -----------------------------------------------

// fakeChecker is a test double for sessionChecker.
type fakeChecker struct {
	sessions map[string]bool
}

func (f fakeChecker) HasSession(_ context.Context, name string) (bool, error) {
	return f.sessions[name], nil
}

// errorChecker always returns an error.
type errorChecker struct{}

func (errorChecker) HasSession(_ context.Context, name string) (bool, error) {
	return false, errors.New("tmux error")
}

func defaultCfg() *config.Config {
	return config.DefaultConfig()
}

func TestResolveSessionName_BareExists(t *testing.T) {
	checker := fakeChecker{sessions: map[string]bool{"muxc-myproj": true}}
	cfg := defaultCfg()

	got, err := resolveSessionNameWith(context.Background(), cfg, "muxc-myproj", checker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "muxc-myproj" {
		t.Errorf("got %q, want %q", got, "muxc-myproj")
	}
}

func TestResolveSessionName_PrefixFallback(t *testing.T) {
	// Only the prefixed version exists.
	checker := fakeChecker{sessions: map[string]bool{"muxc-myproj": true}}
	cfg := defaultCfg() // prefix = "muxc-"

	got, err := resolveSessionNameWith(context.Background(), cfg, "myproj", checker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "muxc-myproj" {
		t.Errorf("got %q, want %q", got, "muxc-myproj")
	}
}

func TestResolveSessionName_NotFound(t *testing.T) {
	checker := fakeChecker{sessions: map[string]bool{}}
	cfg := defaultCfg()

	_, err := resolveSessionNameWith(context.Background(), cfg, "missing", checker)
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestResolveSessionName_CheckerError(t *testing.T) {
	cfg := defaultCfg()
	_, err := resolveSessionNameWith(context.Background(), cfg, "anything", errorChecker{})
	if err == nil {
		t.Fatal("expected error when checker fails")
	}
}

func TestResolveSessionName_BothExist_BareWins(t *testing.T) {
	// When the exact name also happens to be a live session, the bare name wins.
	checker := fakeChecker{sessions: map[string]bool{
		"myproj":      true,
		"muxc-myproj": true,
	}}
	cfg := defaultCfg()

	got, err := resolveSessionNameWith(context.Background(), cfg, "myproj", checker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Bare name is tried first, so it returns "myproj" not "muxc-myproj".
	if got != "myproj" {
		t.Errorf("got %q, want bare name %q", got, "myproj")
	}
}

// ---- ConfigDir tests -------------------------------------------------------

func TestConfigDir_EnvVar(t *testing.T) {
	t.Setenv("MUXC_CONFIG_DIR", "/tmp/muxctest")

	cmd := &cobra.Command{Use: "cmd"}
	cmd.PersistentFlags().String("config", "", "config dir")

	got := ConfigDir(cmd)
	if got != "/tmp/muxctest" {
		t.Errorf("ConfigDir env: got %q, want /tmp/muxctest", got)
	}
}

func TestConfigDir_FlagTakesPrecedence(t *testing.T) {
	t.Setenv("MUXC_CONFIG_DIR", "/tmp/fromenv")

	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("config", "", "config dir")
	if err := root.PersistentFlags().Set("config", "/tmp/fromflag"); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	sub := &cobra.Command{Use: "sub"}
	root.AddCommand(sub)

	got := ConfigDir(sub)
	if got != "/tmp/fromflag" {
		t.Errorf("ConfigDir flag: got %q, want /tmp/fromflag", got)
	}
}

func TestConfigDir_Empty(t *testing.T) {
	// No flag, no env var → empty string (config.Load will use default).
	t.Setenv("MUXC_CONFIG_DIR", "")

	cmd := &cobra.Command{Use: "cmd"}
	cmd.PersistentFlags().String("config", "", "config dir")

	got := ConfigDir(cmd)
	if got != "" {
		t.Errorf("ConfigDir empty: got %q, want empty string", got)
	}
}
