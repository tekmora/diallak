<!--
SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>
SPDX-License-Identifier: CC-BY-SA-4.0
-->

# Concepts

SnatchArr's vocabulary is deliberately double-edged. Every term below maps to one exact
thing in the API, so you can enjoy the innuendo and still know what the numbers mean.

## Snatch

One run against one instance for one kind of item: a **missing snatch** goes after
monitored items that have no file, an **upgrade snatch** goes after items below their
quality cutoff. A snatch is planned by the API, leased by a worker, and ends with a list of
what was searched. Runs live under `/api/v1/runs`.

A worker that goes silent for 90 s loses its lease, and the snatch goes to the next worker.
After three leases that all expired, the snatch ends as failed ("lease expired 3 times without
the run completing") instead of being handed out again, so a snatch that crashes every worker
cannot loop forever. It counts as a failed run for the circuit breaker below.

The planner queues a snatch when the instance's **refractory period** (`cycle_interval_s`,
default 900 s, jittered by ±20 % per instance so several instances never hit the same
indexers in lockstep) has passed since the last one of that kind, nothing is paused by a
schedule, no download client is holding things back, and the circuit is closed (below).

## Per cycle

`missing_per_cycle` (default 1) and `upgrade_per_cycle` (default 0) say how many items a
single snatch goes after. Candidates are filtered **first** (monitored, released, not in
afterglow, not already in the download queue, not grabbed within `recent_grab_window_h`)
and selected **after**, so a snatch is never short because it drew an unlucky page.
Selection is `random` (a random page, then random items), `sequential` (a cursor that
walks the whole backlog) or `recent` (newest releases first, with one pick in five drawn
from the older backlog so it never starves).

## Afterglow

After an item has been snatched it rests. `processed_ttl_h` (default 168 h, one week) is how
long the first afterglow lasts. Every further fruitless snatch of the same item **doubles**
it (336 h, 672 h, ...) up to `afterglow_max_h` (default 720 h), so items that never turn
up stop burning stamina while fresh ones are retried soon. Missing and upgrade afterglows
are tracked separately, so an upgrade search never blocks a missing one.
`POST /api/v1/state/reset` ends every afterglow at once.

## Stamina

`hourly_cap` (default 20, range 1 to 500) is how many **indexer queries** an instance may
spend in any 60 minutes: one per episode in an episode search, one per season pack, one per
season of a series search, one per movie, album or book. It is debited atomically at
dispatch time, so two workers can never overspend it together. The window rolls: spent
stamina comes back 60 to 61 minutes after it was spent, never at the top of the clock hour,
so no 60 minutes ever see more than the cap. At 80 % the dashboard shows the instance
getting tired; at 100 % remaining targets are deferred until stamina frees up, with a
`budget_acquired` event that says so. When less stamina is left than a snatch wants, it
searches as many movies, albums, books or single episodes as the stamina covers and defers
the rest; a season pack, series, artist or author search is one command and waits whole.

Several *arr apps often share the same indexers. A **global stamina**
(`global_hourly_cap` in `/api/v1/settings`, 0 = off) is debited alongside every instance
cap, and `/api/v1/hourly-caps` lists it with `scope: global`.

!!! warning "Indexers ban impatient users"
    The cap exists because indexers count your API hits. Raising it above the default is
    your call; the UI warns you when you do.

## Foreplay

Download clients are observed, never driven ([ADR-0006](adr/0006-download-client-backpressure.md)).
When a client is unreachable, paused or already has `max_active` downloads, the instance's
snatches are deferred and history gets one `snatch_deferred` event (and one
`snatch_resumed` when it clears). With a `bandwidth_budget_bps`, the share of the budget
still free scales the per-cycle count down, so a busy line gets fewer new grabs.

## Circuit breaker

Three failed search commands in a row cut a snatch short (the items already searched
still count). Every failed run doubles the wait before the next attempt (30 min, 1 h, 2 h,
... up to 6 h) and records a `snatch_backoff` event saying when the circuit closes again,
so a dead indexer or instance never burns stamina on errors.

## Quickie

A manual run-now (`POST /api/v1/instances/{id}/runs`). It skips the refractory period and
the schedule but still respects stamina and afterglow, and it shows up in history as a
quickie so nobody wonders why a snatch happened at 3 pm on a Tuesday.

## Schedules

Weekly windows that either **pause** snatching or **override the cap** for one instance or
for everyone. Windows may cross midnight and carry their own time zone.

## History and live events

Everything above is recorded as events (`/api/v1/events`) and streamed over Server-Sent
Events (`/api/v1/events/stream`). Event names are lower snake case: `run_queued`,
`run_started`, `page_fetched`, `candidates_filtered`, `budget_acquired`,
`search_dispatched`, `search_completed`, `search_failed`, `run_finished`,
`snatch_deferred`, `snatch_resumed`, `snatch_paced`, `snatch_backoff`.
