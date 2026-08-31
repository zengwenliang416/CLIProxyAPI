package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func TestDefaultMaxDecodedRequestBodyBytesIs80MiB(t *testing.T) {
	if DefaultMaxDecodedRequestBodyBytes != 80<<20 {
		t.Fatalf("DefaultMaxDecodedRequestBodyBytes = %d, want %d", DefaultMaxDecodedRequestBodyBytes, 80<<20)
	}
}

func TestReadRequestBodyWithLimitRejectsEncodedBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bytes.Repeat([]byte("x"), 9)))

	_, err := ReadRequestBodyWithLimit(ctx, 8)
	if !IsRequestBodyTooLarge(err) {
		t.Fatalf("ReadRequestBodyWithLimit() error = %v, want request body too large", err)
	}
}

func TestReadRequestBodyWithLimitRejectsDecodedZstdBody(t *testing.T) {
	encoder, errEncoder := zstd.NewWriter(nil)
	if errEncoder != nil {
		t.Fatalf("create zstd encoder: %v", errEncoder)
	}
	defer encoder.Close()

	encoded := encoder.EncodeAll(bytes.Repeat([]byte("x"), 4096), nil)
	if len(encoded) >= 1024 {
		t.Fatalf("compressed fixture = %d bytes, want below limit", len(encoded))
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(encoded))
	ctx.Request.Header.Set("Content-Encoding", "zstd")

	_, errRead := ReadRequestBodyWithLimit(ctx, 1024)
	if !IsRequestBodyTooLarge(errRead) {
		t.Fatalf("ReadRequestBodyWithLimit() error = %v, want decoded request body too large", errRead)
	}
}

func TestBaseAPIHandlerReadRequestBodyUsesConfiguredLimit(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("12345")))
	handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{MaxDecodedRequestBodyBytes: 4}, nil)

	_, err := handler.ReadRequestBody(ctx)
	if !IsRequestBodyTooLarge(err) {
		t.Fatalf("ReadRequestBody() error = %v, want configured request body limit", err)
	}

	WriteRequestBodyError(ctx, err)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("response status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestParseRequestFormRejectsOversizedBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("prompt=123456789"))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{MaxDecodedRequestBodyBytes: 8}, nil)

	err := handler.ParseRequestForm(ctx)
	if !IsRequestBodyTooLarge(err) {
		t.Fatalf("ParseRequestForm() error = %v, want request body too large", err)
	}
}

func TestParseRequestFormHoldsAdmissionUntilRelease(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("a=12"))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{
		MaxDecodedRequestBodyBytes:    64,
		LargeRequestThresholdBytes:    1,
		MaxWaitingLargeRequests:       1,
		MaxInflightRequestMemoryBytes: 64,
	}, nil)

	if err := handler.ParseRequestForm(ctx); err != nil {
		t.Fatalf("ParseRequestForm() error = %v", err)
	}
	handler.requestAdmission.mu.Lock()
	inflight := handler.requestAdmission.inflight
	handler.requestAdmission.mu.Unlock()
	if inflight == 0 {
		t.Fatal("ParseRequestForm() did not hold admission")
	}

	ReleaseRequestBody(ctx)
	handler.requestAdmission.mu.Lock()
	inflight = handler.requestAdmission.inflight
	handler.requestAdmission.mu.Unlock()
	if inflight != 0 {
		t.Fatalf("inflight after release = %d, want 0", inflight)
	}
}

func TestRequestBodySpoolRemovesTemporaryFile(t *testing.T) {
	spool, err := spoolRequestBody(nil, strings.NewReader("large body"), 64, 1)
	if err != nil {
		t.Fatalf("spoolRequestBody() error = %v", err)
	}
	if spool.file == nil {
		t.Fatal("spoolRequestBody() did not spill to disk")
	}
	name := spool.file.Name()
	if _, err = os.Stat(name); err != nil {
		t.Fatalf("temporary spool stat error = %v", err)
	}
	if err = spool.Close(); err != nil {
		t.Fatalf("spool Close() error = %v", err)
	}
	if _, err = os.Stat(name); !os.IsNotExist(err) {
		t.Fatalf("temporary spool still exists after Close(): %v", err)
	}
}
