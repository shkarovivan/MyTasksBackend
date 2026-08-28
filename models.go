package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

const maxBodyBytes = 1 << 20 // 1 MB

// App requests.

type TaskRequest struct {
	Text  string `json:"text"`
	Model string `json:"model"`
}

type SearchRequest struct {
	Request string          `json:"request"`
	Tasks   json.RawMessage `json:"tasks"`
	Model   string          `json:"model"`
}

// Responses to the app — the same shapes the app already parses in direct mode.

type TaskResponse struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Date        string `json:"date"`
}

type SearchResponse struct {
	Answer string   `json:"answer"`
	IDs    []string `json:"ids"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// OpenAI-compatible chat/completions format.

type llmChatRequest struct {
	Model    string       `json:"model"`
	Messages []llmMessage `json:"messages"`
}

type llmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type llmChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// parseTaskResponse normalizes the LLM reply into a TaskResponse:
// strips markdown fences, extracts the JSON object, uppercases the type.
func parseTaskResponse(content string) (TaskResponse, error) {
	var task TaskResponse
	raw := extractJSONObject(stripFences(content))
	if err := json.Unmarshal([]byte(raw), &task); err != nil {
		return TaskResponse{}, fmt.Errorf("unmarshal task: %w", err)
	}
	task.Type = strings.ToUpper(strings.TrimSpace(task.Type))
	return task, nil
}

// parseSearchResponse normalizes the LLM reply into a SearchResponse.
// LLMs sometimes emit numeric ids — coerce string/number -> string.
func parseSearchResponse(content string) (SearchResponse, error) {
	var raw struct {
		Answer string `json:"answer"`
		IDs    []any  `json:"ids"`
	}
	extracted := extractJSONObject(stripFences(content))
	if err := json.Unmarshal([]byte(extracted), &raw); err != nil {
		return SearchResponse{}, fmt.Errorf("unmarshal search: %w", err)
	}
	ids := make([]string, 0, len(raw.IDs))
	for _, id := range raw.IDs {
		switch v := id.(type) {
		case string:
			ids = append(ids, v)
		case float64:
			ids = append(ids, fmt.Sprintf("%.0f", v))
		default:
			ids = append(ids, fmt.Sprintf("%v", v))
		}
	}
	return SearchResponse{Answer: raw.Answer, IDs: ids}, nil
}
