// Package catalog lists Hugging Face GGUF models known to run comfortably
// within the 24GB unified memory budget on the target Apple Silicon
// hardware, as a starting point for `otelma pull`. It is a static,
// hand-curated list, not a live Hugging Face search: entries are picked for
// being small, instruction-tuned, and reliably quantized by well-known
// GGUF publishers.
package catalog

// Entry describes one recommended model.
type Entry struct {
	// Name is a short local alias suggestive of `otelma pull <Name> hf:...`.
	Name string
	// HFRef is the ready-to-use source for otelma pull, e.g.
	// "hf:bartowski/SmolLM2-135M-Instruct-GGUF".
	HFRef string
	// ApproxBytes is the approximate Q4_K_M download size; actual size is
	// known precisely only after pull computes it from the real file.
	ApproxBytes uint64
	Description string
}

const (
	mb = 1 << 20
	gb = 1 << 30
)

// Entries is the curated list, roughly smallest to largest.
var Entries = []Entry{
	{
		Name:        "smollm2-135m",
		HFRef:       "hf:bartowski/SmolLM2-135M-Instruct-GGUF",
		ApproxBytes: 100 * mb,
		Description: "Tiny instruct model, good for smoke-testing the pipeline itself.",
	},
	{
		Name:        "smollm2-360m",
		HFRef:       "hf:bartowski/SmolLM2-360M-Instruct-GGUF",
		ApproxBytes: 250 * mb,
		Description: "Slightly stronger than 135m, still near-instant to load.",
	},
	{
		Name:        "qwen2.5-0.5b",
		HFRef:       "hf:Qwen/Qwen2.5-0.5B-Instruct-GGUF",
		ApproxBytes: 350 * mb,
		Description: "Qwen2.5 instruct, competitive quality for its size.",
	},
	{
		Name:        "qwen2.5-1.5b",
		HFRef:       "hf:Qwen/Qwen2.5-1.5B-Instruct-GGUF",
		ApproxBytes: 1 * gb,
		Description: "Noticeably more capable, still fast on M4.",
	},
	{
		Name:        "llama-3.2-1b",
		HFRef:       "hf:bartowski/Llama-3.2-1B-Instruct-GGUF",
		ApproxBytes: 800 * mb,
		Description: "Meta's small Llama 3.2, broad general knowledge.",
	},
	{
		Name:        "llama-3.2-3b",
		HFRef:       "hf:bartowski/Llama-3.2-3B-Instruct-GGUF",
		ApproxBytes: 2 * gb,
		Description: "Larger Llama 3.2, best quality/size tradeoff in this list.",
	},
	{
		Name:        "gemma-2-2b",
		HFRef:       "hf:bartowski/gemma-2-2b-it-GGUF",
		ApproxBytes: 1600 * mb,
		Description: "Google Gemma 2, strong for its parameter count.",
	},
	{
		Name:        "phi-3.5-mini",
		HFRef:       "hf:bartowski/Phi-3.5-mini-instruct-GGUF",
		ApproxBytes: 2200 * mb,
		Description: "Microsoft Phi-3.5, strong reasoning for a ~3.8B model.",
	},
}
