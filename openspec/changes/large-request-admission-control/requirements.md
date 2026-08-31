# Requirements: large-request-admission-control

## Summary

Allow legitimate requests up to the configured absolute limit to proceed
without allowing concurrent large bodies to exhaust process memory. Large
requests are spooled, admitted through a bounded weighted queue, and rejected
only when they exceed the absolute limit or the bounded waiting queue is full.

## Users & Actors

- API clients sending OpenAI, Claude, Gemini, Responses WebSocket, image, or
  video requests.
- Operators configuring request limits and aggregate in-flight capacity.
- Provider auth and executor subsystems, which must remain isolated from local
  request-capacity failures.

## In Scope

- Default absolute encoded and decoded request-body limit of 80 MiB.
- Configurable 8 MiB large-request threshold, 16 waiting large requests, and
  1 GiB aggregate in-flight request-memory budget.
- Low-memory temporary-file spooling before large-request admission.
- Context-aware weighted admission using actual decoded or parsed request size.
- HTTP JSON, zstd, Responses WebSocket, multipart image, multipart video, and
  URL-encoded form request boundaries.
- Bounded zstd decoder memory, window size, and concurrency.
- Deterministic capacity release and temporary-file cleanup on success, error,
  cancellation, and queue rejection.
- A retryable local capacity error that is distinct from provider auth,
  cooldown, and upstream transport failures.
- Hot reload of request-limit and admission settings.

## Out of Scope

- Changing provider credential selection, retry counts, or cooldown policy.
- Adding request timeouts after an upstream connection is established.
- Modifying protocol translation semantics in `internal/translator`.
- Removing support for legitimate 50-60 MiB requests.
- Adding a database, durable queue, or cross-process admission coordinator.
- Deploying or changing configuration on the 80 server.

## UI Design Impact

- Foundation spec: `openspec/specs/ui-design/design.md`
- Required UI decisions: none; this is a backend API and configuration change.

## Theme & Locale Capability Impact

- Theme support: `none`.
- Theme toggle policy: explicitly omit.
- Internationalization: `disabled`.
- Supported locales: none for UI.
- Default locale: none.
- Prototype coverage: none; backend behavior is covered by tests and benchmarks.

## Architecture & Database Impact

- Foundation spec: `openspec/specs/system-architecture/design.md`
- Required architecture/database decisions: add one shared in-process admission
  service owned by `sdk/api/handlers`; add no database, migration, durable
  queue, provider dependency, or deployment-specific dependency.

## Frontend-Backend Data Flow Impact

- Foundation spec: `openspec/specs/frontend-backend-data-flow/design.md`
- Required data-flow decisions: enforce the absolute limit while streaming to
  disk, calculate weight from the bounded parsed size, wait before materializing
  large in-memory representations, and release admission only after all
  request-owned buffers are no longer needed.

## Component Architecture Impact

- Foundation spec: `openspec/specs/component-architecture/design.md`
- Cohesion/coupling impact: protocol handlers reuse a provider-independent
  request-body and admission boundary; provider auth and executors remain
  unaware of admission state.
- Shared extraction requirement: extract reusable weighted admission, spool,
  bounded decompression, and capacity-error helpers rather than duplicating
  them across protocols.

## Unresolved Gaps

- None.
