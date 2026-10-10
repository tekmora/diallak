<!--
SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>
SPDX-License-Identifier: CC-BY-SA-4.0
-->

# Seerr

[Seerr](https://github.com/seerr-team/seerr) users ask for things; SnatchArr makes sure
those things get snatched first. The integration is read-only: SnatchArr never approves,
declines, retries or otherwise touches a request.

## Linking a Seerr server

`POST /api/v1/seerr` with the Seerr URL and an API key (Settings → General in Seerr).
Several links are fine. Each link can name a **fallback** Sonarr and Radarr instance for
requests whose Seerr server cannot be matched to a SnatchArr instance by URL.

`POST /api/v1/seerr/{id}/import-instances` reads the Sonarr and Radarr servers Seerr is
configured with (API keys included) and creates a SnatchArr instance for each one that
does not exist yet, so nothing has to be typed twice. Imported instances carry the source
`seerr` and stay editable.

## What is synced

Every five minutes (or on `POST /api/v1/seerr/{id}/sync`) SnatchArr fetches the
**approved but not yet available** requests (`filter=processing`) and maps each one:

1. **Instance**: the Seerr server the request names, matched by URL (4K servers to 4K
   requests), else Seerr's default server for that media type, else the link's fallback.
2. **Entity**: the Radarr movie (by TMDB id) or Sonarr series (by TVDB id) in that
   instance's library. A request whose media Seerr has not added to the *arr yet stays
   unresolved and is retried on the next sync.
3. **Title** and **requester**, for the dashboard.

Requests Seerr no longer lists as processing are dropped from the cache.

## Priority snatching

Resolved requests are handed to the worker with every snatch of that instance. Candidates
that belong to a requested movie or series are picked **before** anything else, within the
normal per-cycle count and stamina, so a request never waits behind the backlog. Afterglow
still applies: a requested item that was just searched rests like any other.

## Request dashboard

`GET /api/v1/seerr/requests` lists every cached request with its Seerr statuses, the
resolved instance and entity, when it was last snatched, and, for unresolved ones, why.
`POST /api/v1/seerr/requests/{linkId}/{requestId}/snatch` queues a quickie focused on that
one request.

| Field | Meaning |
| --- | --- |
| `request_status` | Seerr `MediaRequestStatus`: 1 pending, 2 approved, 3 declined, 4 failed, 5 completed |
| `media_status` | Seerr `MediaStatus`: 1 unknown, 2 pending, 3 processing, 4 partially available, 5 available |
| `resolved` | Mapped to an instance and a library entity; snatched first |
| `unresolved_reason` | What is missing when `resolved` is false |
| `last_snatched_at` | Last time a snatch searched for it |
