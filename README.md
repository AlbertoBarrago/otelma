<div align="center">
  <img src="assets/icon.svg" width="96" height="96" alt="otelma icon">

  # otelma

  A local LLM inference runtime, built from scratch. Smaller and more didactic
  than Ollama, targeting Apple Silicon (M-series) with a fixed unified memory
  budget as an explicit constraint, not an assumption.
</div>

## Architecture

Four layers:

1. **CLI** (`pull`, `list`, `ps`, `run`, `serve`) — talks to the API over
   HTTP even for local invocations.
2. **Local runtime API** (`internal/api`) — HTTP server exposing the Model
   manager and Scheduler.
   - **Model manager** (`internal/manager`) — registry of models and an
     explicit state machine: `NOT_PRESENT → DOWNLOADED → LOADING → READY →
     BUSY → UNLOADING`. A `Budget` tracks reserved memory against a fixed
     ceiling (default 24GB) and rejects any load that would exceed it.
   - **Scheduler** (`internal/scheduler`) — serializes requests through the
     manager so concurrent callers don't race on the same model's state.
3. **Inference backend abstraction** (`internal/backend`) — a common
   `InferenceBackend` interface behind which concrete engines live:
   - `llamacpp`: spawns `llama-server` (from
     [llama.cpp](https://github.com/ggml-org/llama.cpp)) per loaded model and
     talks to its OpenAI-compatible HTTP API. Requires `llama-server` on
     `PATH`.
   - `echo`: a no-op stand-in that echoes the prompt back, useful for
     exercising the full pipeline without llama.cpp installed.
4. **Model storage** (`internal/storage`) — checksum/size of local GGUF
   files, plus a Hugging Face downloader.

## Requirements

- Go 1.24+
- [llama.cpp](https://github.com/ggml-org/llama.cpp) for real inference:
  `brew install llama.cpp`

## Usage

```sh
go build -o otelma ./cmd/otelma

# start the local runtime API server (background)
./otelma serve &

# browse a curated list of small models known to fit a 24GB budget
./otelma list

# pull a model: a local .gguf path, or a Hugging Face reference
./otelma pull smol hf:bartowski/SmolLM2-135M-Instruct-GGUF
./otelma pull local-model /path/to/model.gguf

# see registered models and their state
./otelma ps

# load (if needed) and run a prompt
./otelma run smol "What is the capital of Italy?"
```

`otelma pull <name> hf:<user>/<repo>[:quant]` downloads via llama.cpp's own
Hugging Face resolver (same one `llama-server -hf` uses), so auth tokens and
caching behave exactly as they do with `llama-cli`/`llama-server` directly.
Quant defaults to `Q4_K_M` if omitted.

## Development

```sh
go build ./...
go vet ./...
go test ./...
gofmt -l .   # should print nothing
```

## Status

v0.1: the full `pull → ps → run` pipeline works end-to-end with real
inference via `llamacpp`. Known limitations:

- Memory budget (24GB) is hardcoded, not yet configurable via flag.
- Scheduler serializes dispatch with a single mutex; no priority/fairness
  queue yet.
- No persistence: registry and state live only in the `serve` process's
  memory.
- `internal/backend/mlx` (native Apple Silicon backend) is not implemented.
