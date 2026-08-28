package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// upstreamError is a non-2xx provider response; its body is passed through
// to the client as is.
type upstreamError struct {
	status int
	body   string
}

func (e *upstreamError) Error() string {
	return fmt.Sprintf("upstream status %d: %s", e.status, e.body)
}

type server struct {
	cfg    config
	client *http.Client
}

func newServer(cfg config) *server {
	return &server{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.llmTimeout},
	}
}

func (s *server) model(requested string) string {
	if requested == "" {
		return s.cfg.defaultModel
	}
	return requested
}

// chatCompletion sends a single user prompt to the OpenAI-compatible
// chat/completions endpoint and returns choices[0].message.content.
func (s *server) chatCompletion(ctx context.Context, model, prompt string) (string, error) {
	body, err := json.Marshal(llmChatRequest{
		Model:    model,
		Messages: []llmMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.llmBaseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.providerAPIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", fmt.Errorf("read llm response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", &upstreamError{status: resp.StatusCode, body: string(raw)}
	}

	var parsed llmChatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("invalid llm response json: %w", err)
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("empty llm response")
	}
	return parsed.Choices[0].Message.Content, nil
}
