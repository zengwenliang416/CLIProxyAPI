package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

type decisionsTestStatusError struct {
	code int
	msg  string
}

func (e decisionsTestStatusError) Error() string   { return e.msg }
func (e decisionsTestStatusError) StatusCode() int { return e.code }

type decisionsTestExecutor struct {
	calls int
	err   error
}

func (e *decisionsTestExecutor) Identifier() string { return "decisions-cooldown-provider" }

func (e *decisionsTestExecutor) Execute(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	e.calls++
	if e.err != nil {
		return cliproxyexecutor.Response{}, e.err
	}
	return cliproxyexecutor.Response{Payload: []byte(`{"status":"ok"}`)}, nil
}

func (e *decisionsTestExecutor) ExecuteStream(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, errors.New("stream not supported")
}

func (e *decisionsTestExecutor) Refresh(context.Context, *Auth) (*Auth, error) { return nil, nil }

func (e *decisionsTestExecutor) CountTokens(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, errors.New("not supported")
}

func (e *decisionsTestExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, errors.New("not supported")
}

func TestManager_Decisions_RequestFault_StopsFallback(t *testing.T) {
	executor := &decisionsTestExecutor{
		err: decisionsTestStatusError{code: http.StatusNotFound, msg: "404 endpoint not found"},
	}
	m := NewManager(nil, nil, nil)
	m.RegisterExecutor(executor)

	model := "gpt-6-luna"
	auth1 := &Auth{ID: "decisions-cooldown-auth1", Provider: executor.Identifier(), Status: StatusActive}
	auth2 := &Auth{ID: "decisions-cooldown-auth2", Provider: executor.Identifier(), Status: StatusActive}
	if _, err := m.Register(context.Background(), auth1); err != nil {
		t.Fatalf("Register auth1: %v", err)
	}
	if _, err := m.Register(context.Background(), auth2); err != nil {
		t.Fatalf("Register auth2: %v", err)
	}
	registry.GetGlobalRegistry().RegisterClient(auth1.ID, auth1.Provider, []*registry.ModelInfo{{ID: model}})
	registry.GetGlobalRegistry().RegisterClient(auth2.ID, auth2.Provider, []*registry.ModelInfo{{ID: model}})
	t.Cleanup(func() {
		registry.GetGlobalRegistry().UnregisterClient(auth1.ID)
		registry.GetGlobalRegistry().UnregisterClient(auth2.ID)
	})

	req := cliproxyexecutor.Request{Model: model, Payload: []byte(`{"input":"hello","questions":[{"type":"predicate"}]}`)}
	opts := cliproxyexecutor.Options{Alt: "decisions"}
	_, errExec := m.Execute(context.Background(), []string{executor.Identifier()}, req, opts)
	if errExec == nil {
		t.Fatal("Execute expected error, got nil")
	}
	if executor.calls != 1 {
		t.Fatalf("executor.calls = %d, want 1", executor.calls)
	}
	for _, id := range []string{auth1.ID, auth2.ID} {
		a, ok := m.GetByID(id)
		if !ok {
			t.Fatalf("auth %s not found", id)
		}
		if state, exists := a.ModelStates[model]; exists && state != nil && state.Unavailable {
			t.Fatalf("auth %s marked unavailable after decisions 404", id)
		}
	}
}
