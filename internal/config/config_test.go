package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempConfigDir redirects os.UserConfigDir into a temp dir for the
// duration of the test, on both Linux (XDG_CONFIG_HOME) and macOS (HOME).
func withTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
}

func TestLoad_MissingFileReturnsDefaults(t *testing.T) {
	withTempConfigDir(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg != Default() {
		t.Fatalf("expected defaults when no file exists, got %+v", cfg)
	}
}

func TestInitThenLoad_RoundTrips(t *testing.T) {
	withTempConfigDir(t)

	want := Default()
	want.MemoryBudgetBytes = 8 << 30
	want.ServeAddr = "localhost:9999"

	path, err := Init(want, false)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file at %s: %v", path, err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestInit_RefusesOverwriteWithoutForce(t *testing.T) {
	withTempConfigDir(t)

	if _, err := Init(Default(), false); err != nil {
		t.Fatalf("first Init failed: %v", err)
	}
	if _, err := Init(Default(), false); err == nil {
		t.Fatal("expected error overwriting existing config without --force")
	}
	if _, err := Init(Default(), true); err != nil {
		t.Fatalf("Init with force=true should succeed: %v", err)
	}
}

func TestLoad_PartialFileMergesOntoDefaults(t *testing.T) {
	withTempConfigDir(t)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"serve_addr":"localhost:1234"}`), 0o644); err != nil {
		t.Fatalf("write partial config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.ServeAddr != "localhost:1234" {
		t.Fatalf("expected overridden ServeAddr, got %q", cfg.ServeAddr)
	}
	if cfg.MemoryBudgetBytes != Default().MemoryBudgetBytes {
		t.Fatalf("expected default MemoryBudgetBytes preserved, got %d", cfg.MemoryBudgetBytes)
	}
}
