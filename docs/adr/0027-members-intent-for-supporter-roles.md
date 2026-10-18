# ADR-0027: The members intent for the supporter function

- **Status**: Accepted
- **Date**: 2026-10-18
- **Authors**: Maintainers
- **Amends**: [ADR-0001](0001-architecture-and-boundaries.md), "The bot
  requests no privileged intent"

## Context

[ADR-0024](0024-supporter-roles-from-funding-platforms.md) has the bot give
"Supporter" to a member who holds "Ko-fi supporter" or "Patreon supporter",
and take it away once the member has held neither for a grace period. No
member data is stored, and the grace state is held in memory only.

To do that, the bot has to learn who holds the two platform roles and when a
member gains or loses one. Discord's API documentation, read on 2026-10-18 at
commit `f7416a2c` of the documentation repository (Sources), says:

- **Member events need the members intent.** The events for a member who
  joins, changes (roles included) or leaves the server, `GUILD_MEMBER_ADD`,
  `GUILD_MEMBER_UPDATE` and `GUILD_MEMBER_REMOVE`, come only with the
  privileged `GUILD_MEMBERS` intent. There is no event for a role change
  without it.
- **Listing members needs it too.** The "List Guild Members" endpoint and a
  gateway request for the whole member list need the same intent.
- **What needs no intent gives no list.** "Get Guild Member" returns one
  member whose id the caller already knows. "Get Guild Role Member Counts"
  returns counts only. An interaction carries the roles of the member who
  sent it, and of nobody else.
- **A privileged intent is a switch in the developer portal** for an
  application in fewer than 100 servers; only a verified application must be
  approved for it. The bot serves one server, and the application already has
  the members intent switched on
  ([ADR-0011](0011-area-roles-from-member-choice.md)).

[ADR-0001](0001-architecture-and-boundaries.md) keeps the bot without a
privileged intent and asks that a feature that needs one adds its own record,
because the intent changes what member data the bot receives
([ADR-0002](0002-data-and-compliance.md)). This is that record. ADR-0001 is
accepted, so its text stays as it is and this record amends it
([records](README.md)).

## Decision

### The supporter function requests the members intent, and nothing else does

The gateway session requests `GUILD_MEMBERS` only while the supporter
function is switched on. Presences and message content are never requested.
With the function off, the session requests what it requested before: no
intent, or the one standard intent of the forum answer
([ADR-0009](0009-support-forum-first-answer.md)).

`gatewayIntents` in [`internal/discord/disgo.go`](../../internal/discord/disgo.go)
takes the function's switch; tests hold that the members intent comes only
with it and that presences and message content never come.

### What the bot does with member events

- **On a member event** the bot reads the member's id and roles, and nothing
  else of the event. A member who gains a platform role gets "Supporter". A
  member who holds neither platform role and holds "Supporter" starts a grace
  timer in memory; regaining a platform role clears it.
- **After a restart** the bot lists the server's members once, page by page,
  and does the same for each. Grace timers lost with the restart start again;
  no role is taken earlier than the grace period allows.
- **Nothing is stored.** Member events and the list are held in memory while
  they are handled. Grace timers live in memory until they run out or are
  cleared. No file, log line or trail line holds a member id for this
  function.

### Privacy texts

The [data inventory](../compliance/data-inventory.md), the
[record of processing](../compliance/record-of-processing.md) and the
[privacy notice](../compliance/privacy-notice.md) name the member events and
the member list as data the bot receives when the function is on, and that it
keeps none of it.

## Consequences

- **Positive**: ADR-0024 holds as accepted: removal after a grace period, no
  stored member data, no platform credentials.
- **Positive**: The intent is tied to one function. A deployment without it
  receives no member event.
- **Negative**: With the function on, every join, leave and change of every
  member reaches the bot, including names and nicknames it does not read.
- **Negative**: A restart lists every member once.

## Alternatives considered

- **No intent; give "Supporter" when a member sends a command.** The
  interaction carries the member's roles, but a member who never sends one
  again would keep "Supporter" after support ends, against ADR-0024.
- **No intent; store the ids of members who hold "Supporter" and check each
  one on a timer.** "Get Guild Member" needs no intent, but the stored ids
  contradict ADR-0024's "no member data stored".

## Sources

- Discord API documentation, repository `discord/discord-api-docs`, commit
  `f7416a2c` (2026-10-16), read 2026-10-18:
  `developers/events/gateway.mdx` (gateway intents, privileged intents),
  `developers/events/gateway-events.mdx` (guild member events, request guild
  members), `developers/resources/guild.mdx` (List Guild Members, Get Guild
  Member, Get Guild Role Member Counts) and
  `developers/interactions/receiving-and-responding.mdx` (the interaction's
  member).
- [ADR-0011](0011-area-roles-from-member-choice.md): the application's members
  intent setting, read 2026-10-11.
- Diallak repository issue #196:
  <https://github.com/tekmora/diallak/issues/196>.
