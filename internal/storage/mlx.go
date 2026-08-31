package storage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// MLXPrefix marks a pull source as an MLX model reference rather than a
// local path or a GGUF Hugging Face reference, e.g.
// "mlx:mlx-community/Qwen2.5-0.5B-Instruct-4bit".
const MLXPrefix = "mlx:"

// IsMLXRef reports whether source names an MLX model repo.
func IsMLXRef(source string) bool {
	return strings.HasPrefix(source, MLXPrefix)
}

// ResolveMLX downloads (if not already cached) the MLX model directory for
// ref = "<user>/<repo>" and returns the local snapshot directory path.
// Unlike GGUF models (one file), an MLX model is a directory: safetensors
// weights, tokenizer, and config.
//
// It shells out to `mlx_lm.generate` (from the mlx-lm Python package,
// https://github.com/ml-explore/mlx-lm) with a minimal one-token
// generation to force a real download without reimplementing the Hugging
// Face client: mlx_lm resolves and downloads the full repo via
// huggingface_hub before generating, then exits cleanly on its own once
// done (unlike llama-cli, no REPL-hang workaround needed here).
func ResolveMLX(ctx context.Context, ref string, timeout time.Duration) (string, error) {
	repo := strings.TrimPrefix(ref, MLXPrefix)
	if repo == "" {
		return "", fmt.Errorf("empty mlx reference")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "mlx_lm.generate", "--model", repo, "--max-tokens", "1", "--prompt", "hi")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("download %q via mlx_lm.generate (is mlx-lm installed? `pip install mlx-lm`): %w\n%s", repo, err, truncate(out, 2000))
	}

	owner, name, _ := splitHFRepo(repo)
	return findCachedMLXSnapshot(owner, name)
}

// findCachedMLXSnapshot locates the snapshot directory huggingface_hub's
// downloader placed under
// ~/.cache/huggingface/hub/models--<owner>--<name>/snapshots/<rev>/.
func findCachedMLXSnapshot(owner, name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	repoDir := filepath.Join(home, ".cache", "huggingface", "hub", fmt.Sprintf("models--%s--%s", owner, name))

	matches, err := filepath.Glob(filepath.Join(repoDir, "snapshots", "*"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no snapshot directory found under %s after download", repoDir)
	}
	return matches[0], nil
}
