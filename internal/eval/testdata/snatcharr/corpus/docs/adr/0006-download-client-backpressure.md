<!--
SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>
SPDX-License-Identifier: CC-BY-SA-4.0
-->

# ADR-0006: Download clients are observed for backpressure, never driven

- Status: accepted
- Date: 2026-09-21

## Context

Huntarr and newtarr only look at the *arr queue size before snatching. A snatch that succeeds
while the download client is unreachable, paused or saturated just piles up grabs the
client cannot take, and a bandwidth-starved client turns every extra search into a stalled
download. Users asked for the common torrent and Usenet clients to be first-class.

## Decision

SnatchArr observes qBittorrent, Transmission, Deluge, rTorrent, SABnzbd and NZBGet through
`goenvoy/downloadclient/*` and reduces each observation to one `Snapshot` (reachable,
paused, active, queued, download and upload rate). Client definitions are imported from
each *arr's `/downloadclient` list (secrets included, unless the *arr masks them) or added
by hand; a hand-added client applies to every instance unless it is pinned to one.

Two rules turn snapshots into snatching decisions:

1. **Backpressure gate** (planner, `snatch.Gate`): an unreachable or paused client, or one
   at its `max_active` limit, defers the instance's snatches. Transitions are recorded as
   `snatch_deferred` / `snatch_resumed` events, never one event per tick.
2. **Bandwidth pacing** (lease): with a `bandwidth_budget_bps`, the share of the budget
   still free scales `per_cycle` (`ceil(per_cycle * free)`, floor 1); a saturated budget
   blocks. The tightest client wins.

SnatchArr never adds, pauses, resumes, removes or reorders downloads (no Swaparr).
Observations are cached for 20 s and taken at most once per client per tick.

## Consequences

- The *arr queue ceiling (`max_queue_size`) stays as a cheap fallback for setups without
  a linked client.
- A client that was never observed does not block: ignorance is not backpressure.
- Secrets are encrypted at rest like *arr API keys and never returned by the API.
