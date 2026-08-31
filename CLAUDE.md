# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

otelma is a local LLM inference runtime for Apple Silicon, built from scratch as a smaller, more didactic
alternative to Ollama. It treats a fixed unified-memory budget (default 24GB) as an explicit constraint the
scheduler and manager enforce, not an assumption.

## VCS

This repo uses **jj (Jujutsu)**, not plain git (`.jj/` present). Do not assume a git branch-first workflow;
confirm with the user how they want changes tracked (bookmarks, colocated git branch, etc.) before making
commits.

## Commands

```sh
go build -o otelma ./cmd/otelma   # build the CLI binary
go build ./...                    # build everything
go vet ./...
go test ./...                     # run all tests
go test ./internal/manager/...    # run a single package's tests
go test ./internal/manager/ -run TestName -v   # run a single test
gofmt -l .                        # should print nothing (formatting check)
```

Requires Go 1.24+. Real inference requires `llama-server` (from llama.cpp, `brew install llama.cpp`) on
`PATH`; without it, only the `echo` backend works.

Manual end-to-end check:

```sh
./otelma serve &
./otelma list
./otelma pull smol hf:bartowski/SmolLM2-135M-Instruct-GGUF
./otelma ps
./otelma run smol "What is the capital of Italy?"
```

## Architecture

Four layers, each with a hard dependency direction (CLI → API → manager/scheduler → backend/storage):

1. **CLI** (`internal/cli`, entrypoint `cmd/otelma/main.go`) — subcommands `pull`, `list`, `ps`, `run`,
   `serve`. Even local invocations go through the HTTP API (`internal/cli/client.go`), so there is a single
   request path regardless of caller.

2. **Local runtime API** (`internal/api/server.go`) — a `net/http` server exposing `POST /api/pull`,
   `GET /api/ps`, `POST /api/run`. Thin: decodes requests, delegates to `Manager`/`Scheduler`, encodes JSON
   responses. No business logic lives here.

3. **Model manager** (`internal/manager`) — owns a `Registry` of models, each driven through an explicit
   state machine: `NOT_PRESENT → DOWNLOADED → LOADING → READY → BUSY → UNLOADING`. `budget.go` tracks
   reserved memory against a fixed ceiling and rejects any load that would exceed it — this is the core
   invariant of the project, don't bypass it when adding new load paths.

4. **Scheduler** (`internal/scheduler`) — serializes requests through the manager so concurrent callers
   don't race on the same model's state; currently a single mutex, no priority/fairness queue.

5. **Inference backend abstraction** (`internal/backend/backend.go`) — the `InferenceBackend` interface
   (`Load`, `Unload`, `Infer`, `MemoryFootprintBytes`) is the only thing the manager/scheduler depend on.
   Concrete engines:
   - `internal/backend/llamacpp` — spawns `llama-server` per loaded model, talks to its OpenAI-compatible
     HTTP API.
   - `internal/backend/echo` — no-op stand-in that echoes the prompt, for exercising the pipeline without
     llama.cpp installed.
   - `internal/backend/mlx` — native Apple Silicon backend, not yet implemented (doc.go only).

6. **Model storage** (`internal/storage`) — checksum/size tracking for local GGUF files, plus a Hugging
   Face downloader (`huggingface.go`). `otelma pull <name> hf:<user>/<repo>[:quant]` shells out to
   llama.cpp's own HF resolver (same one `llama-server -hf` uses), so auth/caching match `llama-cli`
   behavior exactly. Quant defaults to `Q4_K_M`.

`internal/catalog` holds the curated list of small models known to fit the default budget, surfaced by
`otelma list`.

## Known limitations (v0.1, from README)

- 24GB memory budget is hardcoded, not configurable via flag.
- Scheduler has no priority/fairness queue, just a single mutex.
- No persistence: registry/state live only in the `serve` process's memory.
- `internal/backend/mlx` is not implemented.
