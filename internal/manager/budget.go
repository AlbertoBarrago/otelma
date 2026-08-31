package manager

// Budget tracks unified memory usage against a fixed hardware ceiling (e.g.
// 24GB on the target Apple Silicon machine) and decides whether a model may
// transition into READY.
//
// Implementation deferred to the next iteration: this is the interface the
// manager's state machine will call before allowing NotPresent/Downloaded ->
// Loading transitions.
type Budget struct {
	TotalBytes uint64
}

// CanLoad reports whether m can be loaded into the READY state without
// exceeding the configured memory ceiling, given whatever else is currently
// resident.
func (b *Budget) CanLoad(m *Model) bool {
	panic("not implemented")
}
