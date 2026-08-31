// Package mlx implements backend.InferenceBackend by spawning
// mlx_lm.server (from the mlx-lm Python package,
// https://github.com/ml-explore/mlx-lm) as a child process per loaded
// model and talking to its OpenAI-compatible HTTP API — the same pattern
// internal/backend/llamacpp uses for llama-server. Requires mlx_lm.server
// on PATH (e.g. `pip install mlx-lm`); native to Apple Silicon, so this
// backend won't run on non-Apple-Silicon hardware.
package mlx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"

	"github.com/albz/otelma/internal/backend"
)

// Backend runs one mlx_lm.server subprocess for the model passed to Load,
// so the process's resident memory genuinely tracks the manager's READY
// state rather than being an implicit assumption.
type Backend struct {
	cmd     *exec.Cmd
	addr    string
	httpc   *http.Client
	startup time.Duration
}

// New returns an unloaded mlx backend. startupTimeout bounds how long Load
// waits for mlx_lm.server to report healthy before giving up (loading MLX
// weights can take a while on the first run against a given model).
func New(startupTimeout time.Duration) *Backend {
	return &Backend{httpc: &http.Client{Timeout: 120 * time.Second}, startup: startupTimeout}
}

// Load spawns mlx_lm.server against path (a local model directory or a
// Hugging Face repo id — mlx_lm.server accepts either, matching what
// storage.ResolveMLX resolves a "mlx:" pull source to).
func (b *Backend) Load(path string) error {
	port, err := freePort()
	if err != nil {
		return fmt.Errorf("mlx: find free port: %w", err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	cmd := exec.Command("mlx_lm.server", "--model", path, "--host", "127.0.0.1", "--port", fmt.Sprint(port), "--log-level", "ERROR")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mlx: start mlx_lm.server (is it installed? `pip install mlx-lm`): %w", err)
	}

	b.cmd = cmd
	b.addr = addr

	if err := b.waitHealthy(); err != nil {
		_ = b.Unload()
		return err
	}
	return nil
}

func (b *Backend) waitHealthy() error {
	deadline := time.Now().Add(b.startup)
	url := "http://" + b.addr + "/health"
	for time.Now().Before(deadline) {
		resp, err := b.httpc.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("mlx: server did not become healthy within %s", b.startup)
}

func (b *Backend) Unload() error {
	if b.cmd == nil || b.cmd.Process == nil {
		return nil
	}
	err := b.cmd.Process.Kill()
	_ = b.cmd.Wait()
	b.cmd = nil
	b.addr = ""
	return err
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (b *Backend) Infer(messages []backend.Message) (string, error) {
	if b.addr == "" {
		return "", fmt.Errorf("mlx: not loaded")
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("mlx: no messages")
	}

	chatMessages := make([]chatMessage, len(messages))
	for i, m := range messages {
		chatMessages[i] = chatMessage{Role: m.Role, Content: m.Content}
	}

	reqBody, err := json.Marshal(chatRequest{
		Model:    "default_model",
		Messages: chatMessages,
	})
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), b.httpc.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+b.addr+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpc.Do(req)
	if err != nil {
		return "", fmt.Errorf("mlx: request failed: %w", err)
	}
	defer resp.Body.Close()

	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("mlx: decode response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("mlx: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("mlx: empty response")
	}
	return out.Choices[0].Message.Content, nil
}

func (b *Backend) MemoryFootprintBytes() uint64 {
	return 0 // accounted via Model.MemoryFootprintBytes (directory size) in the manager
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
