# ADR-0001: Go control plane, Rust snatch-worker

## Status

Accepted (2026-09-21)

## Context

newtarr ran everything in one Python process: one thread per *arr app, JSON files as state, no coordination between instances, and a cap that was checked once per cycle. SnatchArr must scale the "snatching" part independently of the UI/API, survive restarts without losing memory of what was searched, and stay within a per-instance hourly budget even with several replicas.

## Decision

Split by responsibility, not by convenience:

- `api/` (Go, golusoris) is the **control plane**. It owns all durable state (instances, policies, schedules, processed memory, rate buckets, history) and every decision about *when* and *how much*.
- `worker/` (Rust, tokio + tonic) is the **data plane**. It executes snatch runs against *arr instances: paging wanted lists, filtering, dispatching search commands, reporting events. It holds no state beyond the run it is executing.

## Consequences

- The worker can be scaled with an HPA; the API stays a single-writer for budget and memory.
- *arr HTTP clients exist twice (goenvoy in Go for connectivity tests, a minimal `arr-client` crate in Rust for snatching). The Rust surface is deliberately small (wanted, command, queue, system/status).
- Two images, one Helm chart. Local dev runs both processes.
