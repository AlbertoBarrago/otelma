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

// HFPrefix marks a pull source as a Hugging Face reference rather than a
// local path, e.g. "hf:bartowski/SmolLM2-135M-Instruct-GGUF:Q4_K_M".
const HFPrefix = "hf:"

// IsHuggingFaceRef reports whether source names a Hugging Face repo rather
// than a local file path.
func IsHuggingFaceRef(source string) bool {
	return strings.HasPrefix(source, HFPrefix)
}

// ResolveHuggingFace downloads (if not already cached) the GGUF file for
// ref = "<user>/<repo>[:quant]" and returns its local path.
//
// It shells out to llama-cli's own `-hf` downloader (same resolver used by
// llama-server) instead of reimplementing the Hugging Face API client and
// its auth/redirect handling: `-n 0` makes llama-cli load the model and
// exit immediately without generating, so this call is download-only in
// effect. The resulting file is then located under the standard
// huggingface_hub cache layout.
func ResolveHuggingFace(ctx context.Context, ref string) (string, error) {
	repo := strings.TrimPrefix(ref, HFPrefix)
	if repo == "" {
		return "", fmt.Errorf("empty huggingface reference")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// A non-empty prompt with -st (single-turn) is required for the process
	// to actually exit: an empty -p combined with -n 0 leaves llama-cli's
	// REPL reading turn after turn from a closed stdin (each read returns
	// EOF immediately), spinning forever instead of exiting after one turn.
	cmd := exec.CommandContext(ctx, "llama-cli", "-hf", repo, "-p", "hi", "-n", "1", "-st", "--simple-io", "--log-disable")
	cmd.Stdin = strings.NewReader("")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("download %q via llama-cli (is it installed? `brew install llama.cpp`): %w\n%s", repo, err, truncate(out, 2000))
	}

	owner, name, quant := splitHFRepo(repo)
	return findCachedGGUF(owner, name, quant)
}

func splitHFRepo(repo string) (owner, name, quant string) {
	if i := strings.LastIndex(repo, ":"); i >= 0 {
		quant = repo[i+1:]
		repo = repo[:i]
	}
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1], quant
	}
	return "", parts[0], quant
}

// findCachedGGUF locates the .gguf file huggingface_hub's downloader placed
// under ~/.cache/huggingface/hub/models--<owner>--<name>/snapshots/*/*.gguf,
// preferring one whose filename matches quant when given.
func findCachedGGUF(owner, name, quant string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	repoDir := filepath.Join(home, ".cache", "huggingface", "hub", fmt.Sprintf("models--%s--%s", owner, name))

	matches, err := filepath.Glob(filepath.Join(repoDir, "snapshots", "*", "*.gguf"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no .gguf file found under %s after download", repoDir)
	}

	if quant != "" {
		for _, m := range matches {
			if strings.Contains(strings.ToLower(filepath.Base(m)), strings.ToLower(quant)) {
				return m, nil
			}
		}
	}
	return matches[0], nil
}

func truncate(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[len(b)-n:]
}
