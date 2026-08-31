## Context

Existing protocol handlers consume `[]byte`, so a safe change must protect the
materialization point without rewriting translators or executors. Request
logging and plugin interceptors were already adjusted to avoid retaining
avoidable full-body copies, but HTTP decoding, WebSocket reads, and multipart
parsing still needed one shared capacity boundary.

## Goals / Non-Goals

**Goals:**

- Keep legitimate 50-60 MiB requests usable.
- Bound encoded size, decoded size, decompressor memory, queue length, and
  aggregate admitted bytes.
- Queue large requests with context cancellation instead of rejecting them
  merely for crossing a soft threshold.
- Release capacity and delete temporary files deterministically.
- Keep local capacity errors outside provider auth and cooldown state.
- Preserve config hot reload.

**Non-Goals:**

- Streaming protocol translation or executor payloads field by field.
- Durable or distributed queues.
- Provider retry or cooldown changes.
- Deployment or 80-server configuration changes.

## Decisions

### Spill before materialization

Inputs remain in memory only up to the soft threshold and then spill to a
restrictively created operating-system temporary file. The actual bounded size
is known before admission and before creating the handler `[]byte`.

Alternative: reject everything above a fixed size. Rejected because legitimate
60 MiB requests are a required use case.

Alternative: acquire from `Content-Length`. Rejected because it is optional,
can be inaccurate, and does not represent decoded zstd size.

### FIFO weighted admission

A process-local FIFO controller tracks admitted decoded or parsed bytes.
Requests at or below the soft threshold bypass it. A single valid request above
the configured budget is clamped to the full budget so it can run alone rather
than wait forever.

Alternative: fixed request-count semaphore. Rejected because one 60 MiB request
and one 9 MiB request do not create comparable memory pressure.

### Request-scoped release

HTTP handlers explicitly defer release. A request-context callback provides an
idempotent safety net for SDK consumers that do not use the built-in route
handlers. WebSocket messages release at the start of the next message or when
the session exits.

### Bounded zstd streaming decoder

Encoded input is spooled first. Decoding uses maximum memory and window options,
one decoder worker, low-memory mode, and a second bounded spool. The previous
compatibility behavior for clients that incorrectly label plain JSON as zstd is
retained only after the bounded encoded body is confirmed to be valid JSON.

### Multipart handling

`http.MaxBytesReader` is installed before form parsing. The standard library
keeps large file parts disk-backed, and parsed size is admitted before image or
video transformation. Compatible multipart forwarding skips unnecessary
Base64 conversion; providers that require data URLs use streaming Base64
encoding rather than a full source byte slice plus encoded copy.

## Risks / Trade-offs

- Temporary-file I/O increases for bodies above 8 MiB. This is intentional to
  exchange latency and disk I/O for bounded memory.
- The aggregate byte weight represents request-body materialization, not every
  downstream allocation. The 1 GiB default must be calibrated with pprof and
  container memory before production rollout.
- FIFO admission can cause head-of-line blocking when the first waiter is
  larger than later waiters. Predictable fairness is preferred over bypassing
  older requests.
- Disk exhaustion remains an operational boundary; the absolute limit and
  bounded waiter count cap per-process temporary request storage.

## Migration Plan

No data migration is required. Deploy the binary with configuration defaults
or explicit values, observe queue saturation, temporary storage, RSS, OOM
counters, and provider auth availability, then tune the byte budget if needed.
Rollback restores the prior binary and removes the four new configuration keys.

## Open Questions

- None for implementation. Production budget calibration remains an operations
  measurement, not a product requirement gap.
