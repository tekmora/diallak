<!--
SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>
SPDX-License-Identifier: CC-BY-SA-4.0
-->

# ADR-0007: Read-only Cleanuparr link in v1

- Status: accepted
- Date: 2026-10-09
- Supersedes: [ADR-0004](0004-no-swaparr-cleanuparr-deferred.md), its Cleanuparr part. The
  decision to ship no Swaparr-style download handling stands.

## Context

ADR-0004 deferred a read-only Cleanuparr integration to v1.1 behind a `Disabled`
implementation. It assumed Cleanuparr's API accepts only a JWT login, so integrating would
mean storing a Cleanuparr username and password, and it planned Seeker detection plus a
`respect_cleanuparr_strikes` policy flag that skips struck items.

Cleanuparr also accepts an API key (Settings → General), and a read-only link shipped in v1
(e684a1b). Cleanuparr keys its strikes by download, not by *arr item.

## Decision

- SnatchArr still never adds, pauses, removes or reorders downloads. Cleanuparr owns that
  domain (ADR-0004, ADR-0006).
- v1 links Cleanuparr read-only. `/api/v1/cleanuparr` stores a link's URL and API key; the
  key is sealed with `core/crypto` (`cleanuparr_links.api_key_enc`) and never returned.
  `GET /api/v1/cleanuparr/status`, cached for 30 s, shows each link's health, its instance
  count per *arr type, its struck, marked-for-removal and removed download counts, and the
  latest strikes.
- Nothing is written to Cleanuparr.
- Strikes are not mapped back to *arr items, and there is no `respect_cleanuparr_strikes`
  flag. A download Cleanuparr is striking is still in the *arr queue, and SnatchArr already
  skips queued items.
- There is no Seeker detection. When Cleanuparr removes a download and searches again
  itself, the item's [afterglow](../concepts.md#afterglow) keeps SnatchArr's next attempt from
  piling on.

## Consequences

- v1 carries the `cleanuparr_links` table (migration 000005) and the `/cleanuparr` routes
  that ADR-0004 kept out.
- Operators store a Cleanuparr API key, not a username and password. It gets the same
  encryption at rest and write-only API handling as *arr API keys.
- Strike-aware skipping can come later. It needs a strike-to-item mapping that Cleanuparr
  does not expose today.
