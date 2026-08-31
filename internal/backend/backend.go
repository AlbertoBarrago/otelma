// Package backend defines the abstraction the scheduler talks to, hiding
// which concrete inference engine (llama.cpp, MLX, ...) is actually running
// a given model.
package backend

// Message is one turn in a conversation, in the "role": "user"/"assistant"/
// "system" shape most chat-tuned models and APIs expect.
type Message struct {
	Role    string
	Content string
}

// InferenceBackend is implemented once per concrete engine (e.g. llamacpp,
// mlx). The scheduler and manager depend only on this interface, never on a
// concrete backend package.
type InferenceBackend interface {
	// Load brings the model at path into a ready-to-infer state.
	Load(path string) error
	// Unload releases whatever resources Load acquired.
	Unload() error
	// Infer runs one turn against the loaded model given the conversation
	// so far (messages[len-1] is the newest turn); a single-shot prompt is
	// just a one-message slice.
	Infer(messages []Message) (string, error)
	// MemoryFootprintBytes reports current resident memory used by this
	// backend instance, for the manager's Budget accounting.
	MemoryFootprintBytes() uint64
}
