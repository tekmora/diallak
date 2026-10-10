<!--
SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>
SPDX-License-Identifier: CC-BY-SA-4.0
-->

# SnatchArr

**Keeps your \*arr libraries complete without hammering your indexers.**
Gentle by default, cloud-native from day one, and yes, the name means exactly what you
think it means.

SnatchArr asks Sonarr, Radarr, Lidarr, Readarr and Whisparr (v2 and v3) to search for
**missing** items and **cutoff-unmet upgrades** in small batches. Every search is a
*snatch*. It remembers what it already snatched (the *afterglow*), never exceeds an
instance's hourly *stamina*, and slows down (*foreplay*) when your download client is
busy, paused or gone.

It is a from-scratch rewrite of the Huntarr lineage ([newtarr](https://github.com/elfhosted/newtarr)):
same intent, none of the code.

## Why another one

| Huntarr / newtarr | SnatchArr |
| --- | --- |
| One setting set per app, shared by all instances | A snatch policy **per instance** |
| Hourly cap checked once per cycle | Stamina debited **per dispatch**, shared across workers |
| Processed items never expire | Afterglow with a TTL per item, separate for missing and upgrades |
| Looks at the *arr queue size only | Watches qBittorrent, Transmission, Deluge, rTorrent, SABnzbd and NZBGet for backpressure |
| Python, single process, JSON files | Go control plane, Rust worker, Postgres, Helm, OpenAPI 3.1 |

## Where things live

- [Concepts](concepts.md): snatches, afterglow, stamina, foreplay, quickies.
- [Configuration](configuration.md): environment, profiles, Configarr link.
- [Download clients](download-clients.md): discovery, backpressure, bandwidth pacing.
- [Seerr](seerr.md): requests snatched first, request dashboard, instance import.
- [Cleanuparr](cleanuparr.md): status and strikes next to your snatches.
- [API](api.md): the OpenAPI contract and how to talk to it.
- [Deployment](deployment.md): images, Helm chart, CloudNativePG, ArgoCD, kind.
- [Architecture decisions](adr/README.md): why it is built the way it is.

## Status

Pre-alpha. The first cut will be **0.1.0**.

Code is [EUPL-1.2](https://github.com/lusoris/SnatchArr/blob/main/LICENSES/EUPL-1.2.txt),
documentation is CC BY-SA 4.0.
