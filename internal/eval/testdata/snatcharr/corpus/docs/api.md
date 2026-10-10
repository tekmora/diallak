<!--
SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>
SPDX-License-Identifier: CC-BY-SA-4.0
-->

# API

The HTTP API is an OpenAPI 3.1 contract at
[`api/openapi/openapi.yaml`](https://github.com/lusoris/SnatchArr/blob/main/api/openapi/openapi.yaml),
served under `/api/v1`. The Go server is generated from it with ogen; the web UI's client
types are generated from the same file. Errors are RFC 9457 problem details
(`application/problem+json`).

## Authentication

- **Browser sessions**: `POST /api/v1/auth/login` sets the `snatcharr_session` cookie.
  Unsafe methods need the `X-CSRF-Token` header, which `GET /api/v1/auth/session` returns.
- **API keys**: `Authorization: Bearer sk_...` on any request; no CSRF token needed.

The first start has no users; `GET /api/v1/auth/setup/status` says so and
`POST /api/v1/auth/setup` creates the admin.

## Resources

| Path | What |
| --- | --- |
| `/instances`, `/instances/{id}/test`, `/instances/test` | *arr instances and connectivity probes |
| `/instances/{id}/policy` | Snatch policy |
| `/instances/{id}/runs` | Quickie (manual snatch) |
| `/runs`, `/runs/{id}/cancel` | Snatch runs |
| `/events`, `/events/stream` | History and Server-Sent Events |
| `/hourly-caps` | Stamina per instance |
| `/state/reset` | End every afterglow |
| `/schedules` | Pause and cap-override windows |
| `/download-clients`, `/download-clients/status`, `/instances/{id}/download-clients/discover` | Download clients |
| `/system/status` | Version and build info |

## Worker contract

Workers talk gRPC (`proto/snatcharr/v1/worker.proto`): `LeaseRun`, `Heartbeat`,
`FilterCandidates`, `AcquireBudget`, `ReportEvents`, `CompleteRun`. The API is the only
source of truth for stamina and afterglow; a worker never decides on its own.
