# Guide

A practical walkthrough of otelma's commands. For how it's built, see
[ARCHITECTURE.md](ARCHITECTURE.md). For every config field, see
[CONFIGURATION.md](CONFIGURATION.md).

## Install

```sh
brew tap albertobarrago/otelma
brew install otelma
```

or from source (requires Go 1.24+):

```sh
git clone https://github.com/AlbertoBarrago/otelma.git
cd otelma
go build -o otelma ./cmd/otelma
```

Either way, real inference needs [llama.cpp](https://github.com/ggml-org/llama.cpp)
on `PATH` (`brew install llama.cpp`; the Homebrew tap pulls it in
automatically as a dependency).

## The server starts itself

There is no separate "start the server" step. `pull`, `ps`, `run`, and
`chat` each check whether the API is reachable and, if not, spawn
`otelma serve` in the background before proceeding:

```sh
$ otelma pull smol hf:bartowski/SmolLM2-135M-Instruct-GGUF
otelma: no server running, started one in the background (logs: ~/Library/Caches/otelma/serve.log)
downloading hf:bartowski/SmolLM2-135M-Instruct-GGUF from Hugging Face, this can take a while for larger models...
pulled smol (state=DOWNLOADED)
```

Run `otelma serve` yourself first only if you want to control its address,
backend, or memory budget for that session — see
[Running the server explicitly](#running-the-server-explicitly) below.

## Finding a model

```sh
otelma list
```

prints a curated set of small, instruction-tuned models known to fit
comfortably in a 24GB budget, each with an approximate size and a ready-to-
use `hf:` source. This is a static, hand-picked list
(`internal/catalog/catalog.go`), not a live Hugging Face search.

## Pulling a model

Three ways to name a source:

```sh
# by catalog name (see `otelma list`): resolves to its hf: source for you
otelma pull qwen2.5-0.5b

# from Hugging Face directly: downloads via llama.cpp's own resolver
otelma pull smol hf:bartowski/SmolLM2-135M-Instruct-GGUF

# a specific quantization
otelma pull smol hf:bartowski/SmolLM2-135M-Instruct-GGUF:Q8_0

# a local .gguf file you already have
otelma pull local-model /path/to/model.gguf
```

`pull` computes a real sha256 checksum and file size before registering the
model — that size becomes the memory the Budget reserves when the model is
loaded, so it's never a guess. The first `hf:` pull of a given repo can
take a while (real download); repeat pulls of the same repo/quant are
instant (cached under `~/.cache/huggingface`).

**Pulled models are remembered.** The registry persists to disk after
every successful `pull`, so an `otelma serve` restart doesn't lose track
of what you already have — `otelma ps` shows it again immediately, still
`Downloaded`, no re-download. See
[ARCHITECTURE.md](ARCHITECTURE.md#model-manager-internalmanager) for how.

## Checking status

```sh
$ otelma ps
NAME                     STATE        MEMORY
smol                     READY        100.6MiB
```

`STATE` is one of `NOT_PRESENT`, `DOWNLOADED`, `LOADING`, `READY`, `BUSY`,
`UNLOADING` — the manager's state machine, not a CLI-invented label. See
[ARCHITECTURE.md](ARCHITECTURE.md#2-local-runtime-api-internalapi) for what
each one means and how transitions work.

## Removing a model

```sh
otelma rm smol
```

Unregisters the model — `otelma ps` won't show it anymore, and it won't be
restored on the next `serve` restart. If it's currently `READY`, `rm`
unloads it first (kills the `llama-server` process, releases the memory
budget) so nothing is left dangling. A model that's `LOADING`, `BUSY`, or
`UNLOADING` — mid state-transition — is refused; wait for it to settle and
try again.

**`rm` never deletes the underlying `.gguf` file.** It's typically the
shared Hugging Face cache (`~/.cache/huggingface`), which other tools —
or a future `otelma pull` of the same repo — may still want. Re-pulling
after `rm` is instant if the file is still cached; freeing actual disk
space means deleting from the Hugging Face cache directly.

## Running a prompt

Single-shot, no memory of prior turns:

```sh
otelma run smol "What is the capital of Italy?"
```

If the model isn't already loaded, this loads it first (subject to the
memory budget — see below), runs the prompt, and leaves it `READY` so the
next call doesn't pay the load cost again.

## Chatting

```sh
$ otelma chat smol
chatting with smol (Ctrl+D or /exit to quit, /clear to reset context)
> My name is Alberto.
Nice to meet you, Alberto!
> What's my name?
Your name is Alberto.
> /exit
```

`otelma run smol` with **no** prompt argument does the same thing — `chat`
is just the explicit spelling. Unlike `run`, chat keeps the full transcript
in memory and resends it on every turn, so the model genuinely has access
to prior context (verified against the real `llamacpp` backend: it can
recall a fact stated two turns earlier). `/clear` resets the transcript
without leaving the session; `/exit` or `/quit` (or Ctrl+D) ends it.

## When the memory budget says no

With a 24GB ceiling, loading a second sizable model while a large one is
already `READY` can fail:

```sh
$ otelma run big-model "..."
otelma: load "big-model": cannot load model "big-model": cannot reserve 17179869184 bytes: only 8589934592 available of 25769803776 total
```

This is the Budget doing its job — rejecting the load explicitly rather
than letting the OS kill something under memory pressure. Restart
`otelma serve` to free every loaded model's budget reservation at once
while keeping everything registered as `Downloaded` — see
[Pulled models are remembered](#pulling-a-model) — that's the way to
reclaim memory without losing track of what you've pulled. (
[`otelma rm`](#removing-a-model) also unloads a `READY` model, but it
unregisters it too — you'd need to `pull` it again, instant if still
cached, to use it later.) Or raise `memory_budget_bytes` in the config
file if your machine genuinely has more
headroom.

## Running the server explicitly

```sh
otelma serve -addr localhost:9999 -backend echo -memory-budget-bytes 8589934592
```

Flags override the config file for that one invocation:

- `-addr` — address to listen on (default from config: `serve_addr`)
- `-backend` — `llamacpp` (GGUF, real inference), `mlx` (MLX, real
  inference, Apple Silicon only), or `echo` (no-op stand-in, for testing
  the pipeline without either installed)
- `-memory-budget-bytes` — the unified memory ceiling

If you point the CLI's `client_base_url` at a different `-addr`, update the
config file to match (see [CONFIGURATION.md](CONFIGURATION.md)) — otherwise
`ensureServerRunning` won't find the server you started and will spawn a
second one on the default address.

## Using the MLX backend

```sh
pip install mlx-lm
otelma serve -backend mlx &

otelma pull qwen-mlx mlx:mlx-community/Qwen2.5-0.5B-Instruct-4bit
otelma run qwen-mlx "What is the capital of France?"
```

`mlx:<user>/<repo>` needs an MLX-format repo, not a GGUF one — look for
`mlx-community/...` on Hugging Face (search "MLX" there), not `bartowski/...`
GGUF repos. Everything else works identically to the `llamacpp` path:
`pull` downloads (via `mlx_lm.generate`'s own resolver, so the same
`~/.cache/huggingface` cache and auth apply), `run`/`chat`/`rm` behave the
same, and the registry persists it the same way. The one structural
difference: an MLX model is a *directory* of files (safetensors,
tokenizer, config), not a single `.gguf`, so `otelma ps`'s `MEMORY` column
reflects the whole directory's size (see
[ARCHITECTURE.md](ARCHITECTURE.md#4-model-storage-internalstorage)).

**One `otelma serve` process runs one backend for every model it has
loaded.** If you `pull` an `hf:` (GGUF) model while running
`-backend mlx`, or an `mlx:` model while running the default
`-backend llamacpp`, the pull itself succeeds (registration doesn't care
which backend is active) but loading it — `run`/`chat` — fails, because
the active backend doesn't understand that model's path shape. There's no
per-model backend selection yet; pick one backend per `serve` session
based on which kind of model you're using.

## OpenAI-compatible API

`otelma serve` exposes a minimal subset of the OpenAI chat completions API
(`internal/api/openai.go`) so tools that support a custom OpenAI-compatible
endpoint — editors, IDE assistants, scripts using the OpenAI SDK — can use
otelma as their backend instead of a cloud API.

### `POST /v1/chat/completions`

```sh
curl http://localhost:11535/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "smol",
    "messages": [{"role": "user", "content": "What is the capital of Italy?"}]
  }'
```

```json
{
  "id": "chatcmpl-1788187622075270000",
  "object": "chat.completion",
  "created": 1788187622,
  "model": "smol",
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": "The capital of Italy is Rome."},
    "finish_reason": "stop"
  }],
  "usage": {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
}
```

`model` is an otelma model name (from `otelma pull`), not a Hugging Face
repo — pull it first. The request goes through the same `Scheduler.Submit`
as `otelma run`/`otelma chat`, so a model that's only `DOWNLOADED` gets
loaded first, inside the memory budget.

### `GET /v1/models`

```sh
curl http://localhost:11535/v1/models
```

Lists every registered model (any state, not just `READY`) in the OpenAI
model-list shape, for tools that populate a model picker from this
endpoint.

### What's NOT implemented

- **Streaming** — `{"stream": true}` gets a `400` with an OpenAI-shaped
  error body, not a silently-ignored flag or a broken stream. If a tool
  hard-requires streaming, it won't work against otelma yet.
- **Token usage accounting** — `usage` in the response is always zeroed;
  otelma doesn't tokenize on this path today.
- **Cold-start latency** — the first request against a model that isn't
  `READY` yet pays the full `llama-server` load time (see
  `llamacpp_startup_timeout_seconds` in [CONFIGURATION.md](CONFIGURATION.md)).
  A tool with a short client-side request timeout may time out on that
  first call; `otelma run <name> "warmup"` once beforehand avoids it.

> **Keeping this in sync**: the request/response shapes here mirror
> `internal/api/openai.go` exactly. If you change that file's types or
> behavior (new fields, streaming support, real usage stats), update this
> section, the README's OpenAI-compatible API section, and
> `docs/index.html` in the same change — don't let the docs drift from
> what the endpoint actually does.

## Troubleshooting

**"connection refused" even though I ran a command that should auto-start
the server”** — check `~/Library/Caches/otelma/serve.log` (path printed on
the auto-start message) for why `otelma serve` itself failed to come up;
usually a port already in use by something else, or `llama-server` not on
`PATH`.

**A `hf:` pull hangs** — `ResolveHuggingFace` shells out to `llama-cli`,
which needs `llama-server`/`llama-cli` from llama.cpp installed
(`brew install llama.cpp`). A very large model on a slow connection can
legitimately take a while; the default timeout is 30 minutes
(`huggingface_download_timeout_minutes` in config).

**`otelma run`/`chat` fails with "is otelma serve running?"** — if you're
running `otelma serve` yourself with a non-default `-addr`, make sure
`client_base_url` in the config file matches it.
