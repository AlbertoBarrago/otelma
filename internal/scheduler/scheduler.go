// Package scheduler queues concurrent inference requests and decides when to
// dispatch them to a backend, taking the manager's memory budget into
// account before allowing a model to move toward READY/BUSY.
//
// v0.1 keeps dispatch intentionally simple: a single mutex serializes
// requests through the manager. With a 24GB unified memory budget and no
// guarantee two models fit READY at once, unbounded concurrent dispatch
// would just surface budget rejections at random instead of queuing
// predictably; a priority/fairness queue is deferred to a later iteration.
package scheduler

import (
	"sync"

	"github.com/albz/otelma/internal/backend"
	"github.com/albz/otelma/internal/manager"
)

// Scheduler serializes access to a Manager so concurrent CLI/API callers
// don't race on the same model's state transitions.
type Scheduler struct {
	mu  sync.Mutex
	mgr *manager.Manager
}

// New wraps mgr with serialized dispatch.
func New(mgr *manager.Manager) *Scheduler {
	return &Scheduler{mgr: mgr}
}

// Submit loads name if it isn't already Ready/Busy, runs the conversation
// (messages[len-1] is the newest turn) against it, and returns the result.
// The model is left Ready (not unloaded) so subsequent requests avoid a
// reload.
func (s *Scheduler) Submit(name string, messages []backend.Message) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.mgr.Registry.Get(name)
	if !ok {
		return s.mgr.Infer(name, messages) // surfaces the "not registered" error uniformly
	}

	if m.State == manager.Downloaded {
		if err := s.mgr.LoadModel(name); err != nil {
			return "", err
		}
	}

	return s.mgr.Infer(name, messages)
}
