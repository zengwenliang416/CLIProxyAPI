# Frontend-Backend Data Flow Spec

## Overview

This backend-only project receives protocol-compatible client requests and
returns provider-compatible responses. "Client" below means an HTTP or
WebSocket API consumer; there is no browser frontend.

## Flow Index

| Flow ID | Trigger | Entry UI | API/Service | Persistence | User Result |
| --- | --- | --- | --- | --- | --- |
| `FLOW-MODEL-HTTP` | HTTP model request | API client | Gin route and protocol handler | Optional temporary spool/log | JSON or SSE response |
| `FLOW-RESPONSES-WS` | WebSocket message | API client | Responses WebSocket handler | Session memory and optional logs | WebSocket events |
| `FLOW-MULTIPART-MEDIA` | Image/video form upload | API client | Media handler | Multipart temp files | JSON or SSE response |
| `FLOW-CAPACITY-ADMISSION` | Request exceeds soft threshold | None | Shared request admission controller | Ephemeral queue/spool only | Wait, execute, cancellation, or explicit busy error |

## Boundary Contracts

- Client event contract: valid HTTP request or WebSocket message on a declared
  route.
- Client state contract: clients own retry and cancellation state.
- Request schema: protocol-specific JSON, multipart form, URL-encoded form, or
  WebSocket JSON event.
- Response schema: protocol-compatible JSON, SSE, or WebSocket events.
- Error schema: protocol-compatible structured error with distinct local
  capacity, validation, authentication, and upstream failure codes.
- Permission contract: route admission occurs before provider credential
  selection.

## State Ownership

- URL state: Gin router.
- Local component state: not applicable.
- Shared client cache: not applicable.
- Server state: request lifecycle, admission budget, auth manager, plugin host,
  and provider sessions.
- Database state: none required.
- Derived state: decoded body size, admission weight, selected model/provider,
  stream mode, and translated payload.

## Validation Ownership

- Client-side validation: optional and never trusted.
- Server-side validation: absolute request limits, decompression bounds,
  protocol schema, route-specific fields, multipart file limits, and WebSocket
  message limits.
- Database constraints: not applicable.
- Cross-field rules: protocol handlers and canonical thinking/model routing.
- Error copy source: handler error builders and typed auth/executor errors.

## Error & Empty States

- Empty state: missing request body or required fields returns validation error.
- Permission denied: invalid API key or management auth returns auth error.
- Validation error: malformed or impossible-size request returns `400` or `413`.
- Network error: upstream transport error is translated by the executor path.
- Server error: internal failures return structured server errors.
- Capacity state: a valid large request waits while capacity exists in the
  bounded queue; queue saturation returns an explicit retryable busy response.

## Loading / Optimistic / Retry Behavior

- Initial loading: request bodies are counted and may be spooled before
  execution admission.
- Partial loading: streaming responses begin only after admission and upstream
  execution start.
- Optimistic update: not applicable.
- Retry rule: existing provider retry behavior remains unchanged; local
  admission does not consume provider retry attempts.
- Cancellation rule: client context cancellation removes a waiting admission
  request and cleans temporary files.
- Idempotency rule: existing idempotency metadata is preserved; admission must
  not duplicate execution.
- Rollback behavior: no durable state is written; release capacity and delete
  temporary files on every failed or canceled path.

## End-to-End Flow Details

### FLOW-MODEL-HTTP

1. Client sends an authenticated protocol request.
2. Middleware captures bounded logging data without retaining an unbounded body.
3. The shared body reader enforces the absolute encoded limit and decodes into
   a bounded spool.
4. The admission controller reserves weighted in-flight capacity based on the
   decoded size.
5. The handler loads the admitted body, validates protocol fields, and invokes
   request interceptors.
6. Translation and provider credential selection occur.
7. The executor sends the upstream request and returns JSON or stream chunks.
8. Response interceptors and protocol formatting run.
9. Request capacity is released and temporary files are cleaned.
10. Metrics record encoded/decoded size, wait state, rejection, and completion.

### FLOW-RESPONSES-WS

1. Client upgrades `/v1/responses`.
2. The server sets an absolute WebSocket read limit before `ReadMessage`.
3. Each accepted message enters the same weighted admission policy.
4. Execution and response events follow the Responses handler contract.
5. Per-message capacity is released after execution; session state stays
   separately bounded.

### FLOW-MULTIPART-MEDIA

1. Client submits a multipart or form request.
2. The request body and each file are bounded before form parsing.
3. Files remain disk-backed until the request receives capacity.
4. Media conversion or upstream multipart construction is bounded.
5. Temporary multipart files are removed after completion.

## Async / Realtime Flows

- Queue/event source: in-process bounded admission waiters.
- Subscriber: request goroutines waiting on their context.
- Retry/dead-letter behavior: no dead-letter queue; canceled waiters are
  removed, and saturated admission returns a retryable busy error.
- Realtime update channel: SSE or WebSocket provider response.
- Consistency expectation: one admitted request maps to at most one active
  provider execution per retry attempt.

## Flow Do's and Don'ts

- Do preserve valid large requests by waiting for capacity.
- Do bound queue length, absolute size, decompression memory, and session input.
- Do clean spools on every terminal path.
- Don't read a large body into memory before admission.
- Don't let local admission failures modify provider cooldown/auth state.
- Don't use an unbounded goroutine or channel queue.
