<!--
SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>
SPDX-License-Identifier: CC-BY-SA-4.0
-->

# Configuration

The API reads its configuration from environment variables (prefix `APP_`), the worker
from `SNATCH_*`. Everything else is stored in Postgres and edited through the API or the
UI.

## API (`snatcharr`)

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_DB_DSN` | required | Postgres DSN, e.g. `postgres://snatcharr:secret@postgres:5432/snatcharr?sslmode=disable` |
| `APP_HTTP_ADDR` | `:8080` | HTTP listen address (API, SSE, embedded UI) |
| `APP_GRPC_LISTEN` | `:9090` | gRPC listen address for workers |
| `APP_CRYPTO_KEY` | required in prod | Key that encrypts *arr API keys and download-client secrets at rest |
| `APP_HTTP_CSRF_SECRET` | required in prod | Secret behind the double-submit CSRF token; set it so tokens survive restarts and replicas |
| `APP_LEADER_ENABLED` | `false` | Elect one replica (Postgres advisory lock) to run the planner, Seerr sync and Configarr poll; leave off for a single replica |
| `APP_LEADER_NAME` | `snatcharr` | Lock name; one per deployment |
| `APP_LEADER_IDENTITY` | hostname | This replica's name in logs |
| `APP_LEADER_PG_RETRY` | `2s` | How often a follower retries the lock |
| `APP_SNATCHARR_PROFILE` | `prod` | `prod` or `dev`; `dev` relaxes cookie security and uses the built-in dev key |
| `APP_SNATCHARR_PUBLIC_URL` | | External base URL (cookies, links) |
| `APP_SNATCHARR_WORKER_TOKEN` | | Shared bearer the worker presents over gRPC |
| `APP_SNATCHARR_SESSION_TTL` | `8h` | Cookie session lifetime |
| `APP_SNATCHARR_SESSION_SECURE` | `true` | `Secure` flag on the session cookie |
| `APP_SNATCHARR_APIKEY_SECRET` | required in prod | HMAC secret for API keys |
| `APP_SNATCHARR_CONFIGARR_CONFIG` | | Path to Configarr `config.yml` (linked mode) |
| `APP_SNATCHARR_CONFIGARR_SECRETS` | | Path to Configarr `secrets.yml` |
| `APP_SNATCHARR_CONFIGARR_WATCH` | `true` | Re-import on file change |
| `APP_SNATCHARR_WEB_DEV` | `false` | Disable the embedded SPA (Vite dev server proxies instead) |
| `APP_OTEL_ENABLED` | `true` | OpenTelemetry export (no-op without an endpoint) |
| `APP_LOG_FORMAT` | `json` | `json` or `text` |

## Worker (`snatch-worker`)

| Variable | Default | Meaning |
| --- | --- | --- |
| `SNATCH_API_GRPC` | `http://snatcharr:9090` | API gRPC endpoint |
| `SNATCH_WORKER_TOKEN` | | Bearer token (must match `APP_SNATCHARR_WORKER_TOKEN`) |
| `SNATCH_CONCURRENCY` | `2` | Snatches executed in parallel |
| `RUST_LOG` | `info` | Log filter |

The worker never holds state. It leases a snatch, heartbeats while working, reports events
and completes; a restart loses nothing.

## Snatch policy

Every instance has one policy (`GET/PUT /api/v1/instances/{id}/policy`).

| Field | Default | Range |
| --- | --- | --- |
| `missing_per_cycle` | 1 | 0 to 100 (0 disables) |
| `upgrade_per_cycle` | 0 | 0 to 100 |
| `cycle_interval_s` | 900 | at least 60 |
| `hourly_cap` (stamina) | 20 | 1 to 500 |
| `selection` | `random` | `random`, `sequential`, `recent` |
| `monitored_only` | true | |
| `skip_future_releases` | true | |
| `radarr_release_type` | `physical` | `physical`, `digital`, `cinema` |
| `sonarr_missing_mode` | `episodes` | `episodes`, `season_packs`, `shows` |
| `sonarr_upgrade_mode` | `episodes` | `episodes`, `season_packs` |
| `lidarr_missing_mode` | `artist` | `artist`, `album` |
| `processed_ttl_h` (afterglow) | 168 | 1 to 8760 |
| `max_queue_size` | -1 | -1 disables the *arr queue ceiling |
| `await_command` | false | Poll the search command until it completes (5 min max) |
| `page_size` | 100 | 10 to 1000 |
| `recent_grab_window_h` | 24 | 0 to 720; items grabbed this recently (and everything queued) are skipped |
| `afterglow_max_h` | 720 | 1 to 8760; cap for the doubling afterglow |

## Runtime settings

`GET/PUT /api/v1/settings`:

| Field | Default | Meaning |
| --- | --- | --- |
| `history_retention_days` | 90 | 1 to 3650; how long events and finished runs are kept. The elected replica deletes older ones once an hour |
| `user_agent` | `SnatchArr/1.0 (...)` | Sent to every *arr and download client |
| `global_hourly_cap` | 0 | Stamina shared by every instance (0 = off) |

The same hourly pass also forgets afterglow entries 90 days after they expired, spent
stamina buckets after two hours and expired login sessions. It deletes at most 100,000 rows
per table per pass and logs one `retention: purge pass` line with the counts. A larger
backlog shrinks over the following hours.

## Configarr

Point `APP_SNATCHARR_CONFIGARR_CONFIG` (and `_SECRETS`) at your Configarr files and every
`sonarr`, `radarr`, `lidarr`, `readarr` and `whisparr` instance is imported on start and
kept in sync: the files are polled every 30 seconds (inotify misses Kubernetes ConfigMap
swaps, polling does not). Only `base_url`, `api_key` and the `enabled` flags are read;
`!secret`, `!env` and `!file` values are resolved like Configarr does. Whisparr entries are
probed to tell v2 from v3. An instance that disappears from the file is switched off, not
deleted. Imported instances are read-only in SnatchArr except for their snatch policy.

`POST /api/v1/configarr/import` runs a one-shot import (with `config_path` and
`secrets_path` in the body, or the linked files); `GET /api/v1/configarr/status` shows
linked mode and the last result.
