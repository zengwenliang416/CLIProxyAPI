# Component Architecture & Reuse Spec

## Overview

This project uses Go packages and interfaces rather than browser components.
Component rules apply to handlers, middleware, services, adapters, and helpers.

## Component Taxonomy

- Page/screen components: none.
- Layout components: terminal UI only, outside this change.
- Domain components: protocol handlers, auth manager, translators, executors.
- Form components: multipart and URL-encoded request parsers.
- Data display components: terminal UI and management responses.
- Feedback components: structured HTTP/WebSocket errors.
- Headless hooks: plugin interceptors and execution lifecycle callbacks.
- Domain utilities/services: shared body reader, admission controller, logging spools.

## Cohesion Rules

- A component should have one clear reason to change.
- UI-only rendering, domain transformation, data fetching, and side effects must
  not be mixed unless this spec explicitly allows it.
- Keep component props aligned with user-visible behavior, not database internals.

## Coupling Rules

- Page components may compose shared components.
- Shared components must not import page-specific modules.
- Domain components may depend on domain types, but not on routing globals unless
  declared here.
- Infrastructure, API clients, and database code must not leak into presentational
  components.

## Shared Component Extraction Rules

Extract a component, hook, utility, or service when any of these are true:

- The same UI behavior appears in two or more screens.
- The same state machine is repeated.
- The same validation or formatting rule is repeated.
- A page-local component exceeds a single user-facing responsibility.
- A proposed implementation would duplicate a design-system control.

## Component Public API Rules

- Props must be stable, minimal, and behavior-facing.
- Do not expose raw database entities unless the component is explicitly a data
  boundary component.
- Events should name domain/user intent, not DOM implementation details.
- Slots/children are allowed only when they reduce coupling.

## State Ownership Rules

- Local state: request-scoped handler variables.
- Shared UI state: not applicable.
- Server/cache state: explicit synchronized services only.
- Form state: parsed request-scoped multipart/form values.
- URL state: Gin route parameters and query values.
- Derived state: request size, admission weight, translated payload, and routing metadata.

## Composition Patterns

- Preferred composition patterns: small package helpers behind exported interfaces.
- Forbidden composition patterns: protocol handlers directly implementing provider transports.
- Approved provider/context boundaries: handler to translator to executor; plugin host through plugin APIs.
- Approved headless hook patterns: request/response/stream interceptors with isolated mutable inputs.

## File & Naming Conventions

- Component file naming: existing Go package conventions.
- Hook naming: capability or lifecycle intent.
- Test naming: `Test<Behavior>` and `Benchmark<Behavior>`.
- Story/prototype naming: not applicable for backend-only changes.
- Barrel/export rules: export only stable SDK contracts.

## Testing Expectations

- Shared component tests: unit tests for admission, body limits, and cleanup.
- Hook tests: interceptor mutation and isolation tests.
- Integration tests: protocol handler and cross-module tests under `test`.
- Accessibility checks: not applicable.
- Visual/prototype review: not applicable.

## Refactor Triggers

- Duplicate logic detected:
- Cross-boundary import detected:
- Props become data-source-specific:
- Component grows multiple responsibilities:
- Test setup requires unrelated modules:

## Component Do's and Don'ts

- Do extract reusable UI, hooks, and domain utilities when the extraction rules trigger.
- Do keep shared components independent of page-specific state and routes.
- Do update this spec before adding a new shared component family.
- Don't copy/paste component logic across pages.
- Don't make low-level components know about API clients, database rows, or auth globals.
