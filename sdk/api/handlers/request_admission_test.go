package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func TestRequestAdmissionQueuesAndRejectsWhenWaitersAreFull(t *testing.T) {
	controller := NewRequestAdmissionController(&sdkconfig.SDKConfig{
		LargeRequestThresholdBytes:    1,
		MaxWaitingLargeRequests:       1,
		MaxInflightRequestMemoryBytes: 10,
	})
	releaseFirst, err := controller.Acquire(context.Background(), 10)
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer releaseFirst()

	secondResult := make(chan error, 1)
	secondRelease := make(chan func(), 1)
	go func() {
		release, errAcquire := controller.Acquire(context.Background(), 6)
		if errAcquire == nil {
			secondRelease <- release
		}
		secondResult <- errAcquire
	}()
	waitForAdmissionWaiters(t, controller, 1)

	_, err = controller.Acquire(context.Background(), 6)
	if !IsRequestCapacityUnavailable(err) {
		t.Fatalf("third Acquire() error = %v, want request capacity unavailable", err)
	}

	releaseFirst()
	select {
	case err = <-secondResult:
		if err != nil {
			t.Fatalf("queued Acquire() error = %v", err)
		}
		(<-secondRelease)()
	case <-time.After(time.Second):
		t.Fatal("queued Acquire() did not resume after capacity release")
	}
}

func TestRequestAdmissionCancellationRemovesWaiter(t *testing.T) {
	controller := NewRequestAdmissionController(&sdkconfig.SDKConfig{
		LargeRequestThresholdBytes:    1,
		MaxWaitingLargeRequests:       1,
		MaxInflightRequestMemoryBytes: 10,
	})
	release, err := controller.Acquire(context.Background(), 10)
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, errAcquire := controller.Acquire(ctx, 6)
		result <- errAcquire
	}()
	waitForAdmissionWaiters(t, controller, 1)
	cancel()

	select {
	case err = <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled Acquire() error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled Acquire() did not return")
	}
	waitForAdmissionWaiters(t, controller, 0)
}

func TestRequestAdmissionAllowsOversizedWeightToRunAlone(t *testing.T) {
	controller := NewRequestAdmissionController(&sdkconfig.SDKConfig{
		LargeRequestThresholdBytes:    1,
		MaxWaitingLargeRequests:       1,
		MaxInflightRequestMemoryBytes: 10,
	})
	release, err := controller.Acquire(context.Background(), 100)
	if err != nil {
		t.Fatalf("Acquire() error = %v, want valid request to run alone", err)
	}
	controller.mu.Lock()
	inflight := controller.inflight
	controller.mu.Unlock()
	if inflight != 10 {
		t.Fatalf("inflight = %d, want full budget weight 10", inflight)
	}
	release()
}

func TestRequestAdmissionHotUpdateWakesWaiter(t *testing.T) {
	controller := NewRequestAdmissionController(&sdkconfig.SDKConfig{
		LargeRequestThresholdBytes:    1,
		MaxWaitingLargeRequests:       1,
		MaxInflightRequestMemoryBytes: 10,
	})
	releaseFirst, err := controller.Acquire(context.Background(), 10)
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer releaseFirst()

	result := make(chan func(), 1)
	go func() {
		release, errAcquire := controller.Acquire(context.Background(), 6)
		if errAcquire != nil {
			result <- nil
			return
		}
		result <- release
	}()
	waitForAdmissionWaiters(t, controller, 1)

	controller.Update(&sdkconfig.SDKConfig{
		LargeRequestThresholdBytes:    1,
		MaxWaitingLargeRequests:       1,
		MaxInflightRequestMemoryBytes: 20,
	})
	select {
	case release := <-result:
		if release == nil {
			t.Fatal("queued Acquire() failed after hot update")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("hot update did not wake queued request")
	}
}

func TestRequestAdmissionHotUpdateShrinksQueuedWeight(t *testing.T) {
	controller := NewRequestAdmissionController(&sdkconfig.SDKConfig{
		LargeRequestThresholdBytes:    1,
		MaxWaitingLargeRequests:       1,
		MaxInflightRequestMemoryBytes: 100,
	})
	releaseFirst, err := controller.Acquire(context.Background(), 100)
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer releaseFirst()

	result := make(chan func(), 1)
	go func() {
		release, errAcquire := controller.Acquire(context.Background(), 80)
		if errAcquire != nil {
			result <- nil
			return
		}
		result <- release
	}()
	waitForAdmissionWaiters(t, controller, 1)

	controller.Update(&sdkconfig.SDKConfig{
		LargeRequestThresholdBytes:    1,
		MaxWaitingLargeRequests:       1,
		MaxInflightRequestMemoryBytes: 50,
	})
	releaseFirst()

	select {
	case release := <-result:
		if release == nil {
			t.Fatal("queued Acquire() failed after budget shrink")
		}
		controller.mu.Lock()
		inflight := controller.inflight
		controller.mu.Unlock()
		if inflight != 50 {
			t.Fatalf("inflight = %d, want shrunken full-budget weight 50", inflight)
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("oversized queued request did not run alone after budget shrink")
	}
}

func TestWriteRequestBodyErrorUsesLocalCapacityCode(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	WriteRequestBodyError(ctx, &RequestCapacityUnavailableError{MaxWaiting: 1})

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"code":"request_capacity_unavailable"`) || strings.Contains(body, "auth_unavailable") {
		t.Fatalf("response body = %s, want local capacity code without auth error", body)
	}
}

func waitForAdmissionWaiters(t *testing.T, controller *RequestAdmissionController, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		controller.mu.Lock()
		got := len(controller.waiters)
		controller.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("waiters did not reach %d", want)
}
