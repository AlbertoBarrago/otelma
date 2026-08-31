package manager

// Manager orchestrates state transitions for every Model in the Registry,
// consulting Budget before any transition that increases memory pressure.
type Manager struct {
	Registry *Registry
	Budget   *Budget
}
