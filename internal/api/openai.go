package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/albz/otelma/internal/backend"
)

// This file implements a minimal subset of the OpenAI chat completions API
// (https://platform.openai.com/docs/api-reference/chat) so any tool that
// supports a custom OpenAI-compatible endpoint can point at otelma and use
// whatever local models have been pulled. "model" in the request maps
// directly to an otelma model name (from `otelma pull`); it is dispatched
// through the same Scheduler as `otelma run`/`chat`, so it auto-loads the
// model within the memory budget like every other entry point.
//
// NOT implemented: streaming (stream:true is rejected, not silently
// ignored) and token usage accounting (the "usage" field in the response
// is always zeroed — otelma doesn't currently tokenize on this path).
// Keep this file's request/response shapes in sync with docs/GUIDE.md's
// "OpenAI-compatible API" section and the README/site whenever they change.

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream,omitempty"`
}

type openAIChoice struct {
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIChatResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

func (s *Server) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req openAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Stream {
		writeOpenAIError(w, http.StatusBadRequest, "stream is not supported yet")
		return
	}
	if req.Model == "" {
		writeOpenAIError(w, http.StatusBadRequest, "model is required")
		return
	}
	if len(req.Messages) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "messages must not be empty")
		return
	}

	messages := make([]backend.Message, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = backend.Message{Role: m.Role, Content: m.Content}
	}

	out, err := s.sched.Submit(req.Model, messages)
	if err != nil {
		writeOpenAIError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, openAIChatResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []openAIChoice{{
			Index:        0,
			Message:      openAIMessage{Role: "assistant", Content: out},
			FinishReason: "stop",
		}},
	})
}

type openAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type openAIModelList struct {
	Object string        `json:"object"`
	Data   []openAIModel `json:"data"`
}

func (s *Server) handleOpenAIModels(w http.ResponseWriter, r *http.Request) {
	models := s.mgr.Registry.List()
	data := make([]openAIModel, 0, len(models))
	for _, m := range models {
		data = append(data, openAIModel{ID: m.Name, Object: "model", OwnedBy: "otelma"})
	}
	writeJSON(w, http.StatusOK, openAIModelList{Object: "list", Data: data})
}

type openAIErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

type openAIErrorResponse struct {
	Error openAIErrorBody `json:"error"`
}

func writeOpenAIError(w http.ResponseWriter, status int, message string) {
	errType := "invalid_request_error"
	if status >= 500 {
		errType = "server_error"
	}
	writeJSON(w, status, openAIErrorResponse{Error: openAIErrorBody{Message: message, Type: errType}})
}
