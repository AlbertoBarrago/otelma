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
  `brew install llama.cpp` (installed automatically if you use the Homebrew
  tap below)

## Install

### Homebrew (macOS)

```sh
brew tap albertobarrago/otelma
brew install otelma
```

This builds `otelma` from the [v0.1.0 release
source](https://github.com/AlbertoBarrago/otelma/releases) and pulls in
`llama.cpp` automatically as a dependency. Formula source:
[homebrew-otelma](https://github.com/AlbertoBarrago/homebrew-otelma).

### From source

```sh
git clone https://github.com/AlbertoBarrago/otelma.git
cd otelma
go build -o otelma ./cmd/otelma
```

## Usage

```sh
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

## Configuration

Every hardcoded default lives in a single JSON file. Find it, create it,
and inspect it with:

```sh
otelma config path    # print the file location
otelma config init     # scaffold it with defaults, so it's there to edit
otelma config show     # print the config otelma is actually using
```

The file lives at `~/Library/Application Support/otelma/config.json` on
macOS (`$XDG_CONFIG_HOME/otelma/config.json` on Linux):

```json
{
  "memory_budget_bytes": 25769803776,
  "serve_addr": "localhost:11535",
  "backend": "llamacpp",
  "llamacpp_startup_timeout_seconds": 30,
  "huggingface_download_timeout_minutes": 30,
  "client_base_url": "http://localhost:11535"
}
```

Any subset of fields may be present; missing ones keep their default. Flags
on `otelma serve` (`-addr`, `-backend`, `-memory-budget-bytes`) override the
config file for that single invocation.

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

- Scheduler serializes dispatch with a single mutex; no priority/fairness
  queue yet.
- No persistence: registry and state live only in the `serve` process's
  memory.
- `internal/backend/mlx` (native Apple Silicon backend) is not implemented.
