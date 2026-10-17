# ADR-0019: News posts

- **Status**: Proposed
- **Date**: 2026-10-17
- **Authors**: Maintainers

## Context

The [server definition](../server-definition.md) lists the function `news` as
planned and in need of a record: own releases and security advisories are
posted as they happen, everything else is one digest per channel per day
(`server/definition.json`, function `news`). Issue
[#91](https://github.com/tekmora/diallak/issues/91) holds what is decided so
far:

- News follows the areas. AI news goes to the AI area, video news to the media
  area, own security advisories to the announcement channel. There are no
  extra news channels.
- The bot posts as itself. It holds no right to manage webhooks
  ([ADR-0010](0010-bot-rights-and-apply-path.md), "What the bot does
  not hold").

Three accepted records bound the design:

- [ADR-0001](0001-architecture-and-boundaries.md) gives the bot no inbound
  network path and leaves "Where state is stored" to the first feature that
  stores state. No feature has stored state so far:
  [hosting](../hosting.md) runs the bot with "no database, no volume to write
  to".
- [ADR-0010](0010-bot-rights-and-apply-path.md) gives the announcement channel
  no entry for the bot: "No accepted record names the bot as the one who posts
  there; the news function is planned and brings its own record."
- [ADR-0014](0014-roles-managed-notification-recognition.md) makes the role
  "Notify: releases" mentionable by everyone, so that nobody needs the right to
  mention every role.

This record settles the eight points #91 lists. The units that build the reader
(#92), the conditional requests (#93) and the store of posted items (#94) wait
for it.

## Decision

### 1. The bot polls; nothing comes in

ADR-0001 stands: the bot opens every connection itself.

- **Own releases**: the Atom feed GitHub serves for every repository,
  `https://github.com/<owner>/<repo>/releases.atom`, read every 10 minutes.
  The same reader serves the other feeds (point 2).
- **Own security advisories**: the REST endpoint
  `GET /repos/<owner>/<repo>/security-advisories?state=published`, read every
  15 minutes without a token. The GitHub App of
  [ADR-0004](0004-community-questions.md) keeps its read access to discussions
  only.
- **The budget**: GitHub allows 60 requests an hour without a token (the
  response header `x-ratelimit-limit: 60`, read on 2026-10-17), and a
  `304 Not Modified` answer is free only for a request with an
  `Authorization` header. At four requests an hour per repository the list of
  own repositories holds at most 10 entries. The bot obeys `Retry-After` and
  the `x-ratelimit-*` headers.

### 2. Which feeds, and who adds one

The feeds are data in the server definition, next to the areas:

- `news.repositories`: own repositories, each a public repository of the
  project's organisations, as for the discussions bridge (ADR-0004).
- `news.feeds`: each feed has an id, an `https` address and the area it serves.

A feed is added by a pull request to the definition, which a maintainer
reviews like every other change of the file. The validator refuses an address
that is not `https`, a feed for an area the definition does not hold and more
than 20 feeds.

### 3. An area that is not open

The bot posts a digest into an area channel only when the channel exists on
the server. For an area that is not open, the items of the day are marked as
seen and dropped. Nothing is held back, so an area that opens later starts
with the next day's news, not with a backlog.

### 4. Publishing to following servers

The bot publishes its own release and advisory posts of the announcement
channel to the servers that follow it. Discord asks only `SEND_MESSAGES` for
a message the bot sent itself ("Crosspost Message"). The announcement channel
gets a channel entry for the bot role that allows `VIEW_CHANNEL` and
`SEND_MESSAGES`; it is added to the definition when this record is accepted.
`MANAGE_MESSAGES` on the channel is not needed and not given.

### 5. Times

A post writes every time as `<t:UNIX>` or `<t:UNIX:STYLE>`, which Discord shows
in the reader's own time zone and locale ("Message Formatting"). A release
post uses the default style; a digest names its day with style `D`.

### 6. Mentions

A release or advisory post mentions the role "Notify: releases". The bot
writes the mention, and the message's allowed mentions name that one role and
nothing else, so no text taken from a feed can mention a member, a role or
everyone. A digest mentions nobody.

### 7. Where state is stored

Behind a store interface, as ADR-0001 assumes, in one JSON file on a volume
of its own. A write goes to a new file that replaces the old one in one step,
so a crash leaves the old state or the new one. SQLite stays the next step
when a feature needs queries or a second writer. The deployment gains the
volume when the news code ships.

### 8. What is kept

<!-- markdownlint-disable MD013 -->
| Data | Kept |
| :--- | :--- |
| Per feed: the `ETag` and `Last-Modified` the server sent, the time of the next read | While the feed is listed |
| Per posted or dropped item: a SHA-256 key of feed id and item id, and the time it was first seen | 30 days |
| Items waiting for the day's digest: title and address | Until the digest is posted, at most 24 hours |
<!-- markdownlint-enable MD013 -->

- An item older than 7 days by its own date is never posted, so a key that
  expired after 30 days cannot bring an item back.
- The first read of a new feed marks every item as seen and posts nothing.
- A post carries the item's title and address only; no summary, no author
  name, no image. Feed text is data: markup is removed, Discord formatting is
  escaped, a title is cut at 200 characters and an address must be absolute
  `https`.
- A read has a deadline of 10 seconds and reads at most 1 MiB.
- Nothing about members is read or kept. The privacy notice and the data
  inventory gain a section that says what the bot keeps on disk and why, and
  that news processes no member data; they change with the code, not with
  this record.

### Off by default

The news code is built off: without its setting in the configuration it reads
nothing and posts nothing. The units #92 to #94 build reading and storing
only; the path that posts to Discord follows once this record is accepted.

## Consequences

- **Positive**: No inbound path, no token and no new right beyond one channel
  entry.
- **Positive**: One reader serves own releases and other feeds alike;
  conditional requests keep an unchanged feed cheap.
- **Positive**: Feed text cannot mention anybody, and a post passes on no
  third party's name beyond what a title holds.
- **Negative**: The bot writes to disk for the first time. The deployment
  needs a volume, the root filesystem stays read-only, and the hosting page
  changes.
- **Negative**: Polling without a token caps the own repositories at 10 and
  makes a release appear up to 10 minutes late.
- **Negative**: A day's news for an area that is not open is lost, on
  purpose.

## Alternatives considered

- **A webhook from the forge.** Needs an inbound path, which ADR-0001 rules
  out.
- **A Discord webhook per channel.** Needs `MANAGE_WEBHOOKS`, which ADR-0010
  keeps from the bot.
- **The GitHub App reads advisories.** Saves the request budget but widens the
  app's access beyond discussions; not needed at 10 repositories.
- **SQLite now.** ADR-0001's working assumption. A pure-Go driver adds a large
  dependency tree for a few hundred keys; the store interface keeps the move
  open.
- **Hold news for a closed area.** Floods the area when it opens and keeps
  third-party text for an unknown time.
- **News channels of their own.** Decided against in #91: news follows the
  areas.

## Supersedes

This record changes no accepted statement. It settles "Where state is stored"
of [ADR-0001](0001-architecture-and-boundaries.md) for this feature. It also
gives the announcement channel the bot entry that
[ADR-0010](0010-bot-rights-and-apply-path.md), "Channel entries of the bot's
role", left to this record.

## Open points

- **Not tried against Discord.** No post, crosspost or allowed-mentions value
  has been sent to the server.
- **The Atom feed's limits.** GitHub's documentation reviewed here covers the
  REST API; whether `releases.atom` counts against a limit was not measured.
- **The digest's hour and length.** Proposed: one digest per area channel at
  08:00 UTC with at most 10 items, the newest first; the rest is dropped and
  counted in the log.

## Decided in this record

The issue that asked for this record
([#91](https://github.com/tekmora/diallak/issues/91)) lists the questions.
The work was ordered through the controller. The answers above are proposals
until the record is accepted. These choices were made here:

- Polling intervals of 10 and 15 minutes and the cap of 10 own repositories.
- The cap of 20 feeds.
- Dropping, not holding, the news of an area that is not open.
- The bot crossposts its own posts.
- Advisories mention "Notify: releases" as releases do.
- A JSON file on a volume instead of SQLite.
- The retention of 30 days, the age cut of 7 days and the limits of a read.

## References

- Discord developer documentation, repository `discord/discord-api-docs` at
  commit `f7416a2cf165`, read on 2026-10-17:
  `developers/resources/message.mdx`, "Crosspost Message" ("requires the
  `SEND_MESSAGES` permission, if the current user sent the message") and
  "Allowed Mentions Object"; `developers/reference.mdx`, "Message Formatting"
  and "Timestamp Styles".
- GitHub documentation, repository `github/docs` at commit `dae83f7d28f3`,
  `content/rest/using-the-rest-api/best-practices-for-using-the-rest-api.md`,
  "Use conditional requests".
- RFC 9110, sections 10.2.3 (`Retry-After`), 13.1.2 (`If-None-Match`), 13.1.3
  (`If-Modified-Since`) and 15.4.5 (`304 Not Modified`).
- `server/definition.json`: function `news`, channel `announcements`, role
  `notify-releases`.
