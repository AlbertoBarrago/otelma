// Package config defines otelma's on-disk configuration: every value that
// used to be a hardcoded constant lives here instead, with a single file
// the user can find (`otelma config path`), scaffold (`otelma config init`)
// and edit directly.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds every otelma setting that can be tuned without recompiling.
// Field names match their JSON keys so the file is self-explanatory when
// opened directly.
type Config struct {
	// MemoryBudgetBytes is the unified memory ceiling the manager's Budget
	// enforces. Default: 24GB, the target Apple Silicon hardware's total.
	MemoryBudgetBytes uint64 `json:"memory_budget_bytes"`
	// ServeAddr is the default address `otelma serve` listens on.
	ServeAddr string `json:"serve_addr"`
	// Backend is the default inference backend: "llamacpp", "mlx", or "echo".
	Backend string `json:"backend"`
	// LlamaCppStartupTimeoutSeconds bounds how long Load waits for
	// llama-server to report healthy before giving up.
	LlamaCppStartupTimeoutSeconds int `json:"llamacpp_startup_timeout_seconds"`
	// HuggingFaceDownloadTimeoutMinutes bounds a `pull hf:...` download.
	HuggingFaceDownloadTimeoutMinutes int `json:"huggingface_download_timeout_minutes"`
	// ClientBaseURL is the address the CLI talks to as an API client.
	ClientBaseURL string `json:"client_base_url"`
}

// Default returns otelma's built-in defaults, used when no config file
// exists and as the base that a partial file is merged onto.
func Default() Config {
	return Config{
		MemoryBudgetBytes:                 24 * 1 << 30,
		ServeAddr:                         "localhost:11535",
		Backend:                           "llamacpp",
		LlamaCppStartupTimeoutSeconds:     30,
		HuggingFaceDownloadTimeoutMinutes: 30,
		ClientBaseURL:                     "http://localhost:11535",
	}
}

// Path returns the config file location: $XDG_CONFIG_HOME/otelma/config.json
// on Linux, ~/Library/Application Support/otelma/config.json on macOS (via
// os.UserConfigDir, so this needs no extra dependency).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(dir, "otelma", "config.json"), nil
}

// RegistryPath returns where `otelma serve` persists the pulled-model
// registry (see manager.SaveRegistry/LoadRegistry): a sibling of the
// config file, same directory.
func RegistryPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(dir, "otelma", "registry.json"), nil
}

// Load reads the config file if present and merges it onto Default();
// missing fields (or a missing file entirely) keep their default value. A
// missing file is not an error.
func Load() (Config, error) {
	cfg := Default()

	path, err := Path()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

// Init writes cfg to the config file, creating parent directories as
// needed. It refuses to overwrite an existing file unless force is true.
func Init(cfg Config, force bool) (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}

	if !force {
		if _, err := os.Stat(path); err == nil {
			return path, fmt.Errorf("config already exists at %s (use --force to overwrite)", path)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write config %s: %w", path, err)
	}
	return path, nil
}
