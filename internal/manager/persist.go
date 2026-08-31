package manager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// persistedModel is the on-disk shape SaveRegistry/LoadRegistry use. Only
// identity fields are persisted, deliberately not State: a transient state
// (LOADING/READY/BUSY/UNLOADING) never survives a restart honestly, since
// no backend process is actually resident after one. LoadRegistry always
// restores everything as Downloaded.
type persistedModel struct {
	Name                 string `json:"name"`
	Path                 string `json:"path"`
	Checksum             string `json:"checksum"`
	MemoryFootprintBytes uint64 `json:"memory_footprint_bytes"`
}

// SaveRegistry writes reg's models to path as JSON, atomically (write to a
// temp file in the same directory, then rename) so a crash mid-write can't
// leave a corrupt file behind.
func SaveRegistry(reg *Registry, path string) error {
	models := reg.List()
	out := make([]persistedModel, 0, len(models))
	for _, m := range models {
		if m.Checksum == "" {
			continue // never successfully pulled; nothing worth remembering
		}
		out = append(out, persistedModel{
			Name:                 m.Name,
			Path:                 m.Path,
			Checksum:             m.Checksum,
			MemoryFootprintBytes: m.MemoryFootprintBytes,
		})
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write registry: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("finalize registry: %w", err)
	}
	return nil
}

// LoadRegistry reads a registry previously written by SaveRegistry. A
// missing file is not an error (fresh install / first run) and returns an
// empty Registry. Every restored model is set to Downloaded regardless of
// what state it was in when last saved, since after a restart no backend
// process is actually resident. A model whose file has moved or been
// deleted since the last save is dropped rather than restored as a
// dangling reference.
func LoadRegistry(path string) (*Registry, error) {
	reg := NewRegistry()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return reg, nil
	}
	if err != nil {
		return reg, fmt.Errorf("read registry %s: %w", path, err)
	}

	var persisted []persistedModel
	if err := json.Unmarshal(data, &persisted); err != nil {
		return reg, fmt.Errorf("parse registry %s: %w", path, err)
	}

	for _, p := range persisted {
		if _, err := os.Stat(p.Path); err != nil {
			continue
		}
		_ = reg.Register(&Model{
			Name:                 p.Name,
			Path:                 p.Path,
			Checksum:             p.Checksum,
			State:                Downloaded,
			MemoryFootprintBytes: p.MemoryFootprintBytes,
		})
	}
	return reg, nil
}
