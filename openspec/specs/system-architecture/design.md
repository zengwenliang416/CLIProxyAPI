# System Architecture & Database Spec

## Overview

CLIProxyAPI is a Go 1.26+ HTTP/WebSocket proxy that exposes OpenAI, Claude,
Gemini, Codex, and compatible API surfaces. It translates requests, selects an
authorized provider credential, executes upstream calls, and translates
responses back to the caller. The server has no project-owned frontend or
relational database.

## Application Topology

- Frontend runtime: none; the optional terminal UI is under `internal/tui`.
- Backend runtime: one Go server created by `cmd/server`.
- API gateway or edge layer: deployment-owned Nginx/Cloudflare may sit in front
  of the Go server but is not part of this repository.
- Background workers: configuration watchers, model registry updates, logging,
  usage aggregation, and optional Home integration.
- External services: model providers, optional storage backends, Redis protocol
  integration, and optional Home control services.
- Local development entrypoints: `go run ./cmd/server`.
- Production deployment shape: compiled server binary or Docker container.

## Module Boundaries

- `internal/api`: Gin server, route registration, middleware, management API,
  and runtime configuration reload.
- `sdk/api/handlers`: protocol-facing request parsing, execution orchestration,
  response formatting, streaming, and plugin interceptor integration.
- `internal/translator`: provider protocol translations. It must remain
  independent of HTTP routing and deployment configuration.
- `internal/runtime/executor`: provider-specific upstream execution.
- `sdk/cliproxy/auth`: credential selection, cooldown, retry, and Home
  concurrency integration.
- `internal/pluginhost`: plugin lifecycle and isolated interceptor invocation.
- `internal/logging`: request, response, streaming, and file-backed log capture.
- `internal/config`: configuration parsing and validation.
- `internal/store`: optional configuration/auth storage backends.

Public contracts are the exported Go SDK interfaces and HTTP/WebSocket routes.
Handlers may depend on translators, executors, auth, logging, and plugin
interfaces. Translators must not depend on Gin handlers. Provider executors must
not own public request validation.

Shared request-admission component:

- Responsibility: bound request size, decompression, spooling, and aggregate
  in-flight memory before provider execution.
- Public contract: acquire request capacity with context cancellation and return
  a release function.
- Owned data: in-process admission counters, bounded waiter state, and ephemeral
  request spool metadata.
- Dependencies: Go standard library synchronization, filesystem, and request
  contexts.
- Forbidden dependencies: provider credentials, translators, executors, and
  deployment-specific Nginx state.
- Extension points: metrics callbacks and protocol adapters for HTTP,
  WebSocket, and multipart inputs.

## Frontend Architecture

There is no browser frontend. The terminal UI must consume service APIs and
configuration boundaries rather than bypassing them. No browser routing, form
state, client cache, design-system runtime, theme switcher, or locale switcher
is in scope.

## Backend Architecture

- API style: JSON HTTP APIs, multipart upload APIs, SSE streams, and selected
  WebSocket routes.
- Request validation: protocol handlers own shape validation; shared body-size
  and admission controls belong in `sdk/api/handlers`.
- Auth/session model: API key admission plus provider OAuth/API-key credential
  selection through the auth manager.
- Domain service boundaries: handlers orchestrate; translators transform;
  executors perform upstream I/O.
- Background jobs: model/config watchers and bounded observability workers.
- File/object storage: optional auth/config backends and temporary file-backed
  logging/request spooling.
- Observability: logrus, request logs, usage aggregation, pprof, and deployment
  metrics.

## API Surface

| Route or RPC | Owner | Input | Output | Auth | Side Effects |
| --- | --- | --- | --- | --- | --- |
| `/v1/chat/completions` | OpenAI handler | JSON | JSON/SSE | API key | Upstream model request |
| `/v1/responses` | Responses handler | JSON/WebSocket messages | JSON/SSE/WebSocket events | API key | Upstream model request |
| `/v1/messages` | Claude handler | JSON | JSON/SSE | API key | Upstream model request |
| `/v1beta/*` | Gemini handler | JSON | JSON/SSE | API key | Upstream model request |
| `/v1/images/*` | OpenAI images handler | JSON/multipart | JSON/SSE | API key | Upstream image request |
| `/v1/videos` and related routes | OpenAI videos handler | JSON/form | JSON | API key | Upstream video request |
| `/v0/management/*` | Management handlers | JSON/multipart | JSON | Management auth | Configuration/auth mutation |

## Database Model

The project has no required relational database entities. Configuration,
credential, plugin, and optional usage state are owned by their respective
storage abstractions. This change must not add a database, migration, or durable
queue. Temporary request spools are ephemeral files and must be cleaned on
success, error, or cancellation.

- Entity purpose: no new durable entity; request spools exist only during one
  request lifecycle.
- Entity fields: spool path, encoded size, decoded size, and admission weight.
- Entity relationships: one spool belongs to one request context.
- Entity indexes: not applicable.
- Entity retention: delete immediately after request completion, cancellation,
  or failed admission.

## Permissions & Security

- Public model routes require configured API-key admission unless explicitly
  configured otherwise.
- Management routes use separate management authentication.
- Provider credentials and request authorization data must never be logged.
- Request bodies from untrusted clients require absolute size, decompression,
  WebSocket frame, multipart, and aggregate in-flight memory bounds.
- Plugin inputs remain isolated from shared handler buffers.
- Capacity rejection must use a distinct error code and must not masquerade as
  provider authentication failure.

## Integration Boundaries

- Model providers are accessed only through provider executors.
- Plugin hooks are accessed only through plugin APIs and `internal/pluginhost`.
- Nginx, Cloudflare, Docker limits, and external metrics are deployment
  boundaries; application limits must remain independently safe.
- No payment, email, SMS, or project-owned analytics integration is required.

## Operational Constraints

- Valid large requests must be queued by bounded admission rather than rejected
  solely for crossing a soft threshold.
- An absolute request limit remains mandatory to prevent impossible or abusive
  workloads.
- Temporary request spooling must be bounded and cleaned deterministically.
- No new timeout may be added after an upstream connection is established.
- Configuration hot reload must update request limits without process restart.
- Required validation is `go test ./...`, the repository build command, and
  `git diff --check`.
- Deployment must preserve rollback configuration and verify OOM counters,
  memory peaks, request rejections, and provider auth availability.

## Architecture Do's and Don'ts

- Do keep admission control shared across protocol handlers.
- Do separate absolute safety limits from soft large-request thresholds.
- Do keep provider auth selection independent of local capacity admission.
- Do use file-backed spooling before reserving large in-memory buffers.
- Don't add an unbounded queue or unbounded decompression window.
- Don't return `auth_unavailable` for local memory pressure.
- Don't introduce a new service or database for request admission.
