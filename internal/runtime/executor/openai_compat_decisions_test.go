package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestOpenAICompatExecutorDecisionsPassthrough(t *testing.T) {
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"dec_1","object":"decision","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{{
			Name:                  "compat",
			SupportPromptCacheKey: true,
		}},
	})
	auth := &cliproxyauth.Auth{
		Provider: "openai-compatibility",
		Attributes: map[string]string{
			"base_url":     server.URL + "/v1",
			"api_key":      "test",
			"compat_name":  "compat",
			"provider_key": "compat",
		},
	}
	payload := []byte(`{"model":"gpt-6-luna(high)","input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}],"questions":[{"type":"score","id":"urgency"}],"stream":false}`)
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-6-luna(high)",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Alt:          "decisions",
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotPath != "/v1/decisions" {
		t.Fatalf("path = %q, want /v1/decisions", gotPath)
	}
	if gjson.GetBytes(gotBody, "model").String() != "gpt-6-luna" {
		t.Fatalf("model = %s", gjson.GetBytes(gotBody, "model").Raw)
	}
	if !gjson.GetBytes(gotBody, "input").Exists() || !gjson.GetBytes(gotBody, "questions").Exists() {
		t.Fatalf("input or questions dropped: %s", gotBody)
	}
	if gjson.GetBytes(gotBody, "messages").Exists() || gjson.GetBytes(gotBody, "prompt_cache_key").Exists() || gjson.GetBytes(gotBody, "stream").Exists() {
		t.Fatalf("translated or cache fields injected: %s", gotBody)
	}
	if string(resp.Payload) != `{"id":"dec_1","object":"decision","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}` {
		t.Fatalf("payload = %s", resp.Payload)
	}
}

func TestOpenAICompatExecutorDecisionsRejectsStream(t *testing.T) {
	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	_, err := executor.ExecuteStream(context.Background(), &cliproxyauth.Auth{}, cliproxyexecutor.Request{
		Model:   "gpt-6-luna",
		Payload: []byte(`{"model":"gpt-6-luna","input":"hi","questions":[{"type":"predicate"}]}`),
	}, cliproxyexecutor.Options{Alt: "decisions"})
	if statusCodeOf(err) != http.StatusBadRequest {
		t.Fatalf("status = %d, err = %v", statusCodeOf(err), err)
	}
}
