# Stable Error Contract

CyberAgent Workbench preserves human-readable error text while assigning every failure a stable machine code. CLI callers use exit codes; the future Go HTTP API will use the same code through `apperror.CodeOf` and `apperror.HTTPStatus`.

| Code | CLI exit | HTTP status | Meaning |
| --- | ---: | ---: | --- |
| `INTERNAL` | 1 | 500 | Unexpected internal failure |
| `INVALID_ARGUMENT` | 2 | 400 | Invalid command, flag, input, or domain value |
| `NOT_FOUND` | 3 | 404 | Requested durable object or file does not exist |
| `CONFLICT` | 4 | 409 | Concurrent or uniqueness conflict |
| `FAILED_PRECONDITION` | 4 | 412 | Current state does not permit the operation |
| `POLICY_DENIED` | 5 | 403 | Safety policy rejected the operation |
| `UNAVAILABLE` | 6 | 503 | Dependency is temporarily unavailable |
| `CANCELLED` | 7 | 499 | Caller cancelled the operation |
| `RESOURCE_EXHAUSTED` | 8 | 429 | Capacity, quota, or budget is exhausted |
| `DEADLINE_EXCEEDED` | 9 | 504 | Operation exceeded its deadline |

`apperror.Normalize` provides a compatibility bridge for legacy plain Go errors. New application services should return typed errors directly and must not branch on human-readable text.

Provider failures use a separate `llm.Outcome` classification before mapping into this contract. Exhausted retryable transport failures map to `UNAVAILABLE`; rate limits map to `RESOURCE_EXHAUSTED`; invalid/permanent responses map to `FAILED_PRECONDITION`; caller cancellation and model deadlines map to `CANCELLED` and `DEADLINE_EXCEEDED`. The original typed Provider error remains available through Go error unwrapping, while persisted and user-facing text is redacted.

Run tool-call budget exhaustion also returns `RESOURCE_EXHAUSTED`/CLI exit 8 from direct Gateway-backed commands. The first rejected call beyond the configured limit appends one `tool.budget_exhausted` Run event; repeated attempts remain rejected without duplicating that event. Use `cyberagent run usage <run-id>` to inspect the durable counter and exhaustion timestamp.
