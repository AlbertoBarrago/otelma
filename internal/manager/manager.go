package manager

import (
	"fmt"
	"sync"
)

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
}

// NewManager wires a Registry and Budget together.
func NewManager(registry *Registry, budget *Budget) *Manager {
	return &Manager{Registry: registry, Budget: budget}
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
