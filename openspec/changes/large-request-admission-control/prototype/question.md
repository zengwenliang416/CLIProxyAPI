# Prototype Question: large-request-admission-control

## Question

Can one protocol-neutral API contract distinguish absolute-size rejection from
temporary local capacity saturation while preserving valid large requests and
keeping both failures outside provider authentication?

## Branch

`api-contract`

## Review Target

- Entry: `api/examples.json`
- Required reviewer decision: approve the `413 request_too_large` and retryable
  `503 request_capacity_unavailable` shapes as the shared handler contract.

## Out of Scope

- Production implementation.
- Database writes.
- Deployment behavior.
