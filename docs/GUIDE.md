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
than letting the OS kill something under memory pressure. Unload the model
you don't need right now (there is no `unload` CLI command yet in v0.1;
restarting `otelma serve` clears all state) or raise
`memory_budget_bytes` in the config file if your machine genuinely has more
headroom.

## Running the server explicitly

```sh
otelma serve -addr localhost:9999 -backend echo -memory-budget-bytes 8589934592
```

Flags override the config file for that one invocation:

- `-addr` — address to listen on (default from config: `serve_addr`)
- `-backend` — `llamacpp` (real inference) or `echo` (no-op stand-in, for
  testing the pipeline without llama.cpp installed)
- `-memory-budget-bytes` — the unified memory ceiling

If you point the CLI's `client_base_url` at a different `-addr`, update the
config file to match (see [CONFIGURATION.md](CONFIGURATION.md)) — otherwise
`ensureServerRunning` won't find the server you started and will spawn a
second one on the default address.

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
