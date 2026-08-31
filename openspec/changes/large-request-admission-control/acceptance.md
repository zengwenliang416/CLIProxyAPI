# Acceptance Criteria: large-request-admission-control

## User-Visible Criteria

- A valid 50-60 MiB request below the configured 80 MiB absolute limit is not
  rejected merely because it crosses the 8 MiB large-request threshold.
- When aggregate request capacity is temporarily unavailable, a large request
  waits in the bounded queue and proceeds when capacity is released.
- A client cancellation while waiting stops the request and removes its
  temporary files without invoking a provider.
- A request exceeding the absolute encoded or decoded limit returns HTTP `413`.
- When all configured waiting slots are occupied, the next large request
  receives a retryable local capacity error with code
  `request_capacity_unavailable`, not `auth_unavailable`.

## System Criteria

- The default absolute body limit is 80 MiB, the default large-request threshold
  is 8 MiB, the default waiting limit is 16, and the default weighted in-flight
  budget is 1 GiB.
- Requests at or below the soft threshold do not consume large-request queue
  slots.
- Weighted admission never reports more admitted bytes than the configured
  budget and supports a single request whose weight is at most that budget.
- zstd decoding uses bounded decoder memory, bounded window size, single
  decoder concurrency, and low-memory mode.
- Responses WebSocket applies an absolute read limit before reading messages
  and applies per-message large-request admission.
- Multipart image/video and URL-encoded requests cannot bypass the absolute
  request limit or retain avoidable duplicate full-body buffers.
- Hot-reloaded configuration updates the effective limits and admission policy
  without restarting the process.
- Local limit, queue, spool, and cancellation failures do not record provider
  auth failure, consume provider retries, or schedule provider cooldown.

## Data Criteria

- Temporary spools contain only the current request body, use restrictive file
  permissions inherited from the operating system temporary-file API, and are
  removed on every terminal path.
- No database or durable request queue is introduced.
- Logging captures at most its configured bounded prefix in memory and may move
  large bodies through disk-backed capture without duplicating the full body.

## Component Criteria

- Reusable components, hooks, utilities, or services named in
  `component-impact-map.json` are extracted instead of duplicated.

## Verification Surfaces

- Facticity: inspect config defaults, handler wiring, and error classification.
- Static: `gofmt`, `go vet` where practical, and `git diff --check`.
- Unit: body limit, weighted admission, FIFO wakeup, queue saturation,
  cancellation, cleanup, zstd bounds, WebSocket read limit, and multipart limits.
- Redteam: decompression bomb, oversized frame, queue flood, cancellation race,
  and temporary-file leak tests.
- E2E: protocol handler tests for JSON, WebSocket, multipart image, and video
  form paths without a real provider call.
- Sensory: not applicable to a backend-only change.

## Unresolved Gaps

- None.
