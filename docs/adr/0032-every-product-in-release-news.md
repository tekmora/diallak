# ADR-0032: Every product in release news

- **Status**: Accepted
- **Date**: 2026-11-23
- **Authors**: Maintainers
- **Supersedes in part**:
  [ADR-0019](0019-news-posts.md), point 1 and its consequence for the repository cap

## Context

ADR-0019 point 1 capped the list of own repositories at 10 to stay within
GitHub's unauthenticated REST rate limit. Ten more product repositories
publish releases, so the definition now lists 20. A GraphQL read on 2026-11-23
at 08:59 CET confirmed all 20 were public and unarchived.

## Decision

Under the fleet decision of 2026-11-23, every product repository joins release
news. Each release gets its own announcement with the `Notify: releases` role
mention, with no daily fold. The definition can list up to 50 own repositories.
With news enabled, the bot starts without a token for up to 10 repositories and
stops at startup above 10 if the token is absent.

The cluster deployment creates the read-only GitHub token as an app-to-app
secret, mounts it as a file named by `DIALLAK_NEWS_GITHUB_TOKEN_FILE`, and
never prints it. The bot sends the token only to `api.github.com` for advisory
reads and never logs it.

### Measurement

On 2026-11-23 at 08:57:58 CET, unauthenticated `GET /rate_limit` on
`api.github.com` showed 55 requests remaining in the REST core budget. Three
reads of `https://github.com/tekmora/diallak/releases.atom` each returned
HTTP 200 with 30,376 bytes and no `x-ratelimit-*` header. The next REST rate
limit read still showed 55 remaining.

An unauthenticated `GET /repos/tekmora/diallak/security-advisories?state=published`
on `api.github.com` showed `x-ratelimit-limit: 60` and 54 remaining. Repeating
it with `If-None-Match` returned HTTP 304 and 53 remaining. The unauthenticated
304 cost one request.

At the 15-minute interval, each repository needs four advisory reads an hour.
The 20 product repositories need 80, exceeding the unauthenticated limit
of 60 per address. Ten repositories use 40 reads and leave 20 for retries
and other clients sharing the address.

GitHub documents a limit of 5,000 requests an hour for the read-only token.
An authenticated HTTP 304 does not count against the primary limit, as recorded
in ADR-0019 point 1 from documentation read on 2026-10-17. The 2026-11-23
measurement covered unauthenticated requests only. The chosen cap of 50
repositories schedules 200 advisory reads an hour out of the 5,000 available
with authentication.

## Supersedes

- ADR-0019 point 1: “At four requests an hour per repository the list of
  own repositories holds at most 10 entries.”
- ADR-0019 consequence: “Polling without a token caps the own repositories at 10”.

The measurement answers ADR-0019's open point “The Atom feed's limits” for
the REST core budget. Atom release reads left that count unchanged. Other
limits on `github.com` remain unmeasured.

## Consequences

- **Positive**. Every product's releases are announced.
- **Negative**. Posts and role mentions grow with release volume. In the seven
  days to 2026-11-23, `plurano/goenvoy` created 49 releases and
  `plurano/sveltesentio` created 12, both through per-module tags. Each release
  gets a separate post with the role mention.
- **Negative**. A deployment with news enabled for more than 10 own
  repositories needs the read-only token. The committed definition lists 20.

## Alternatives considered

- **Keep the cap of 10.** This leaves products out of release news.
- **Fold releases into the daily digest.** The fleet decision of 2026-11-23
  gives each release its own post.
- **Read advisories every 30 minutes.** Twenty repositories would use 40
  reads an hour, leaving the same 20-read reserve and delaying advisories.
  At the chosen 15-minute interval, the full unauthenticated budget serves
  at most 15 repositories before allowing for retries and other clients.
- **Read advisories with the Discussions GitHub App.** This would give the
  app access beyond discussions, which ADR-0019 keeps separate.

## Open points

Other rate limits for Atom reads from `github.com` remain unmeasured.

## Decided in this record

Every product repository joins without folding release posts. The definition
holds up to 50 own repositories and startup requires the read-only token
above 10 when news is enabled.

## References

- [ADR-0019](0019-news-posts.md), news posts.
- [GitHub REST API rate limits](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api)
  and [conditional requests](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api#use-conditional-requests).
- [Server definition](../../server/definition.json) and
  [announcement posting](../../internal/news/announce.go).
