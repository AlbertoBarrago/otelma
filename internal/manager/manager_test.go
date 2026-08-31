package manager

import "testing"

func newTestManager(totalBytes uint64) (*Manager, *Model) {
	m := &Model{Name: "test-model", State: NotPresent, MemoryFootprintBytes: 4}
	reg := NewRegistry()
	_ = reg.Register(m)
	return NewManager(reg, NewBudget(totalBytes)), m
}

func TestTransition_FullLifecycle(t *testing.T) {
	mgr, m := newTestManager(10)

	steps := []ModelState{Downloaded, Loading, Ready, Busy, Ready, Unloading, Downloaded}
	for _, to := range steps {
		if err := mgr.Transition(m, to); err != nil {
			t.Fatalf("transition to %s failed: %v", to, err)
		}
	}
	if m.State != Downloaded {
		t.Fatalf("expected final state Downloaded, got %s", m.State)
	}
	if got := mgr.Budget.AvailableBytes(); got != 10 {
		t.Fatalf("expected budget fully released, available=%d want=10", got)
	}
}

func TestTransition_IllegalRejected(t *testing.T) {
	mgr, m := newTestManager(10)

	if err := mgr.Transition(m, Ready); err == nil {
		t.Fatal("expected error transitioning NotPresent -> Ready directly")
	}
	if m.State != NotPresent {
		t.Fatalf("state must not change on illegal transition, got %s", m.State)
	}
}

func TestTransition_LoadingRollbackReleasesBudget(t *testing.T) {
	mgr, m := newTestManager(10)
	_ = mgr.Transition(m, Downloaded)
	_ = mgr.Transition(m, Loading)

	if got := mgr.Budget.AvailableBytes(); got != 6 {
		t.Fatalf("expected 6 bytes available after reserve, got %d", got)
	}

	if err := mgr.Transition(m, Downloaded); err != nil {
		t.Fatalf("rollback transition failed: %v", err)
	}
	if got := mgr.Budget.AvailableBytes(); got != 10 {
		t.Fatalf("expected budget released after rollback, got %d", got)
	}
}

func TestTransition_RejectsWhenBudgetExhausted(t *testing.T) {
	mgr, m := newTestManager(3) // model needs 4 bytes, budget only has 3
	_ = mgr.Transition(m, Downloaded)

	err := mgr.Transition(m, Loading)
	if err == nil {
		t.Fatal("expected error loading model larger than remaining budget")
	}
	if m.State != Downloaded {
		t.Fatalf("state must not change when budget rejects load, got %s", m.State)
	}
}

func TestBudget_CanLoadDoesNotReserve(t *testing.T) {
	b := NewBudget(10)
	m := &Model{Name: "m", MemoryFootprintBytes: 6}

	if !b.CanLoad(m) {
		t.Fatal("expected CanLoad true when budget has room")
	}
	if got := b.AvailableBytes(); got != 10 {
		t.Fatalf("CanLoad must not reserve, available=%d want=10", got)
	}
}

func TestBudget_ReleaseMoreThanReservedPanics(t *testing.T) {
	b := NewBudget(10)
	_ = b.Reserve(4)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic releasing more than reserved")
		}
	}()
	b.Release(5)
}

func TestRegistry_DuplicateRegisterFails(t *testing.T) {
	reg := NewRegistry()
	m := &Model{Name: "dup"}
	if err := reg.Register(m); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := reg.Register(m); err == nil {
		t.Fatal("expected error on duplicate registration")
	}
}
