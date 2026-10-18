# ADR-0025: The bot as adapter for wishes and polls

- **Status**: Proposed
- **Date**: 2026-10-18
- **Authors**: Maintainers

## Context

Wishes and polls are not built in this repository. They belong to a shared
core that the projects plan to use from every channel, and the bot becomes its
Discord adapter ([README](../../README.md), "Wishes and polls (after 1.0)";
[product](../product.md), "After 1.0"; issues #100 and #101). The core has no
record yet, so this record can only settle the adapter's side. Issue #100
names three points: what the adapter shows and sends, which identifier of a
member leaves Discord, and what happens while the core cannot be reached.

What is already decided and is not reopened here:

- **The core decides, counts and closes a vote; the bot shows it and passes a
  member's choice on** (#100, README).
- **Members send no polls of the platform's own.** The definition gives no
  member the right `SEND_POLLS`, so a vote that counts runs through this path
  ([ADR-0015](0015-moderator-without-rights-and-server-settings-as-data.md),
  "Members send no polls"; [server definition](../server-definition.md);
  `TestDefinition_Boundary_NoSendPolls`).
- **The bot opens every connection itself** and has no inbound network path
  ([ADR-0001](0001-architecture-and-boundaries.md), "A gateway connection,
  least privilege").
- **A member is identified by the user id of the interaction**, and a stored
  record holds only the identifiers, timestamps and state a feature needs
  ([ADR-0002](0002-data-and-compliance.md)).

## Decision

### 1. What the adapter shows and sends

- A member acts through commands and message components, which arrive as
  interactions and need no privileged gateway intent, as `/areas` does today
  ([ADR-0011](0011-area-roles-from-member-choice.md)).
- The adapter sends the core what a member chose and the member's identifier
  (point 2), and nothing else from Discord: no name, no role, no message text.
- The adapter shows what the core returns: the question, the options, and the
  state and result once the core reports them. It counts nothing itself and
  keeps no tally.

### 2. Which identifier leaves Discord

<!-- markdownlint-disable MD013 -->
| Option | How it works | For | Against |
| :--- | :--- | :--- | :--- |
| A. The Discord user id | The adapter sends the user id of the interaction. | The core can answer a deletion request that names that id; nothing to keep in the bot. | A Discord identifier is held by a second system, which the privacy notice and data inventory must name. |
| B. A keyed pseudonym | The adapter sends a keyed hash of the user id; the key stays with the bot. | The core never holds a Discord identifier. | A deletion request has to pass through the bot; losing or rotating the key splits a member's votes. |
<!-- markdownlint-enable MD013 -->

**Option B is chosen** (operator decision through the controller, decision id
diallak-adr-0025-identifier, 2026-10-18). The core never holds a Discord
identifier:

- **The pseudonym** is an HMAC-SHA-256 of the user id under a key that only the
  bot holds, sent with the key's version as `<version>:<hash>`. The version
  names the key, never its value.
- **A deletion request** is one bot command. The bot computes the member's
  pseudonym under every key it still holds and asks the core to delete each.
  Nothing else in the bot needs to be looked up, since the bot keeps no
  record of who voted.
- **A key rotation keeps a member's votes joinable.** The bot adds the new key
  and keeps the old one, for lookups only. While both are held, the adapter
  sends a member's choice under the new pseudonym, together with the old
  pseudonym once, so the core can join the two. A deletion request covers both
  keys. The old key is retired only once no open vote holds a pseudonym under
  it. A member who never returns stays under the old pseudonym and still
  counts as one member.
- **A lost key** cannot be recovered by design. Votes under it stay countable,
  but a deletion request for them can no longer be matched to a member. The
  proposal is to hold the key as a cluster secret with a backup, decided with
  the adapter's deployment.

The privacy notice, the data inventory and the record of processing gain the
entry in the pull request that builds the adapter, not before (ADR-0002).

### 3. While the core cannot be reached

The adapter answers the member, visible to that member only, that the vote
could not be passed on, and keeps nothing. It holds no queue of choices to
send later, so the bot stores no vote and gains no state of its own. Every call
to the core carries a deadline (ADR-0001).

## Consequences

- **Positive**: The bot stays without a count, a store of votes and an inbound
  path; one core serves every channel the projects use.
- **Positive**: No privileged intent and no new right for members.
- **Negative**: Nothing ships before the core exists, and a vote cannot be
  taken while the core is down.
- **Negative**: A member's choice leaves Discord, so the privacy texts change
  with the code.

## Alternatives considered

- **Count in the bot.** Ruled out by #100: the core decides and counts, so
  every channel sees one result.
- **The platform's own polls, sent by the bot.** The definition keeps members
  from sending polls; a poll the bot sends would be counted by Discord, not by
  the core. Not taken up.
- **A queue of choices while the core is down.** Gives the bot stored state and
  a retention period for a short outage. Not taken up.

## Open points

- **The core's interface.** Address, authentication, the shape of a choice and
  of a result, and how the core tells the adapter that a vote closed. Settled
  by the core's own record.
- **How a vote is opened in Discord**: who may open one, in which channels,
  and whether a command or the core starts it.

## Decided in this record

Point 2: option B, the keyed pseudonym (decision id
diallak-adr-0025-identifier, 2026-10-18). The proposals in points 1 and 3
were written here, on an order through the controller, and stay proposals
until the record is accepted.

Acceptance waits for the operator and for the core's record.

## Sources

- Diallak repository issues #100 and #101:
  <https://github.com/tekmora/diallak/issues/100>,
  <https://github.com/tekmora/diallak/issues/101>.
- Discord developer documentation, `developers/topics/permissions.mdx`
  (`SEND_POLLS`), repository `discord/discord-api-docs` at commit
  `37148620d232`, read 2026-10-12 (cited in #100).
- `server/definition.json` and `TestDefinition_Boundary_NoSendPolls`.
