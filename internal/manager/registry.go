package manager

// Registry resolves model names to their manifest data (path, checksum,
// declared memory footprint) and tracks the current Model set known to the
// system, independent of runtime state.
type Registry struct{}
