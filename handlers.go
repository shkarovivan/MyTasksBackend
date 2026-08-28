package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func postOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
			return
		}
		next(w, r)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// decodeJSON reads the body with a size limit and replies 400 on malformed JSON.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid json"})
		return false
	}
	return true
}

// writeUpstreamError: provider errors -> 502 with the provider body, anything else -> 502.
func writeUpstreamError(w http.ResponseWriter, err error) {
	var upErr *upstreamError
	if errors.As(err, &upErr) {
		log.Printf("llm upstream error: %s", upErr)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(upErr.body))
		return
	}
	log.Printf("llm error: %v", err)
	writeJSON(w, http.StatusBadGateway, errorResponse{Error: "llm unavailable: " + err.Error()})
}

// handleTask: {"text","model"} -> add-task prompt -> LLM -> TaskResponse.
func (s *server) handleTask(w http.ResponseWriter, r *http.Request) {
	var req TaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "text is required"})
		return
	}

	prompt := buildAddTaskPrompt(req.Text, time.Now())
	content, err := s.chatCompletion(r.Context(), s.model(req.Model), prompt)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	task, err := parseTaskResponse(content)
	if err != nil {
		log.Printf("task: failed to parse LLM response: %v; raw: %q", err, content)
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to parse LLM response"})
		return
	}
	writeJSON(w, http.StatusOK, task)
}

// handleSearch: {"request","tasks":[...],"model"} -> search prompt -> LLM -> {"answer","ids"}.
// The task list goes into the prompt byte-for-byte as sent by the client.
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Request) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "request is required"})
		return
	}
	if len(req.Tasks) == 0 || string(req.Tasks) == "null" {
		req.Tasks = json.RawMessage("[]")
	}

	prompt := buildSearchPrompt(req.Request, req.Tasks)
	content, err := s.chatCompletion(r.Context(), s.model(req.Model), prompt)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	search, err := parseSearchResponse(content)
	if err != nil {
		log.Printf("search: failed to parse LLM response: %v; raw: %q", err, content)
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to parse LLM response"})
		return
	}
	writeJSON(w, http.StatusOK, search)
}
