## 1. Shared Limits And Admission

- [x] 1.1 Add 80 MiB absolute limit, 8 MiB threshold, 16 waiters, and 1 GiB
  aggregate budget configuration with hot reload.
- [x] 1.2 Implement FIFO weighted admission, singleton handling, cancellation,
  queue saturation, and idempotent release.
- [x] 1.3 Implement bounded memory-to-disk request spooling and cleanup.

## 2. Protocol Integration

- [x] 2.1 Route all JSON handlers through bounded encoded/decoded body reading.
- [x] 2.2 Bound zstd memory, window size, concurrency, and decoded output.
- [x] 2.3 Spool and admit Responses WebSocket messages with an 80 MiB read limit.
- [x] 2.4 Bound multipart image and video form parsing and remove unnecessary
  image Base64 duplication.

## 3. Error Isolation And Existing Copies

- [x] 3.1 Return `request_capacity_unavailable` only for local queue saturation.
- [x] 3.2 Verify local capacity failures occur before provider selection and do
  not alter auth retry or cooldown state.
- [x] 3.3 Preserve bounded request logging and plugin interceptor buffer
  isolation from the existing patch.

## 4. Verification

- [x] 4.1 Add unit and race tests for limits, queueing, cancellation, hot reload,
  cleanup, zstd, form parsing, WebSocket limits, and error codes.
- [x] 4.2 Run `go test ./...`, build `./cmd/server`, and `git diff --check`.
- [x] 4.3 Record production calibration as a separate deployment activity; do
  not connect to or modify the 80 server in this change.
