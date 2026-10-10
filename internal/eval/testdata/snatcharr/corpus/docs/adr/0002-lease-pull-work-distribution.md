# ADR-0002: Lease/pull work distribution over gRPC

## Status

Accepted (2026-09-21)

## Context

With the split from ADR-0001, work has to move from the API to one or more workers. A push model (API dials workers) needs worker discovery, breaks when a worker restarts mid-run, and makes the hourly cap hard to share across replicas.

## Decision

The worker **pulls**. `snatcharr.v1.WorkerService` (served by the API on `:9090`) offers:

1. `LeaseRun` — long-poll for a queued run; the API leases it with `SELECT … FOR UPDATE SKIP LOCKED` and a 90 s lease.
2. `Heartbeat` every 30 s extends the lease and can return `CANCEL`.
3. `FilterCandidates` — the API strips processed ids before selection (filter first, then select).
4. `AcquireBudget` — the API atomically debits the per-instance hourly bucket and returns the granted count.
5. `ReportEvents` — client stream of snatch events.
6. `CompleteRun` — the API records searched items into processed memory and closes the run.

Instance API keys travel in the lease payload; gRPC stays in-cluster behind a NetworkPolicy, optionally TLS, and the worker authenticates with a shared token.

## Consequences

- An expired lease requeues the run: worker crashes are safe.
- Budget and memory are enforced by one writer, so N workers cannot exceed the cap together.
- The worker needs no database and no configuration beyond the API address and token.
