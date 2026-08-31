package manager

import (
	"fmt"
	"sync"
)

// Budget tracks unified memory usage against a fixed hardware ceiling (e.g.
// 24GB on the target Apple Silicon machine) and decides whether a model may
// transition into READY. It is the single source of truth for "is there
// room to load this model" — the manager must never load a model without
// consulting it first.
type Budget struct {
	mu            sync.Mutex
	TotalBytes    uint64
	reservedBytes uint64
}

// NewBudget returns a Budget with the given total ceiling and nothing
// reserved yet.
func NewBudget(totalBytes uint64) *Budget {
	return &Budget{TotalBytes: totalBytes}
}

// CanLoad reports whether m's declared footprint fits within the remaining
// budget, given whatever is already reserved. It does not reserve anything;
// callers must call Reserve to actually commit the memory.
func (b *Budget) CanLoad(m *Model) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reservedBytes+m.MemoryFootprintBytes <= b.TotalBytes
}

// AvailableBytes returns how much budget remains unreserved.
func (b *Budget) AvailableBytes() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.TotalBytes - b.reservedBytes
}

// Reserve commits bytes against the budget. It fails if doing so would
// exceed TotalBytes, so reservation is the only path by which memory
// pressure can increase — no implicit reservation happens elsewhere.
func (b *Budget) Reserve(bytes uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.reservedBytes+bytes > b.TotalBytes {
		return fmt.Errorf("cannot reserve %d bytes: only %d available of %d total",
			bytes, b.TotalBytes-b.reservedBytes, b.TotalBytes)
	}
	b.reservedBytes += bytes
	return nil
}

// Release gives bytes back to the budget. Releasing more than is currently
// reserved indicates a bookkeeping bug in the caller and panics rather than
// silently underflowing the reservation.
func (b *Budget) Release(bytes uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if bytes > b.reservedBytes {
		panic(fmt.Sprintf("release of %d bytes exceeds %d reserved", bytes, b.reservedBytes))
	}
	b.reservedBytes -= bytes
}
