// Package ai makes real HTTP calls to an OpenAI-compatible or Anthropic
// chat endpoint, injecting the current cluster/resource context into the
// prompt. It has no dependency on internal/k8s or internal/mock — the UI
// supplies whatever Context it has on hand.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Provider presets offered by the /config modal.
type Provider struct {
	Label string
	URL   string
	Model string
}

var Providers = []Provider{
	{"OpenAI-compatible", "https://api.openai.com/v1", "gpt-5"},
	{"Anthropic", "https://api.anthropic.com/v1", "claude-sonnet-5"},
}

// Config is the user's /config settings.
type Config struct {
	Provider string // "openai" | "anthropic"
	BaseURL  string
	Model    string
	APIKey   string
}

// Context is cluster state injected into the system prompt so answers can
// reference what's actually on screen.
type Context struct {
	ClusterContext string
	Namespace      string
	ResourceKind   string
	SelectedName   string
}

func (c Context) systemPrompt() string {
	var b strings.Builder
	b.WriteString("You are k10s, an AI assistant embedded in a terminal Kubernetes dashboard. ")
	b.WriteString("Answer concisely, in plain text (no markdown tables), suitable for a fixed-width terminal pane. ")
	b.WriteString("Prefer concrete kubectl commands when suggesting next steps.\n\nCurrent view:\n")
	fmt.Fprintf(&b, "- kube context: %s\n", c.ClusterContext)
	fmt.Fprintf(&b, "- namespace: %s\n", c.Namespace)
	fmt.Fprintf(&b, "- resource kind: %s\n", c.ResourceKind)
	if c.SelectedName != "" {
		fmt.Fprintf(&b, "- selected: %s\n", c.SelectedName)
	}
	return b.String()
}

// Ask sends prompt to the configured model and returns its reply.
func Ask(ctx context.Context, cfg Config, cc Context, prompt string) (string, error) {
	if cfg.APIKey == "" {
		return "", fmt.Errorf("no API key set — /settings to add one")
	}
	if cfg.Model == "" {
		return "", fmt.Errorf("no model set — /settings to add one")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	switch cfg.Provider {
	case "anthropic":
		return askAnthropic(ctx, cfg, cc, prompt)
	default:
		return askOpenAI(ctx, cfg, cc, prompt)
	}
}

func askOpenAI(ctx context.Context, cfg Config, cc Context, prompt string) (string, error) {
	body := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": cc.systemPrompt()},
			{"role": "user", "content": prompt},
		},
	}
	b, _ := json.Marshal(body)

	url := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", apiError(resp.StatusCode, data)
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return out.Choices[0].Message.Content, nil
}

func askAnthropic(ctx context.Context, cfg Config, cc Context, prompt string) (string, error) {
	body := map[string]any{
		"model":      cfg.Model,
		"max_tokens": 1024,
		"system":     cc.systemPrompt(),
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	b, _ := json.Marshal(body)

	url := strings.TrimRight(cfg.BaseURL, "/") + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", apiError(resp.StatusCode, data)
	}

	var out struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(out.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}
	var b2 strings.Builder
	for _, c := range out.Content {
		b2.WriteString(c.Text)
	}
	return b2.String(), nil
}

func apiError(status int, body []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return fmt.Errorf("http %d: %s", status, e.Error.Message)
	}
	msg := strings.TrimSpace(string(body))
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return fmt.Errorf("http %d: %s", status, msg)
}
