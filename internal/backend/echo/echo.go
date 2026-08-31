// Package echo is a stand-in backend.InferenceBackend used for v0.1 so the
// full pull->load->infer pipeline is testable end-to-end before llamacpp
// (see internal/backend/llamacpp) is implemented. It does not run any real
// inference: Infer echoes the prompt back with a fixed prefix.
package echo

import "fmt"

// Backend is a no-op InferenceBackend: Load/Unload just track a loaded
// flag, Infer echoes the prompt. It reports a fixed, non-zero memory
// footprint so it still exercises the manager's budget accounting.
type Backend struct {
	path   string
	loaded bool
}

// New returns an unloaded echo backend.
func New() *Backend {
	return &Backend{}
}

func (b *Backend) Load(path string) error {
	b.path = path
	b.loaded = true
	return nil
}

func (b *Backend) Unload() error {
	b.loaded = false
	b.path = ""
	return nil
}

func (b *Backend) Infer(prompt string) (string, error) {
	if !b.loaded {
		return "", fmt.Errorf("echo backend: not loaded")
	}
	return fmt.Sprintf("[echo:%s] %s", b.path, prompt), nil
}

func (b *Backend) MemoryFootprintBytes() uint64 {
	if !b.loaded {
		return 0
	}
	return 1 // negligible, real footprint is accounted via Model.MemoryFootprintBytes
}
