package cli

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ensureServerRunning checks whether the API server at c.baseURL already
// answers, and if not, spawns `otelma serve` as a detached background
// process before returning. Without this, a fresh `brew install` followed
// straight by `otelma run ...` fails with a bare "connection refused" that
// gives no hint a separate `otelma serve` needs to be running first.
func ensureServerRunning(c *client) error {
	if isServerUp(c) {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate otelma binary to auto-start serve: %w", err)
	}

	logPath, err := serveLogPath()
	if err != nil {
		return fmt.Errorf("prepare serve log: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open serve log %s: %w", logPath, err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "serve")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("auto-start `otelma serve`: %w", err)
	}
	fmt.Fprintf(os.Stderr, "otelma: no server running, started one in the background (logs: %s)\n", logPath)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if isServerUp(c) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("started `otelma serve` but it didn't become ready in time; check %s", logPath)
}

func isServerUp(c *client) bool {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/ps", nil)
	if err != nil {
		return false
	}
	resp, err := (&http.Client{Timeout: 1 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

func serveLogPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "otelma")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "serve.log"), nil
}
