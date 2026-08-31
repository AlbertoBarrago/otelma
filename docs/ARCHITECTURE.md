# Architecture

otelma is a local LLM inference runtime split into four layers, mirroring
Ollama's shape but kept small and didactic. Each layer only talks to the
one below it through a narrow interface, so a layer can be replaced (a new
inference backend, a different storage strategy) without the others
changing.

```
┌─────────────────────────────────────────────────────────┐
│ CLI (internal/cli)                                       │
│  pull · list · ps · run · chat · serve · config           │
│  talks to the API over HTTP even for local invocations    │
└───────────────────────────┬─────────────────────────────┘
                             │ HTTP (JSON)
┌───────────────────────────▼─────────────────────────────┐
│ Local runtime API (internal/api)                          │
│  ┌─────────────────────┐   ┌───────────────────────────┐ │
│  │ Model manager        │   │ Scheduler                  │ │
│  │ (internal/manager)   │◄──┤ (internal/scheduler)       │ │
│  │ registry + state     │   │ serializes dispatch through│ │
│  │ machine + Budget     │   │ the manager                │ │
│  └──────────┬───────────┘   └───────────────────────────┘ │
└─────────────┼─────────────────────────────────────────────┘
              │ backend.InferenceBackend
┌─────────────▼─────────────────────────────────────────────┐
│ Inference backend abstraction (internal/backend)           │
│  llamacpp (real) · echo (test stand-in) · mlx (planned)    │
└─────────────┬─────────────────────────────────────────────┘
              │
┌─────────────▼─────────────────────────────────────────────┐
│ Model storage (internal/storage)                            │
│  checksum/size of local GGUF files, Hugging Face downloader │
└───────────────────────────────────────────────────────────┘
```

## 1. CLI (`internal/cli`)

A thin HTTP client. It never touches the manager, scheduler, or a backend
directly, even when running against a server on the same machine — this
keeps "local" and "remote" otelma indistinguishable from the CLI's point of
view, and means the CLI process can be short-lived while the server
persists loaded models between invocations.

Commands that need the API (`pull`, `ps`, `run`, `chat`) call
`ensureServerRunning` first (`internal/cli/autostart.go`): if
`GET /api/ps` doesn't answer, they spawn `otelma serve` as a detached
background process (its own session via `setsid` on Unix, so it survives
the CLI invocation exiting) and poll for readiness before proceeding. This
is what makes a fresh `otelma pull ...` work with no separate setup step.

## 2. Local runtime API (`internal/api`)

A `net/http` server (Go 1.22+ pattern-based `ServeMux`, no router
dependency) exposing otelma's native endpoints plus an OpenAI-compatible
subset:

- `POST /api/pull` — registers a model (see Model manager below)
- `GET /api/ps` — lists registered models and their state
- `POST /api/run` — runs a conversation turn against a model; accepts
  either `{"prompt": "..."}` (single-shot) or `{"messages": [...]}`
  (multi-turn, used by `otelma chat`)
- `POST /v1/chat/completions` / `GET /v1/models` (`internal/api/openai.go`)
  — a minimal OpenAI chat-completions-compatible surface, so any tool with
  "custom OpenAI endpoint" support can use otelma as its backend. `model`
  in the request is an otelma model name, dispatched through the same
  `Scheduler.Submit` as the native endpoints. No streaming, no real token
  usage accounting — see
  [GUIDE.md](GUIDE.md#openai-compatible-api) for the full contract and
  the note on keeping these docs in sync with `openai.go`.

### Model manager (`internal/manager`)

The core of the memory-safety story. Two pieces:

**Registry** — a thread-safe `map[string]*Model` keyed by the user-chosen
local name. `Pull` computes a real sha256 checksum and file size before
registering, so a `Model`'s `MemoryFootprintBytes` is never a guess.

**State machine** — every `Model` moves through an explicit, exhaustively
enumerated graph (`legalTransitions` in `manager.go`):

```
NOT_PRESENT ──pull──► DOWNLOADED ──load──► LOADING ─┬─► READY ──busy──► BUSY
                            ▲                        │     │              │
                            │                        │     └──unload──►   │
                            └────rollback (failed)────┘   UNLOADING       │
                                                             │             │
                                                             └─────────────┘
                                                        (Unloading/Busy → back to Downloaded/Ready)
```

`Manager.Transition(model, newState)` is the *only* way a model's state
changes. It rejects any transition not in the graph with an error rather
than silently clamping or ignoring it — a bug that tries to move a model
from `BUSY` straight to `NOT_PRESENT` fails loudly instead of corrupting
state.

**Budget** — a `Budget` tracks `reservedBytes` against a fixed
`TotalBytes` ceiling (24GB by default, see [CONFIGURATION.md](CONFIGURATION.md)).
Reservation happens at exactly one edge in the state graph
(`Downloaded → Loading`) and is released at exactly two
(`Loading → Downloaded` on a failed load, `Unloading → Downloaded` on a
normal evict). This means the budget ledger can never drift from which
models are actually resident: there's no code path that loads a model
without going through `Reserve`, and no code path that evicts one without
going through `Release`. `Budget.CanLoad` is available for a caller that
wants to check without committing, but the manager itself always calls
`Reserve` (which fails atomically) rather than check-then-reserve, closing
the race a naive implementation would have between two concurrent loads.

### Scheduler (`internal/scheduler`)

v0.1 keeps dispatch intentionally simple: a single mutex serializes every
`Submit` call through the manager. With a 24GB budget and no guarantee two
sizable models fit `READY` at once, unbounded concurrent dispatch would
just surface budget rejections unpredictably instead of queuing them; a
priority/fairness queue is deferred to a later iteration (see the Status
section of the [README](../README.md)).

## 3. Inference backend abstraction (`internal/backend`)

```go
type InferenceBackend interface {
    Load(path string) error
    Unload() error
    Infer(messages []Message) (string, error)
    MemoryFootprintBytes() uint64
}
```

Neither the manager nor the scheduler know which concrete engine they're
driving. Two implementations exist:

- **`llamacpp`** (`internal/backend/llamacpp`) — the real one. `Load`
  spawns `llama-server` (from
  [llama.cpp](https://github.com/ggml-org/llama.cpp)) as a child process
  bound to a free local port and polls `/health` until it's ready; `Infer`
  POSTs the full message history to its OpenAI-compatible
  `/v1/chat/completions` endpoint; `Unload` kills the process. Because a
  real OS process is spawned and killed, the manager's `READY` state
  genuinely corresponds to resident memory, not an assumption.
- **`echo`** (`internal/backend/echo`) — a no-op stand-in that echoes the
  last message back with a fixed prefix. Exists so the full
  pull→load→infer→unload pipeline is testable without llama.cpp installed
  (`otelma serve --backend echo`).

`internal/backend/mlx` is a placeholder for a future native Apple Silicon
backend (MLX instead of llama.cpp); not implemented yet.

## 4. Model storage (`internal/storage`)

Two responsibilities:

- **Local files**: `Checksum` (sha256) and `Size`, used by `Manager.Pull`
  to populate a `Model`'s identity and memory footprint from the real file
  on disk.
- **Hugging Face downloads**: `ResolveHuggingFace` shells out to
  `llama-cli -hf <repo>` (llama.cpp's own downloader — same one
  `llama-server -hf` uses) rather than reimplementing the Hugging Face API
  client and its auth/redirect handling, then locates the resulting
  `.gguf` file under the standard `huggingface_hub` cache layout
  (`~/.cache/huggingface/hub/models--<owner>--<repo>/snapshots/*/*.gguf`).
  A non-empty single-turn prompt (`-p hi -n 1 -st`) is required to make
  `llama-cli` actually exit after the download instead of spinning forever
  reading empty turns from closed stdin — see the comment in
  `internal/storage/huggingface.go` for the full story.

## Configuration (`internal/config`)

Every value mentioned above as "default" (24GB budget, `llamacpp` backend,
timeouts, addresses) lives in one JSON file rather than being scattered as
constants. See [CONFIGURATION.md](CONFIGURATION.md) for the full reference.
