package executor

import (
	"context"
	"errors"
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

func TestCodexExecutorDecisionsPassthrough(t *testing.T) {
	var gotPath string
	var gotBody []byte
	var gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"dec_1","object":"decision","answers":[{"type":"choice","choice":"billing"}],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}`))
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test",
	}}
	payload := []byte(`{"model":"gpt-6-luna(high)","input":"I was charged twice.","stream":false,"questions":[{"type":"choice","id":"department","options":["billing","technical"]}],"metadata":{"source":"client"}}`)
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
	if gotPath != "/decisions" {
		t.Fatalf("path = %q, want /decisions", gotPath)
	}
	if gotAccept != "application/json" {
		t.Fatalf("Accept = %q, want application/json", gotAccept)
	}
	if gjson.GetBytes(gotBody, "model").String() != "gpt-6-luna" {
		t.Fatalf("model = %s, body=%s", gjson.GetBytes(gotBody, "model").Raw, gotBody)
	}
	if gjson.GetBytes(gotBody, "input").String() != "I was charged twice." {
		t.Fatalf("input = %s", gjson.GetBytes(gotBody, "input").Raw)
	}
	if gjson.GetBytes(gotBody, "questions.0.id").String() != "department" {
		t.Fatalf("questions changed: %s", gotBody)
	}
	if gjson.GetBytes(gotBody, "metadata.source").String() != "client" {
		t.Fatalf("metadata changed: %s", gotBody)
	}
	for _, field := range []string{"stream", "prompt_cache_key", "instructions", "tools"} {
		if gjson.GetBytes(gotBody, field).Exists() {
			t.Fatalf("field %s was injected: %s", field, gotBody)
		}
	}
	const want = `{"id":"dec_1","object":"decision","answers":[{"type":"choice","choice":"billing"}],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}`
	if string(resp.Payload) != want {
		t.Fatalf("payload = %s", resp.Payload)
	}
}

func TestCodexWebsocketsExecutorDecisionsUsesHTTP(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"dec_ws","object":"decision"}`))
	}))
	defer server.Close()

	executor := NewCodexWebsocketsExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test",
	}}
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-6-luna",
		Payload: []byte(`{"model":"gpt-6-luna","input":"hello","questions":[{"type":"predicate","id":"yes"}]}`),
	}, cliproxyexecutor.Options{Alt: "decisions"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotPath != "/decisions" {
		t.Fatalf("path = %q, want /decisions", gotPath)
	}
	if string(resp.Payload) != `{"id":"dec_ws","object":"decision"}` {
		t.Fatalf("payload = %s", resp.Payload)
	}
}

func TestCodexExecutorDecisionsRejectsStream(t *testing.T) {
	executor := NewCodexExecutor(&config.Config{})
	_, err := executor.ExecuteStream(context.Background(), &cliproxyauth.Auth{}, cliproxyexecutor.Request{
		Model:   "gpt-6-luna",
		Payload: []byte(`{"model":"gpt-6-luna","input":"hello","questions":[{"type":"predicate"}]}`),
	}, cliproxyexecutor.Options{Alt: "decisions"})
	if statusCodeOf(err) != http.StatusBadRequest {
		t.Fatalf("status = %d, err = %v", statusCodeOf(err), err)
	}

	wsExecutor := NewCodexWebsocketsExecutor(&config.Config{})
	_, err = wsExecutor.ExecuteStream(context.Background(), &cliproxyauth.Auth{ID: "ws-decisions"}, cliproxyexecutor.Request{
		Model:   "gpt-6-luna",
		Payload: []byte(`{"model":"gpt-6-luna","input":"hello","questions":[{"type":"predicate"}]}`),
	}, cliproxyexecutor.Options{Alt: "decisions"})
	if statusCodeOf(err) != http.StatusBadRequest {
		t.Fatalf("websocket status = %d, err = %v", statusCodeOf(err), err)
	}
}

func TestCodexExecutorDecisionsRejectsMissingQuestions(t *testing.T) {
	executor := NewCodexExecutor(&config.Config{})
	_, err := executor.Execute(context.Background(), &cliproxyauth.Auth{
		Attributes: map[string]string{"base_url": "http://127.0.0.1:1", "api_key": "test"},
	}, cliproxyexecutor.Request{
		Model:   "gpt-6-luna",
		Payload: []byte(`{"model":"gpt-6-luna","input":"hello","questions":[]}`),
	}, cliproxyexecutor.Options{Alt: "decisions"})
	if statusCodeOf(err) != http.StatusBadRequest {
		t.Fatalf("status = %d, err = %v", statusCodeOf(err), err)
	}
}

func statusCodeOf(err error) int {
	var coder interface{ StatusCode() int }
	if errors.As(err, &coder) && coder != nil {
		return coder.StatusCode()
	}
	return 0
}
