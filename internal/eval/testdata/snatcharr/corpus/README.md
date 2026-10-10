<!--
SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>

SPDX-License-Identifier: CC-BY-SA-4.0
-->

# SnatchArr

<img src="web/static/brand/mark.svg" alt="" width="72" align="right" />

SnatchArr keeps your *arr libraries complete without hammering your indexers. It asks Sonarr, Radarr, Lidarr, Readarr and Whisparr (v2 and v3) to search for **missing** items and **cutoff-unmet upgrades** in small batches. Every search is a *snatch*: it remembers what it already snatched (the *afterglow*), never exceeds an instance's hourly *stamina*, and slows down (*foreplay*) while your download client is busy, paused or gone. Yes, the name means what you think it means.

Docs: <https://lusoris.github.io/SnatchArr/>

It is a from-scratch, cloud-native rewrite of the Huntarr lineage ([newtarr](https://github.com/elfhosted/newtarr)), built on the lusoris stack:

| Component | Stack |
| --- | --- |
| `api/` | Go 1.27 control plane on [golusoris](https://github.com/golusoris/golusoris) with [goenvoy](https://github.com/golusoris/goenvoy) *arr clients, OpenAPI 3.1, RFC 9457, SSE |
| `worker/` | Rust snatch-worker (tokio + tonic) that leases snatch runs from the API over gRPC |
| `web/` | SvelteKit SPA on [sveltesentio](https://github.com/golusoris/sveltesentio), WCAG 2.2 AA / EN 301 549, en + de |
| `deploy/` | Distroless OCI images, Helm chart, CloudNativePG, ArgoCD |

Governance: [praetor](https://github.com/cordanaLLM/praetor) (HISS invariants, canonical `AGENTS.md`).

## Status

Pre-alpha, heading for 0.1.0. Releases publish `ghcr.io/lusoris/snatcharr-api`,
`ghcr.io/lusoris/snatcharr-worker` and the chart `oci://ghcr.io/lusoris/charts/snatcharr`,
all cosign-signed with SBOM and provenance.

## Deploy

```sh
helm install snatcharr oci://ghcr.io/lusoris/charts/snatcharr --namespace snatcharr --create-namespace
```

Needs the CloudNativePG operator (or an external Postgres). Details, ArgoCD and a kind
recipe: [Deployment](https://lusoris.github.io/SnatchArr/deployment/).

## Instances from Configarr

If you already run [Configarr](https://github.com/raydak-labs/configarr), point SnatchArr at its `config.yml` and `secrets.yml` and every instance is imported (and kept in sync) instead of being configured twice.

## Running Cleanuparr too?

[Cleanuparr](https://github.com/Cleanuparr/Cleanuparr) cleans up what your download clients choke on; SnatchArr only decides what to search for next. They complement each other. v1 links Cleanuparr read-only and shows its status and recent strikes next to your snatches ([ADR-0007](docs/adr/0007-read-only-cleanuparr-link.md)). A download Cleanuparr is striking is still in the *arr queue, so SnatchArr already skips its item.

## License

Code: [EUPL-1.2](LICENSES/EUPL-1.2.txt). Documentation: [CC BY-SA 4.0](LICENSES/CC-BY-SA-4.0.txt). REUSE-compliant.

<!-- praetor:readme-governance:start -->
[![HISS Adopted][praetor-hiss-badge]][praetor-hiss-agents]

Praetor manages this repository's declared governance policy. This managed
block records adoption state; it is not a verification certificate.

**Verification**: `make verify-all` runs the repository's configured
verification cascade.

**HISS Audit**: `praetorctl audit` enforces policy, generated-surface
integrity, and the debt ratchet.

**Context Sync**: `praetorctl compile-context --verify` verifies every
generated agent context against `AGENTS.md`.

**Debt Baseline**: `.standards-baseline.json` anchors the debt ratchet at
0 recorded infractions; audit forbids growth.

[praetor-hiss-badge]: https://img.shields.io/badge/Standards-HISS%20Adopted-blue
[praetor-hiss-agents]: https://github.com/lusoris/SnatchArr/blob/HEAD/AGENTS.md
<!-- praetor:readme-governance:end -->
