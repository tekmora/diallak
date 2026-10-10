<!--
SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>
SPDX-License-Identifier: CC-BY-SA-4.0
-->

# Download clients

SnatchArr watches the clients your *arr apps hand grabs to, so it stops snatching when the
grabs would only pile up. It never adds, pauses, resumes, removes or reorders a download.

Supported out of the box: **qBittorrent**, **Transmission**, **Deluge**, **rTorrent**,
**SABnzbd** and **NZBGet**.

## Getting them in

- **Discover**: `POST /api/v1/instances/{id}/download-clients/discover` reads the
  instance's own `/downloadclient` list and imports every supported client, credentials
  included. Clients the *arr no longer lists are removed again on the next discovery.
  Some *arr versions mask passwords in that list; those clients arrive without a secret
  and the result says so.
- **By hand**: `POST /api/v1/download-clients`. A hand-added client applies to every
  instance unless you pin it with `instance_id`.

`POST /api/v1/download-clients/test` observes credentials before you save them.

## What is observed

| Field | Meaning |
| --- | --- |
| `reachable` | The client answered and accepted the credentials |
| `paused` | Global pause (SABnzbd, NZBGet) |
| `active` | Downloads currently transferring |
| `queued` | Downloads waiting to start |
| `download_rate_bps` / `upload_rate_bps` | Current throughput |

`GET /api/v1/download-clients/status` returns the latest observation of every enabled
client; observations are cached for 20 seconds.

## Foreplay: backpressure and pacing

| Setting | Effect |
| --- | --- |
| client unreachable or paused | The instance's snatches are deferred until it is back |
| `max_active` | Defer snatches while at least this many downloads are active (0 = no limit) |
| `bandwidth_budget_bps` | Scale per-cycle counts by the share of the budget still free; a saturated budget defers (0 = off) |

Deferrals are recorded once as `snatch_deferred` and cleared with `snatch_resumed`; a paced
lease is recorded as a foreplay event naming the old and new per-cycle count. A client that
has never been observed does not block anything.

See [ADR-0006](adr/0006-download-client-backpressure.md) for the reasoning.
