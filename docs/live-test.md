# Live test of the built functions

This page is the protocol for the test that switches on every built function
in the hub and checks it once ([#198](https://github.com/tekmora/diallak/issues/198)).
It is the operator's act and runs after the cutover
([#36](https://github.com/tekmora/diallak/issues/36)) and after the server
definition is applied to the hub
([#197](https://github.com/tekmora/diallak/issues/197)). The first live run,
`/ask` in excerpt mode, has its own issue
([#35](https://github.com/tekmora/diallak/issues/35)).

Each point below names what to do, what counts as a pass, and the record or
code that decides the behaviour. A point passes only when the result matches;
a failure is fixed in a patch release and that point is run again.

## Before the test

- **The image.** The released image the deployment runs, by tag and digest.
- **Settings.** Each function that is off without its settings needs them set
  in the deployment (README, "Run"). Credentials stay in the deployment's
  secret store and never appear in a result.

<!-- markdownlint-disable MD013 -->
| Function | Settings it needs | Off without them |
| :--- | :--- | :--- |
| `/areas`, `/bug`, moderation | none beyond the required three | always on |
| News | `DIALLAK_NEWS_STATE_FILE`; `DIALLAK_NEWS_GITHUB_TOKEN_FILE` optional | yes ([`internal/config`](../internal/config/config.go)) |
| Discussions bridge | `DIALLAK_DISCUSSIONS_APP_CLIENT_ID`, `DIALLAK_DISCUSSIONS_APP_KEY_FILE` and the news state file | yes ([`internal/config`](../internal/config/config.go)) |
| Health listener | `DIALLAK_HEALTH_ADDR` | yes ([hosting](hosting.md), "Health checks") |
<!-- markdownlint-enable MD013 -->

- **Commands.** Run the image once with `-register-commands`. It registers
  `/ask`, `/areas`, `Remove message`, `/timeout` and `/bug` for the server,
  prints `commands registered for guild <id>` and exits 0
  ([`cmd/diallak`](../cmd/diallak/main.go)).
- **Ready.** With the health listener on, `GET /readyz` passes once the
  gateway is open ([hosting](hosting.md), "Health checks").

## What to record

One line per point: the point, pass or fail, the time in UTC, and for a fail
what was seen. A result holds no member id, no member name and no message
text. The moderation trail holds member ids by design
([ADR-0021](adr/0021-moderation-commands-and-trail.md)); the result only
says whether its line has the stated form.

## The points

### 1. `/areas`

Decided in [ADR-0011](adr/0011-area-roles-from-member-choice.md).

- A test member runs `/areas`. The reply is visible to that member only and
  shows the interest, language and notification menus.
- The member takes one interest role, one language role and "Notify:
  releases", then drops all three. Pass: the member holds each role after
  taking it and none after dropping it; the bot keeps no record of the
  choice, so the roles on the server are the whole state.

### 2. `/bug`

Listed in the [product definition](product.md) under ADR-0001.

- A test member runs `/bug` with a title and steps. Pass: the reply is
  visible to that member only and holds a link that opens a new, filled-in
  issue of the repository named in `bug_reports.repository` of
  [`server/definition.json`](../server/definition.json). The test does not
  submit the issue.

### 3. Moderation

Decided in [ADR-0021](adr/0021-moderation-commands-and-trail.md); the
moderator's side is in the [moderator guide](moderator-guide.md).

- **Refusal.** A member without the Moderator role uses `Remove message` on
  a test message. Pass: a reply only that member sees, the message stays, and
  no trail line is written.
- **Remove.** A moderator uses `Remove message` on a test message with a
  reason. Pass: the message is gone and one line of the form
  `<time> · <moderator> · removed message · <affected> · reason: <reason>`
  appears in the log channel, holding no text of the message.
- **Timeout.** A moderator times a test member out for 1 minute with a
  reason. Pass: the member is timed out and one line with
  `timed out until <end in UTC>` appears in the log channel.
- **Limits.** A blank reason and a length above 40320 minutes are refused
  before anything is done.
- **Queue.** Issue #198 names the queue. ADR-0021 writes a moderator's act to
  the trail only; the queue channel receives the platform filter's flags
  ([moderator guide](moderator-guide.md)). The test records which channel
  each act reached.

### 4. News

Decided in [ADR-0019](adr/0019-news-posts.md); the repositories and feeds
are the `news` entry of the definition.

- **Release.** A release of a listed repository appears in the announcement
  channel within the release read interval (10 minutes,
  [`internal/news`](../internal/news/advisory.go)). Pass: the post mentions
  "Notify: releases" and no one else, and it is published to the servers
  that follow the channel.
- **Advisory.** A published security advisory of a listed repository appears
  in the announcement channel within its read interval (15 minutes). It can
  only be tested when one is published; otherwise the result says "not
  tested" and why.
- **Digest.** Everything else arrives as one digest per channel per day at
  08:00 UTC ([`internal/news`](../internal/news/run.go)). Pass: one digest per
  channel that exists, mentioning nobody; an area that is not open gets
  nothing.

### 5. Discussions bridge

Decided in [ADR-0004](adr/0004-community-questions.md).

- A new discussion in the category Q&A or Ideas of a listed repository. Pass:
  within the read interval (15 minutes,
  [`internal/news`](../internal/news/discussions.go)) one post with a link
  back to the discussion.

### 6. Memory and CPU

With every function on, read the container's memory and CPU from the
platform's own metrics at rest and while points 1 to 5 run. The measured
values replace the starting values in [hosting](hosting.md), "Resources",
in the pull request that records the results.

## Done

The test is done when every point has a recorded result and every failure is
fixed in a patch release (#198, "Done when").
