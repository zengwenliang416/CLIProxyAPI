# Prototype Handoff: large-request-admission-control

## Approved Branch And Variant

The approved branch is `api-contract`; the approved variant is
`bounded-large-request-errors-v1` in `prototype/api/examples.json`.

## Screens Or Flows

The reviewed flow is request intake, absolute size validation, bounded waiting,
provider-independent rejection, and existing provider execution after admission.

## Components To Create

Create a shared weighted request admission controller and bounded request spool.

## Components To Reuse

Reuse `BaseAPIHandler`, Gin request cancellation, `ErrorResponse`, standard
multipart temporary files, and the existing provider execution boundary.

## Extraction Targets

Extract body spooling, zstd bounds, form limits, admission, release, cleanup,
and local capacity error classification into shared handler utilities/services.

## API Contracts

Use HTTP 413 for encoded or decoded bodies above the absolute limit. Use
retryable HTTP 503 with code `request_capacity_unavailable` only when the
bounded waiting queue is full. Never use `auth_unavailable` for local capacity.

## Data Flows

Spool and count input, acquire weighted capacity from actual size, materialize
the handler buffer, execute through the existing provider path, release
capacity, and clean temporary files.

## State Behavior

The relevant states are admitted, waiting, canceled, absolute-limit rejected,
capacity rejected, executing, and released. UI loading, empty, disabled, and
permission states are not applicable to this backend-only change.

## Theme And Locale Policy

Theme support is none, theme controls are omitted, i18n is disabled, and no UI
locale is defined.

## Out Of Scope

Deployment, 80-server configuration, durable queues, databases, provider retry
changes, cooldown changes, and translator redesign remain out of scope.

## Required Tests

Require unit/race tests for admission, cancellation, hot reload, cleanup, zstd,
forms, WebSocket size handling, error codes, full repository tests, and build.

## Open Risks

The 1 GiB byte budget is an initial application default. Production rollout
must calibrate it against pprof, RSS, temporary disk use, container memory, and
real protocol amplification before deployment.
