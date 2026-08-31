## Why

Legitimate 50-60 MiB requests can create several full-size in-memory copies
across decoding, logging, interceptors, translation, and protocol adapters.
Concurrent large requests can therefore exhaust the proxy process and surface
as misleading provider `auth_unavailable` failures after the process or auth
runtime becomes unhealthy.

## What Changes

- Raise the default absolute encoded and decoded request limit to 80 MiB.
- Add a configurable 8 MiB soft threshold, 16-request bounded waiting queue,
  and 1 GiB aggregate weighted request-memory budget.
- Spool large HTTP and WebSocket inputs before materializing handler buffers.
- Bound zstd decoder memory, window size, and concurrency.
- Apply the same limits to JSON, Responses WebSocket, multipart image, and
  video form inputs.
- Return a distinct retryable `request_capacity_unavailable` error only when
  the bounded waiting queue is full.
- Preserve provider auth, retry, and cooldown behavior by rejecting local
  capacity failures before provider selection.

## Capabilities

### New Capabilities

- `large-request-admission`: Bounded request spooling, size enforcement,
  weighted admission, cancellation, cleanup, and local capacity errors.

### Modified Capabilities

- None.

## Impact

- Affected code: `sdk/api/handlers`, OpenAI protocol handlers,
  `internal/config`, request logging, and plugin interceptor adapters.
- Affected APIs: existing JSON, WebSocket, multipart, and form endpoints gain
  consistent size and local-capacity error behavior.
- Dependencies: no new external dependency, service, database, or migration.
- Operations: configuration can be hot reloaded; deployment values still need
  calibration against container memory, pprof, and real request amplification.
