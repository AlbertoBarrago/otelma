package manager

import (
	"fmt"
	"sync"
)

// Registry resolves model names to their manifest data (path, checksum,
// declared memory footprint) and tracks the current Model set known to the
// system, independent of runtime state.
type Registry struct {
	mu     sync.RWMutex
	models map[string]*Model
}

// NewRegistry returns an empty Registry ready to use.
func NewRegistry() *Registry {
	return &Registry{models: make(map[string]*Model)}
}

// Register adds m to the registry. It fails if a model with the same name
// is already registered, since re-registration would silently discard
// runtime state (e.g. a model mid-Loading).
func (r *Registry) Register(m *Model) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.models[m.Name]; exists {
		return fmt.Errorf("model %q already registered", m.Name)
	}
	r.models[m.Name] = m
	return nil
}

// Get returns the model registered under name, if any.
func (r *Registry) Get(name string) (*Model, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.models[name]
	return m, ok
}

// Unregister removes name from the registry, reporting whether it was
// present. It does not touch the underlying file on disk (typically the
// shared Hugging Face cache) or check the model's state — callers must do
// their own safety checks first (see Manager.Remove).
func (r *Registry) Unregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.models[name]; !exists {
		return false
	}
	delete(r.models, name)
	return true
}

// List returns every registered model.
func (r *Registry) List() []*Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Model, 0, len(r.models))
	for _, m := range r.models {
		out = append(out, m)
	}
	return out
}
