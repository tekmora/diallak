# ADR-0003: Configarr as an instance source

## Status

Accepted (2026-09-21)

## Context

Operators who run Configarr already declare every *arr instance (URL, API key) in `config.yml` + `secrets.yml`. Asking them to repeat that in SnatchArr doubles configuration and drifts.

## Decision

SnatchArr imports instances from Configarr's files:

- The importer walks the YAML as `yaml.Node` and resolves Configarr's custom tags: `!secret KEY` (secrets.yml), `!env NAME`, `!file PATH` (bounded to 64 KiB).
- Top-level `sonarr | radarr | lidarr | readarr | whisparr` maps become instances with `source=configarr` and a stable `configarr_key` (`sonarr.instance1`). Whisparr v2 vs v3 is resolved on the connectivity test.
- **Linked mode** watches the files and re-imports on change; linked instances are read-only in the UI except their snatch policy. **One-shot mode** imports an uploaded file once.
- A manual instance with the same normalised base URL is flagged; the Configarr definition wins.

## Consequences

- Kubernetes deployments mount the same ConfigMap/Secret Configarr uses.
- Renaming an instance in Configarr changes its `configarr_key`; SnatchArr keeps its own stable id and re-links by base URL when possible.
