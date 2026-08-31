// Package api exposes the local runtime as an HTTP server. The CLI talks to
// it even for local invocations, keeping a single request path.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/albz/otelma/internal/backend"
	"github.com/albz/otelma/internal/manager"
	"github.com/albz/otelma/internal/scheduler"
)

// Server exposes Manager/Scheduler operations over HTTP.
type Server struct {
	mgr   *manager.Manager
	sched *scheduler.Scheduler
	log   *slog.Logger
}

// New builds a Server ready to Serve.
func New(mgr *manager.Manager, sched *scheduler.Scheduler, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{mgr: mgr, sched: sched, log: log}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/pull", s.handlePull)
	mux.HandleFunc("GET /api/ps", s.handlePS)
	mux.HandleFunc("POST /api/run", s.handleRun)
	// OpenAI-compatible subset (see openai.go) so tools that support a
	// custom OpenAI endpoint can use otelma as their backend.
	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAIChatCompletions)
	mux.HandleFunc("GET /v1/models", s.handleOpenAIModels)
	return mux
}

// ListenAndServe starts the HTTP server on addr and blocks until ctx is
// cancelled, then shuts down gracefully.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.routes()}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

type pullRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Server) handlePull(w http.ResponseWriter, r *http.Request) {
	var req pullRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	m, err := s.mgr.Pull(r.Context(), req.Name, req.Path)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, modelView(m))
}

func (s *Server) handlePS(w http.ResponseWriter, r *http.Request) {
	models := s.mgr.Registry.List()
	views := make([]modelResponse, 0, len(models))
	for _, m := range models {
		views = append(views, modelView(m))
	}
	writeJSON(w, http.StatusOK, views)
}

type runMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type runRequest struct {
	Name     string       `json:"name"`
	Prompt   string       `json:"prompt,omitempty"` // single-shot convenience; ignored if Messages is set
	Messages []runMessage `json:"messages,omitempty"`
}

type runResponse struct {
	Output string `json:"output"`
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	messages := req.Messages
	if len(messages) == 0 && req.Prompt != "" {
		messages = []runMessage{{Role: "user", Content: req.Prompt}}
	}
	if len(messages) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("request must set prompt or messages"))
		return
	}

	backendMessages := make([]backend.Message, len(messages))
	for i, m := range messages {
		backendMessages[i] = backend.Message{Role: m.Role, Content: m.Content}
	}

	out, err := s.sched.Submit(req.Name, backendMessages)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, runResponse{Output: out})
}

type modelResponse struct {
	Name                 string `json:"name"`
	State                string `json:"state"`
	MemoryFootprintBytes uint64 `json:"memory_footprint_bytes"`
}

func modelView(m *manager.Model) modelResponse {
	return modelResponse{Name: m.Name, State: m.State.String(), MemoryFootprintBytes: m.MemoryFootprintBytes}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorResponse{Error: err.Error()})
}
