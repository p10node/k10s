package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAskRequiresAPIKey(t *testing.T) {
	_, err := Ask(context.Background(), Config{Model: "gpt-5"}, Context{}, "hi")
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("Ask with no key: err = %v, want an API-key error", err)
	}
}

func TestAskRequiresModel(t *testing.T) {
	_, err := Ask(context.Background(), Config{APIKey: "sk-test"}, Context{}, "hi")
	if err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("Ask with no model: err = %v, want a model error", err)
	}
}

func TestAskOpenAISendsExpectedRequestAndParsesReply(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"pods look fine"}}]}`))
	}))
	defer srv.Close()

	cfg := Config{Provider: "openai", BaseURL: srv.URL, Model: "gpt-5", APIKey: "sk-test"}
	cc := Context{ClusterContext: "kind-dev", Namespace: "default", ResourceKind: "Pods", SelectedName: "web-1"}

	got, err := Ask(context.Background(), cfg, cc, "what's wrong with web-1?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if got != "pods look fine" {
		t.Errorf("Ask() = %q, want %q", got, "pods look fine")
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want Bearer sk-test", gotAuth)
	}
	if gotBody["model"] != "gpt-5" {
		t.Errorf("request model = %v, want gpt-5", gotBody["model"])
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("request messages = %v, want 2 entries (system, user)", msgs)
	}
	sys, _ := msgs[0].(map[string]any)
	if !strings.Contains(sys["content"].(string), "web-1") {
		t.Errorf("system prompt missing injected context: %v", sys["content"])
	}
}

func TestAskAnthropicSendsExpectedRequestAndParsesReply(t *testing.T) {
	var gotPath, gotKey, gotVersion string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"content":[{"text":"looks "},{"text":"healthy"}]}`))
	}))
	defer srv.Close()

	cfg := Config{Provider: "anthropic", BaseURL: srv.URL, Model: "claude-sonnet-5", APIKey: "sk-ant-test"}
	got, err := Ask(context.Background(), cfg, Context{}, "status?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if got != "looks healthy" {
		t.Errorf("Ask() = %q, want concatenated \"looks healthy\"", got)
	}
	if gotPath != "/messages" {
		t.Errorf("path = %q, want /messages", gotPath)
	}
	if gotKey != "sk-ant-test" {
		t.Errorf("x-api-key = %q, want sk-ant-test", gotKey)
	}
	if gotVersion == "" {
		t.Errorf("anthropic-version header missing")
	}
	if gotBody["model"] != "claude-sonnet-5" {
		t.Errorf("request model = %v, want claude-sonnet-5", gotBody["model"])
	}
}

func TestAskSurfacesHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	cfg := Config{Provider: "openai", BaseURL: srv.URL, Model: "gpt-5", APIKey: "bad-key"}
	_, err := Ask(context.Background(), cfg, Context{}, "hi")
	if err == nil {
		t.Fatal("expected an error for HTTP 401")
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("error = %v, want it to surface the server's message", err)
	}
}
