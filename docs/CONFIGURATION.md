# Configuration

Every otelma default lives in one JSON file (`internal/config/config.go`)
instead of being scattered across constants. This is a reference for each
field; for a quick start see [GUIDE.md](GUIDE.md).

## Locating and managing the file

```sh
otelma config path    # print the file location
otelma config init     # scaffold it with defaults, so it's there to edit
otelma config init --force   # overwrite an existing file with defaults
otelma config show     # print the config otelma is actually using right now
```

Location: `os.UserConfigDir()/otelma/config.json`, which resolves to
`~/Library/Application Support/otelma/config.json` on macOS and
`$XDG_CONFIG_HOME/otelma/config.json` (or `~/.config/otelma/config.json`)
on Linux.

A sibling file, `registry.json` in the same directory, holds the persisted
list of pulled models (see
[ARCHITECTURE.md](ARCHITECTURE.md#model-manager-internalmanager)) — not
user-edited config, but worth knowing it's there if you're inspecting the
directory or scripting a clean reset (delete both files to start fresh).

A missing file is not an error — `otelma config show` (and every command
that loads config internally) falls back to built-in defaults. A partial
file is merged onto the defaults field-by-field, so you only need to write
the fields you actually want to change:

```json
{
  "serve_addr": "localhost:9999"
}
```

is a complete, valid config file that overrides only the port.

## Fields

| Field | Default | Meaning |
|---|---|---|
| `memory_budget_bytes` | `25769803776` (24GB) | The unified memory ceiling `Budget` enforces. A `Loading` transition that would push reserved memory past this is rejected before it happens. See [ARCHITECTURE.md](ARCHITECTURE.md#model-manager-internalmanager). |
| `serve_addr` | `"localhost:11535"` | Address `otelma serve` listens on. Must match `client_base_url` (below) for the CLI's auto-start and requests to reach it. |
| `backend` | `"llamacpp"` | Which `InferenceBackend` `otelma serve` wires in: `"llamacpp"` for real inference, `"echo"` for a no-op stand-in (testing the pipeline without llama.cpp installed). |
| `llamacpp_startup_timeout_seconds` | `30` | How long `Load` waits for a spawned `llama-server` process to report `/health` before giving up. Raise this for large models on slower hardware. |
| `huggingface_download_timeout_minutes` | `30` | Upper bound on a `pull hf:...` call, covering both the download and llama.cpp's own load-to-verify step. Raise this for large models on a slow connection. |
| `client_base_url` | `"http://localhost:11535"` | Address the CLI talks to as an HTTP client — for `pull`/`ps`/`run`/`chat`, and for `ensureServerRunning`'s health check before auto-starting a server. |

## Precedence

1. Built-in defaults (`config.Default()`)
2. Config file, merged field-by-field over the defaults
3. CLI flags on `otelma serve` (`-addr`, `-backend`,
   `-memory-budget-bytes`), which override the config file for that one
   invocation only — they don't rewrite the file

## Common changes

**Run on a different port permanently:**

```sh
otelma config init
# edit serve_addr AND client_base_url to the new port, then:
otelma serve &
```

Changing only `serve_addr` without also updating `client_base_url` means
the CLI will look for a server on the old port, conclude none is running,
and auto-start a second `otelma serve` on the new port — two servers, two
separate in-memory registries, confusing `ps` output.

**Raise the memory budget** (e.g. on a machine with more than 24GB):

```json
{
  "memory_budget_bytes": 68719476736
}
```

(64GB, in bytes — `64 * 1024^3`.)

**Use the echo backend by default** (for CI or testing without
llama.cpp):

```json
{
  "backend": "echo"
}
```
