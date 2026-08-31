// Package backend defines the abstraction the scheduler talks to, hiding
// which concrete inference engine (llama.cpp, MLX, ...) is actually running
// a given model.
package backend

// InferenceBackend is implemented once per concrete engine (e.g. llamacpp,
// mlx). The scheduler and manager depend only on this interface, never on a
// concrete backend package.
type InferenceBackend interface {
	// Load brings the model at path into a ready-to-infer state.
	Load(path string) error
	// Unload releases whatever resources Load acquired.
	Unload() error
	// Infer runs a single inference request against the loaded model.
	Infer(prompt string) (string, error)
	// MemoryFootprintBytes reports current resident memory used by this
	// backend instance, for the manager's Budget accounting.
	MemoryFootprintBytes() uint64
}
