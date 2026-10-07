package openai

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
	"github.com/tidwall/gjson"
)

type decisionsCaptureExecutor struct {
	alt          string
	sourceFormat string
	payload      []byte
	calls        int
}

func (e *decisionsCaptureExecutor) Identifier() string { return "decisions-test-provider" }

func (e *decisionsCaptureExecutor) Execute(_ context.Context, _ *coreauth.Auth, req coreexecutor.Request, opts coreexecutor.Options) (coreexecutor.Response, error) {
	e.calls++
	e.alt = opts.Alt
	e.sourceFormat = opts.SourceFormat.String()
	e.payload = append([]byte(nil), req.Payload...)
	return coreexecutor.Response{Payload: []byte(`{"ok":true}`)}, nil
}

func (e *decisionsCaptureExecutor) ExecuteStream(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	return nil, errors.New("not implemented")
}

func (e *decisionsCaptureExecutor) Refresh(_ context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	return auth, nil
}

func (e *decisionsCaptureExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, errors.New("not implemented")
}

func (e *decisionsCaptureExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	return nil, errors.New("not implemented")
}

func newDecisionsTestHandler(t *testing.T, executor *decisionsCaptureExecutor, authID string) *OpenAIResponsesAPIHandler {
	t.Helper()
	manager := coreauth.NewManager(nil, nil, nil)
	manager.RegisterExecutor(executor)
	auth := &coreauth.Auth{ID: authID, Provider: executor.Identifier(), Status: coreauth.StatusActive}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	registry.GetGlobalRegistry().RegisterClient(auth.ID, auth.Provider, []*registry.ModelInfo{{ID: "gpt-6-luna"}})
	t.Cleanup(func() {
		registry.GetGlobalRegistry().UnregisterClient(auth.ID)
	})
	return NewOpenAIResponsesAPIHandler(handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager))
}

func TestOpenAIResponsesDecisionsExecute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	executor := &decisionsCaptureExecutor{}
	h := newDecisionsTestHandler(t, executor, "decisions-auth-execute")
	router := gin.New()
	router.POST("/v1/decisions", h.Decisions)

	body := `{"model":"gpt-6-luna","input":"I was charged twice.","questions":[{"type":"choice","id":"department"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
	}
	if executor.calls != 1 {
		t.Fatalf("calls = %d, want 1", executor.calls)
	}
	if executor.alt != "decisions" {
		t.Fatalf("alt = %q", executor.alt)
	}
	if executor.sourceFormat != "openai-response" {
		t.Fatalf("source format = %q", executor.sourceFormat)
	}
	if gjson.GetBytes(executor.payload, "questions.0.id").String() != "department" {
		t.Fatalf("questions not forwarded: %s", executor.payload)
	}
	if strings.TrimSpace(resp.Body.String()) != `{"ok":true}` {
		t.Fatalf("body = %s", resp.Body.String())
	}
}

func TestOpenAIResponsesDecisionsRejectsStreamAndMissingQuestions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "stream", body: `{"model":"gpt-6-luna","input":"hi","stream":true,"questions":[{"type":"predicate"}]}`, want: "Streaming is not supported"},
		{name: "questions", body: `{"model":"gpt-6-luna","input":"hi","questions":[]}`, want: "questions"},
		{name: "input", body: `{"model":"gpt-6-luna","questions":[{"type":"predicate"}]}`, want: "input"},
		{name: "model", body: `{"input":"hi","questions":[{"type":"predicate"}]}`, want: "model"},
		{name: "json", body: `[]`, want: "Invalid JSON body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			executor := &decisionsCaptureExecutor{}
			h := newDecisionsTestHandler(t, executor, "decisions-auth-"+tc.name)
			router := gin.New()
			router.POST("/v1/decisions", h.Decisions)
			req := httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
			}
			if executor.calls != 0 {
				t.Fatalf("executor calls = %d, want 0", executor.calls)
			}
			if !strings.Contains(resp.Body.String(), tc.want) {
				t.Fatalf("body = %s, want substring %q", resp.Body.String(), tc.want)
			}
		})
	}
}

func TestOpenAIResponsesDecisionsDecodesZstdRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	executor := &decisionsCaptureExecutor{}
	h := newDecisionsTestHandler(t, executor, "decisions-auth-zstd")
	router := gin.New()
	router.POST("/v1/decisions", h.Decisions)

	var compressed bytes.Buffer
	encoder, err := zstd.NewWriter(&compressed)
	if err != nil {
		t.Fatalf("zstd.NewWriter: %v", err)
	}
	if _, errWrite := encoder.Write([]byte(`{"model":"gpt-6-luna","input":"hello","questions":[{"type":"score","id":"q"}]}`)); errWrite != nil {
		t.Fatalf("zstd write: %v", errWrite)
	}
	if errClose := encoder.Close(); errClose != nil {
		t.Fatalf("zstd close: %v", errClose)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/decisions", bytes.NewReader(compressed.Bytes()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "zstd")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
	}
	if executor.calls != 1 || executor.alt != "decisions" {
		t.Fatalf("calls=%d alt=%q", executor.calls, executor.alt)
	}
}
