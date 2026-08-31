package config

import "testing"

func TestParseConfigBytesDecodedRequestBodyLimit(t *testing.T) {
	cfg, err := ParseConfigBytes([]byte(`
max-decoded-request-body-bytes: 1234
large-request-threshold-bytes: 234
max-waiting-large-requests: 7
max-inflight-request-memory-bytes: 5678
`))
	if err != nil {
		t.Fatalf("ParseConfigBytes() error = %v", err)
	}
	if cfg.MaxDecodedRequestBodyBytes != 1234 {
		t.Fatalf("MaxDecodedRequestBodyBytes = %d, want 1234", cfg.MaxDecodedRequestBodyBytes)
	}
	if cfg.LargeRequestThresholdBytes != 234 {
		t.Fatalf("LargeRequestThresholdBytes = %d, want 234", cfg.LargeRequestThresholdBytes)
	}
	if cfg.MaxWaitingLargeRequests != 7 {
		t.Fatalf("MaxWaitingLargeRequests = %d, want 7", cfg.MaxWaitingLargeRequests)
	}
	if cfg.MaxInflightRequestMemoryBytes != 5678 {
		t.Fatalf("MaxInflightRequestMemoryBytes = %d, want 5678", cfg.MaxInflightRequestMemoryBytes)
	}
}
