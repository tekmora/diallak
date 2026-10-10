<!--
SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>
SPDX-License-Identifier: CC-BY-SA-4.0
-->

# Cleanuparr

[Cleanuparr](https://github.com/Cleanuparr/Cleanuparr) strikes and removes downloads
that stall, crawl, fail to import or turn out dead. SnatchArr decides what to search for
next. The two do not overlap ([ADR-0007](adr/0007-read-only-cleanuparr-link.md)),
and SnatchArr shows what Cleanuparr is doing right next to its own snatches.

## Linking

`POST /api/v1/cleanuparr` with the Cleanuparr URL and an API key (Cleanuparr → Settings
→ General). Nothing is ever written to Cleanuparr.

## What you see

`GET /api/v1/cleanuparr/status` (cached 30 s) returns, per link:

| Field | Meaning |
| --- | --- |
| `reachable`, `version`, `started_at` | Server health |
| `media_managers` | Instance count per *arr type Cleanuparr manages |
| `struck_downloads` | Downloads that currently carry strikes |
| `marked_for_removal`, `removed_downloads` | What Cleanuparr is about to, or already did, remove |
| `recent_strikes` | The latest strikes with type, title and download id |

## How it fits snatching

A download that Cleanuparr is striking is still in the *arr queue, so SnatchArr already
skips its item (see [Concepts](concepts.md#per-cycle)). When Cleanuparr removes a download,
the item becomes missing again; Cleanuparr can trigger a search itself and SnatchArr's
afterglow keeps its own next attempt from piling on. Strikes are keyed by download, not
by *arr item, so SnatchArr does not try to map them back to episodes or movies.
