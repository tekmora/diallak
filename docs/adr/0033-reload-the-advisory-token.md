# ADR-0033: Reload the GitHub advisory token before each read

- **Status**: Accepted
- **Date**: 2026-11-25
- **Authors**: Maintainers
- **Supersedes in part**: [ADR-0032](0032-every-product-in-release-news.md),
  the token paragraph in Decision

## Context

ADR-0032 requires a token for advisory reads above 10 own repositories. The
fleet decision of 2026-11-23 makes that token a GitHub App installation token
instead of a personal access token. An installation token expires one hour
after it is minted. Before this record the bot read the token file once at
startup, so it kept the first token after the deployment had refreshed the
file.

## Decision

The deployment mints the installation token, writes it to the file named by
`DIALLAK_NEWS_GITHUB_TOKEN_FILE` and refreshes that file at the same path
before the token expires. The deployment never prints the token.

The bot checks the file at startup, as before: a missing, empty or oversized
file stops the start. Before each advisory read to `api.github.com` the bot
reads the file again and sends the token it finds. A rewrite in place and a
replacement of the file both take effect on the next read. Reads to other
hosts never open the file. The file must be a regular file at startup and on
each reread so a FIFO cannot stall the advisory round.

If the file cannot be read or fails the checks at read time, the bot skips
that request and tries again at the next advisory interval of 15 minutes. It
never falls back to an unauthenticated request, and the logged error names the
variable and the failure but not the token.

## Supersedes

- ADR-0032 Decision, second paragraph: the cluster deployment creates the
  read-only token as a secret, and the bot reads it once.

## Consequences

- **Positive**. A token refreshed by the deployment takes effect without a
  restart.
- **Positive**. The token value no longer stays in the configuration for the
  life of the process; the configuration holds only the file path.
- **Negative**. The bot opens the token file once per advisory read. With 20
  own repositories that is 80 small file reads an hour.
- **Negative**. A deployment that fails to refresh the file still sends an
  expired token. The bot does not yet treat GitHub's 401 reply apart from other
  failed reads.

## Alternatives considered

- **Read the file once at startup.** The token expires one hour later.
- **Use a personal access token.** The fleet decision of 2026-11-23 selects a
  GitHub App for this automation.
- **Read advisories with the Discussions GitHub App.** This would give that app
  access beyond discussions, which ADR-0032 rejects.

## Open points

GitHub's 401 reply to an expired token is handled like any other failed read.

## Decided in this record

The bot reads `DIALLAK_NEWS_GITHUB_TOKEN_FILE` before each advisory read to
`api.github.com` and skips the read when the file fails its checks.

## References

- [ADR-0032](0032-every-product-in-release-news.md), every product in release
  news.
- [Configuration and file checks](../../internal/config/config.go),
  `ReadNewsGitHubToken`.
- [Advisory requests](../../internal/news/advisory.go),
  `UseAPITokenSource`, and the wiring in
  [`cmd/diallak/news.go`](../../cmd/diallak/news.go).
- Tests: rotation and file failures in
  [`internal/news/advisory_test.go`](../../internal/news/advisory_test.go),
  the retry in [`internal/news/run_test.go`](../../internal/news/run_test.go),
  startup checks in
  [`internal/config/news_test.go`](../../internal/config/news_test.go).
- [GitHub App installation tokens](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/authenticating-as-a-github-app-installation).
