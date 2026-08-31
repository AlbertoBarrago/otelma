package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/albz/otelma/internal/backend"
)

func writeFixture(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("fake weights: "+name), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	return path
}

func TestLoadRegistry_MissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	reg, err := LoadRegistry(filepath.Join(dir, "does-not-exist.json"))
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	if len(reg.List()) != 0 {
		t.Fatalf("expected empty registry, got %d models", len(reg.List()))
	}
}

func TestSaveLoadRegistry_RoundTripsAsDownloaded(t *testing.T) {
	dir := t.TempDir()
	path1 := writeFixture(t, dir, "one.gguf")
	path2 := writeFixture(t, dir, "two.gguf")

	reg := NewRegistry()
	_ = reg.Register(&Model{Name: "one", Path: path1, Checksum: "abc", State: Ready, MemoryFootprintBytes: 10})
	_ = reg.Register(&Model{Name: "two", Path: path2, Checksum: "def", State: Downloaded, MemoryFootprintBytes: 20})

	regPath := filepath.Join(dir, "registry.json")
	if err := SaveRegistry(reg, regPath); err != nil {
		t.Fatalf("SaveRegistry failed: %v", err)
	}

	loaded, err := LoadRegistry(regPath)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}

	models := loaded.List()
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	for _, m := range models {
		if m.State != Downloaded {
			t.Errorf("model %q: expected State=Downloaded after reload regardless of pre-save state, got %s",
				m.Name, m.State)
		}
	}

	one, ok := loaded.Get("one")
	if !ok {
		t.Fatal("expected model 'one' to survive round-trip")
	}
	if one.Checksum != "abc" || one.MemoryFootprintBytes != 10 {
		t.Fatalf("model 'one' fields not preserved: %+v", one)
	}
}

func TestSaveRegistry_SkipsNeverPulledModels(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	_ = reg.Register(&Model{Name: "ghost", State: NotPresent}) // no checksum: never actually pulled

	regPath := filepath.Join(dir, "registry.json")
	if err := SaveRegistry(reg, regPath); err != nil {
		t.Fatalf("SaveRegistry failed: %v", err)
	}

	loaded, err := LoadRegistry(regPath)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	if len(loaded.List()) != 0 {
		t.Fatalf("expected never-pulled model to be skipped, got %d models", len(loaded.List()))
	}
}

func TestLoadRegistry_DropsModelsWithMissingFile(t *testing.T) {
	dir := t.TempDir()
	present := writeFixture(t, dir, "present.gguf")
	missing := filepath.Join(dir, "gone.gguf") // never created

	reg := NewRegistry()
	_ = reg.Register(&Model{Name: "present", Path: present, Checksum: "x", State: Downloaded})
	_ = reg.Register(&Model{Name: "gone", Path: missing, Checksum: "y", State: Downloaded})

	regPath := filepath.Join(dir, "registry.json")
	if err := SaveRegistry(reg, regPath); err != nil {
		t.Fatalf("SaveRegistry failed: %v", err)
	}

	loaded, err := LoadRegistry(regPath)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	if _, ok := loaded.Get("present"); !ok {
		t.Error("expected 'present' to survive reload")
	}
	if _, ok := loaded.Get("gone"); ok {
		t.Error("expected 'gone' to be dropped since its file no longer exists")
	}
}

func TestManager_PullPersistsRegistry(t *testing.T) {
	dir := t.TempDir()
	modelPath := writeFixture(t, dir, "model.gguf")
	regPath := filepath.Join(dir, "registry.json")

	reg := NewRegistry()
	mgr := NewManager(reg, NewBudget(1<<20), func() backend.InferenceBackend { return &stubBackend{} })
	mgr.RegistryPath = regPath

	if _, err := mgr.Pull(context.Background(), "demo", modelPath); err != nil {
		t.Fatalf("Pull failed: %v", err)
	}

	if _, err := os.Stat(regPath); err != nil {
		t.Fatalf("expected registry file to be written after Pull: %v", err)
	}

	loaded, err := LoadRegistry(regPath)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	if _, ok := loaded.Get("demo"); !ok {
		t.Fatal("expected 'demo' to be persisted after Pull")
	}
}
