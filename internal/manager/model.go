// Package manager implements the Model manager: registry of models and the
// state machine governing each model's lifecycle from disk to inference-ready.
package manager

// ModelState represents a model's position in its lifecycle. Transitions are
// driven exclusively by the manager; no other package may mutate state directly.
type ModelState int

const (
	// NotPresent means the model is known to the registry (or referenced by
	// name) but has no local files yet.
	NotPresent ModelState = iota
	// Downloaded means model files are on disk and checksum-verified, but
	// not loaded into memory.
	Downloaded
	// Loading means the manager has committed memory budget and asked the
	// backend to load the weights.
	Loading
	// Ready means the model is loaded and idle, available to be scheduled.
	Ready
	// Busy means the model is currently serving an inference request.
	Busy
	// Unloading means the manager is releasing the model's memory budget.
	Unloading
)

func (s ModelState) String() string {
	switch s {
	case NotPresent:
		return "NOT_PRESENT"
	case Downloaded:
		return "DOWNLOADED"
	case Loading:
		return "LOADING"
	case Ready:
		return "READY"
	case Busy:
		return "BUSY"
	case Unloading:
		return "UNLOADING"
	default:
		return "UNKNOWN"
	}
}

// Model is the registry's view of a single model: identity, on-disk
// location, and the runtime state needed to make scheduling and memory
// decisions.
type Model struct {
	Name     string
	Path     string
	Checksum string
	State    ModelState
	// MemoryFootprintBytes is the estimated resident memory required to hold
	// this model in the READY state. Populated from the manifest once known;
	// the estimation logic itself lives in budget.go.
	MemoryFootprintBytes uint64
}
