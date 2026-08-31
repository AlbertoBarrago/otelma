package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/albz/otelma/internal/backend"
	"github.com/albz/otelma/internal/storage"
)

// defaultHFDownloadTimeout is used when Manager.HFDownloadTimeout is left
// at its zero value.
const defaultHFDownloadTimeout = 30 * time.Minute

// legalTransitions enumerates every allowed ModelState transition. Anything
// not listed here is rejected by Transition, so the state machine is
// exhaustive by construction rather than by convention.
var legalTransitions = map[ModelState][]ModelState{
	NotPresent: {Downloaded},
	Downloaded: {Loading},
	Loading:    {Ready, Downloaded}, // Downloaded = load failed, rollback
	Ready:      {Busy, Unloading},
	Busy:       {Ready},
	Unloading:  {Downloaded},
}

func isLegalTransition(from, to ModelState) bool {
	for _, s := range legalTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Manager orchestrates state transitions for every Model in the Registry,
// consulting Budget before any transition that increases memory pressure.
// Memory is reserved exactly on Downloaded->Loading and released exactly on
// Unloading->Downloaded, so the budget ledger always matches which models
// are actually resident.
type Manager struct {
	mu       sync.Mutex
	Registry *Registry
	Budget   *Budget

	// NewBackend constructs a fresh backend instance for a model being
	// loaded. Injected so the manager stays decoupled from any concrete
	// backend implementation (v0.1 wires in backend/echo).
	NewBackend func() backend.InferenceBackend

	// HFDownloadTimeout bounds a Pull from a Hugging Face reference. Zero
	// means defaultHFDownloadTimeout.
	HFDownloadTimeout time.Duration

	backends map[string]backend.InferenceBackend
}

// NewManager wires a Registry and Budget together. newBackend is called once
// per LoadModel to obtain the backend instance for that model.
func NewManager(registry *Registry, budget *Budget, newBackend func() backend.InferenceBackend) *Manager {
	return &Manager{
		Registry:   registry,
		Budget:     budget,
		NewBackend: newBackend,
		backends:   make(map[string]backend.InferenceBackend),
	}
}

// Pull registers a model from source, which is either a local file path or
// a Hugging Face reference ("hf:<user>/<repo>[:quant]"). Hugging Face
// sources are downloaded first (see storage.ResolveHuggingFace); either way
// the resulting local file's checksum and size are computed and the model
// enters the registry in the Downloaded state. It does not touch the
// memory budget.
func (mgr *Manager) Pull(ctx context.Context, name, source string) (*Model, error) {
	timeout := mgr.HFDownloadTimeout
	if timeout == 0 {
		timeout = defaultHFDownloadTimeout
	}

	path := source
	if storage.IsHuggingFaceRef(source) {
		resolved, err := storage.ResolveHuggingFace(ctx, source, timeout)
		if err != nil {
			return nil, fmt.Errorf("pull %q: %w", name, err)
		}
		path = resolved
	}

	checksum, err := storage.Checksum(path)
	if err != nil {
		return nil, fmt.Errorf("pull %q: %w", name, err)
	}
	size, err := storage.Size(path)
	if err != nil {
		return nil, fmt.Errorf("pull %q: %w", name, err)
	}

	m := &Model{
		Name:                 name,
		Path:                 path,
		Checksum:             checksum,
		State:                NotPresent,
		MemoryFootprintBytes: size,
	}
	if err := mgr.Registry.Register(m); err != nil {
		return nil, fmt.Errorf("pull %q: %w", name, err)
	}
	if err := mgr.Transition(m, Downloaded); err != nil {
		return nil, fmt.Errorf("pull %q: %w", name, err)
	}
	return m, nil
}

// LoadModel moves a Downloaded model through Loading into Ready, reserving
// budget and invoking the backend. On backend failure the model is rolled
// back to Downloaded and its budget reservation released.
func (mgr *Manager) LoadModel(name string) error {
	m, ok := mgr.Registry.Get(name)
	if !ok {
		return fmt.Errorf("load %q: model not registered", name)
	}

	if err := mgr.Transition(m, Loading); err != nil {
		return err
	}

	be := mgr.NewBackend()
	if err := be.Load(m.Path); err != nil {
		_ = mgr.Transition(m, Downloaded) // rollback releases the reservation
		return fmt.Errorf("load %q: backend load failed: %w", name, err)
	}

	mgr.mu.Lock()
	mgr.backends[name] = be
	mgr.mu.Unlock()

	if err := mgr.Transition(m, Ready); err != nil {
		return err
	}
	return nil
}

// UnloadModel moves a Ready model through Unloading back to Downloaded,
// releasing its budget reservation.
func (mgr *Manager) UnloadModel(name string) error {
	m, ok := mgr.Registry.Get(name)
	if !ok {
		return fmt.Errorf("unload %q: model not registered", name)
	}

	if err := mgr.Transition(m, Unloading); err != nil {
		return err
	}

	mgr.mu.Lock()
	be, hasBackend := mgr.backends[name]
	delete(mgr.backends, name)
	mgr.mu.Unlock()

	if hasBackend {
		if err := be.Unload(); err != nil {
			return fmt.Errorf("unload %q: backend unload failed: %w", name, err)
		}
	}

	return mgr.Transition(m, Downloaded)
}

// Infer runs prompt against a Ready model, marking it Busy for the duration.
func (mgr *Manager) Infer(name, prompt string) (string, error) {
	m, ok := mgr.Registry.Get(name)
	if !ok {
		return "", fmt.Errorf("infer %q: model not registered", name)
	}

	mgr.mu.Lock()
	be, hasBackend := mgr.backends[name]
	mgr.mu.Unlock()
	if !hasBackend {
		return "", fmt.Errorf("infer %q: model not loaded", name)
	}

	if err := mgr.Transition(m, Busy); err != nil {
		return "", err
	}
	defer func() { _ = mgr.Transition(m, Ready) }()

	return be.Infer(prompt)
}

// Transition moves m from its current state to `to`, enforcing the legal
// transition graph and the memory budget. It fails without mutating m.State
// if the transition is illegal or (for Loading) if the budget has no room.
func (mgr *Manager) Transition(m *Model, to ModelState) error {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	from := m.State
	if !isLegalTransition(from, to) {
		return fmt.Errorf("illegal transition for model %q: %s -> %s", m.Name, from, to)
	}

	switch {
	case from == Downloaded && to == Loading:
		if err := mgr.Budget.Reserve(m.MemoryFootprintBytes); err != nil {
			return fmt.Errorf("cannot load model %q: %w", m.Name, err)
		}
	case to == Downloaded && (from == Unloading || from == Loading):
		// Unloading->Downloaded is a normal evict; Loading->Downloaded is a
		// failed load being rolled back. Both release what Loading reserved.
		mgr.Budget.Release(m.MemoryFootprintBytes)
	}

	m.State = to
	return nil
}
