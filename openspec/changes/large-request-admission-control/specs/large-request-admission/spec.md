## ADDED Requirements

### Requirement: Absolute Request Limit

The proxy SHALL enforce one configurable absolute limit on encoded and decoded
HTTP bodies, WebSocket messages, multipart bodies, and URL-encoded forms. The
default limit SHALL be 80 MiB.

#### Scenario: Valid large request remains usable

- **WHEN** a valid request is 60 MiB and the configured absolute limit is 80 MiB
- **THEN** the proxy does not reject it solely because it exceeds the soft
  large-request threshold

#### Scenario: Encoded request exceeds the limit

- **WHEN** the encoded request body exceeds the configured absolute limit
- **THEN** the proxy returns HTTP 413 before provider selection

#### Scenario: Decoded request exceeds the limit

- **WHEN** a compressed request expands beyond the configured absolute limit
- **THEN** the proxy stops decoding and returns HTTP 413

### Requirement: Weighted Large-Request Admission

The proxy SHALL spool requests before materializing large handler buffers and
SHALL admit requests above the soft threshold against a bounded aggregate
weighted byte budget.

#### Scenario: Capacity becomes available

- **WHEN** a valid large request is waiting and an admitted request releases
  enough capacity
- **THEN** the waiting request proceeds without a client retry

#### Scenario: Request exceeds the configured budget

- **WHEN** one valid request is larger than the aggregate budget but remains
  below the absolute request limit
- **THEN** it is allowed to run alone and occupies the full admission budget

#### Scenario: Waiting queue is full

- **WHEN** the configured number of waiting large requests is already reached
- **THEN** the next large request receives a retryable local
  `request_capacity_unavailable` error

### Requirement: Cancellation And Cleanup

The proxy SHALL remove canceled waiters, release admission exactly once, and
delete request-owned temporary files on every terminal path.

#### Scenario: Client cancels while waiting

- **WHEN** a client context is canceled before admission
- **THEN** its queue slot and temporary spool are removed without invoking a
  provider

#### Scenario: Handler completes

- **WHEN** an admitted handler succeeds or fails
- **THEN** its admission weight is released and its request spool is deleted

### Requirement: Provider Isolation

Local request size, spool, admission, and cancellation failures SHALL remain
outside provider credential selection, retry, and cooldown state.

#### Scenario: Local capacity rejection

- **WHEN** a request is rejected because the local waiting queue is full
- **THEN** the response does not use `auth_unavailable` and no provider auth is
  marked failed or cooled down

### Requirement: Protocol Coverage

The proxy SHALL apply the shared protection boundary to JSON HTTP requests,
zstd content encoding, Responses WebSocket messages, multipart image requests,
and video multipart or URL-encoded forms.

#### Scenario: Oversized WebSocket message

- **WHEN** a Responses WebSocket message exceeds the absolute limit
- **THEN** the connection receives close code 1009 without provider execution

#### Scenario: Multipart forwarding

- **WHEN** a multipart image request is routed to a compatible upstream
- **THEN** the proxy avoids an unnecessary full Base64 conversion before
  rebuilding the upstream multipart body

### Requirement: Hot Reload

The proxy SHALL apply updated threshold, waiting, budget, and absolute-limit
configuration to new request admission decisions without process restart.

#### Scenario: Budget increases

- **WHEN** a hot reload increases the aggregate budget
- **THEN** queued requests that now fit are admitted
