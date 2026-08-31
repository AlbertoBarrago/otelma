// Package cli implements the otelma command-line interface (pull, run,
// serve, ps), acting purely as an HTTP client of the api package.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL is the address the CLI talks to when serve is running
// with its default settings.
const DefaultBaseURL = "http://localhost:11535"

type client struct {
	baseURL string
	http    *http.Client
}

func newClient(baseURL string) *client {
	return &client{baseURL: baseURL, http: &http.Client{Timeout: 60 * time.Second}}
}

func (c *client) post(path string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.http.Post(c.baseURL+path, "application/json", bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("request to otelma server failed (is `otelma serve` running?): %w", err)
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

func (c *client) get(path string, out any) error {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request to otelma server failed (is `otelma serve` running?): %w", err)
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

func decode(resp *http.Response, out any) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error != "" {
			return fmt.Errorf("server error: %s", apiErr.Error)
		}
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}
