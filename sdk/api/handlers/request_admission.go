package handlers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

const (
	DefaultLargeRequestThresholdBytes    int64 = 8 << 20
	DefaultMaxWaitingLargeRequests             = 16
	DefaultMaxInflightRequestMemoryBytes int64 = 1 << 30
)

// RequestCapacityUnavailableError reports local request-capacity saturation.
// It is intentionally independent of provider auth and cooldown errors.
type RequestCapacityUnavailableError struct {
	MaxWaiting int
}

func (e *RequestCapacityUnavailableError) Error() string {
	return fmt.Sprintf("large request capacity queue is full (max waiting: %d)", e.MaxWaiting)
}

// IsRequestCapacityUnavailable reports whether err represents local admission
// saturation rather than an upstream or provider-auth failure.
func IsRequestCapacityUnavailable(err error) bool {
	var unavailable *RequestCapacityUnavailableError
	return errors.As(err, &unavailable)
}

type requestAdmissionPolicy struct {
	threshold  int64
	maxWaiting int
	budget     int64
}

func requestAdmissionPolicyFromConfig(cfg *config.SDKConfig) requestAdmissionPolicy {
	policy := requestAdmissionPolicy{
		threshold:  DefaultLargeRequestThresholdBytes,
		maxWaiting: DefaultMaxWaitingLargeRequests,
		budget:     DefaultMaxInflightRequestMemoryBytes,
	}
	if cfg == nil {
		return policy
	}
	if cfg.LargeRequestThresholdBytes > 0 {
		policy.threshold = cfg.LargeRequestThresholdBytes
	}
	if cfg.MaxWaitingLargeRequests > 0 {
		policy.maxWaiting = cfg.MaxWaitingLargeRequests
	}
	if cfg.MaxInflightRequestMemoryBytes > 0 {
		policy.budget = cfg.MaxInflightRequestMemoryBytes
	}
	maxBody := DefaultMaxDecodedRequestBodyBytes
	if cfg.MaxDecodedRequestBodyBytes > 0 {
		maxBody = cfg.MaxDecodedRequestBodyBytes
	}
	policy.threshold = min(policy.threshold, maxBody)
	return policy
}

type requestAdmissionWaiter struct {
	size    int64
	weight  int64
	ready   chan struct{}
	granted bool
}

// RequestAdmissionController serializes large request materialization against
// a configurable aggregate byte budget.
type RequestAdmissionController struct {
	mu       sync.Mutex
	policy   requestAdmissionPolicy
	inflight int64
	waiters  []*requestAdmissionWaiter
}

func NewRequestAdmissionController(cfg *config.SDKConfig) *RequestAdmissionController {
	return &RequestAdmissionController{policy: requestAdmissionPolicyFromConfig(cfg)}
}

// MaxRequestBodyBytes returns the effective absolute request-body limit.
func (h *BaseAPIHandler) MaxRequestBodyBytes() int64 {
	if h != nil && h.Cfg != nil && h.Cfg.MaxDecodedRequestBodyBytes > 0 {
		return h.Cfg.MaxDecodedRequestBodyBytes
	}
	return DefaultMaxDecodedRequestBodyBytes
}

// LargeRequestThresholdBytes returns the effective admission threshold.
func (h *BaseAPIHandler) LargeRequestThresholdBytes() int64 {
	limit := h.MaxRequestBodyBytes()
	if h != nil && h.Cfg != nil && h.Cfg.LargeRequestThresholdBytes > 0 {
		return min(h.Cfg.LargeRequestThresholdBytes, limit)
	}
	return min(DefaultLargeRequestThresholdBytes, limit)
}

// AcquireRequestCapacity reserves weighted capacity for a parsed request body.
func (h *BaseAPIHandler) AcquireRequestCapacity(ctx context.Context, size int64) (func(), error) {
	if h == nil {
		return func() {}, nil
	}
	if h.requestAdmission == nil {
		h.requestAdmission = NewRequestAdmissionController(h.Cfg)
	}
	return h.requestAdmission.Acquire(ctx, size)
}

// Update applies hot-reloaded limits and wakes waiters that now fit.
func (c *RequestAdmissionController) Update(cfg *config.SDKConfig) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.policy = requestAdmissionPolicyFromConfig(cfg)
	for _, waiter := range c.waiters {
		waiter.weight = min(waiter.size, c.policy.budget)
	}
	c.grantWaitersLocked()
	c.mu.Unlock()
}

// Acquire reserves weighted capacity for a large request. Requests at or below
// the threshold bypass admission. A request larger than the configured budget
// occupies the whole budget and is allowed to run alone.
func (c *RequestAdmissionController) Acquire(ctx context.Context, size int64) (func(), error) {
	if c == nil {
		return func() {}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.mu.Lock()
	policy := c.policy
	if size <= policy.threshold {
		c.mu.Unlock()
		return func() {}, nil
	}
	weight := size
	if weight > policy.budget {
		weight = policy.budget
	}
	if len(c.waiters) == 0 && c.inflight+weight <= policy.budget {
		c.inflight += weight
		c.mu.Unlock()
		return c.releaseFunc(weight), nil
	}
	if len(c.waiters) >= policy.maxWaiting {
		c.mu.Unlock()
		return nil, &RequestCapacityUnavailableError{MaxWaiting: policy.maxWaiting}
	}
	waiter := &requestAdmissionWaiter{size: size, weight: weight, ready: make(chan struct{})}
	c.waiters = append(c.waiters, waiter)
	c.mu.Unlock()

	select {
	case <-waiter.ready:
		if err := ctx.Err(); err != nil {
			release := c.releaseFunc(waiter.weight)
			release()
			return nil, err
		}
		return c.releaseFunc(waiter.weight), nil
	case <-ctx.Done():
		c.mu.Lock()
		if waiter.granted {
			c.inflight -= waiter.weight
			c.grantWaitersLocked()
		} else {
			for i, queued := range c.waiters {
				if queued == waiter {
					c.waiters = append(c.waiters[:i], c.waiters[i+1:]...)
					break
				}
			}
		}
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *RequestAdmissionController) releaseFunc(weight int64) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			c.inflight -= weight
			if c.inflight < 0 {
				c.inflight = 0
			}
			c.grantWaitersLocked()
			c.mu.Unlock()
		})
	}
}

func (c *RequestAdmissionController) grantWaitersLocked() {
	for len(c.waiters) > 0 {
		waiter := c.waiters[0]
		if c.inflight+waiter.weight > c.policy.budget {
			return
		}
		c.waiters = c.waiters[1:]
		c.inflight += waiter.weight
		waiter.granted = true
		close(waiter.ready)
	}
}
