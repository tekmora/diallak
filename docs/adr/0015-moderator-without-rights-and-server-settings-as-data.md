# ADR-0015: A Moderator role without rights, and server settings as data

- **Status**: Accepted
- **Date**: 2026-10-12
- **Authors**: Maintainers

## Context

Two things in the accepted records no longer fit what the operator wants.

- **A moderator had two ways to act.**
  [ADR-0005](0005-moderation-and-escalation.md) gives the Moderator role the
  rights to delete messages and to time members out, and promises a trail of
  what moderators do through the bot.
  [ADR-0010](0010-bot-rights-and-apply-path.md) gives the bot the same two
  rights, to act when a moderator asks. A moderator who holds the rights can
  also act in the app, and that act leaves no line in the trail.
- **Two server settings were set by hand.** The platform's filter rules and
  the server's verification level are part of what makes a set-up safe.
  ADR-0010 leaves them to the operator's hand in the raised run, so they are
  the one part of a set-up that cannot be compared or repeated.

## Decision

### The Moderator role holds no right

The Moderator role keeps its place in the order and its entries on channels,
which let its holders see every area and room and read the queue and the
trail. It holds no server-wide right, and no entry gives it one of the
moderation rights.

- **A moderator acts through the bot only.** Deleting a message and timing a
  member out are commands of the bot. The bot checks that the caller holds
  the Moderator role, carries out the act with its own rights
  (`MANAGE_MESSAGES`, `MODERATE_MEMBERS`, both held since ADR-0010) and
  writes the line of the trail.
- **The line names who acted and why.** It holds the time, the moderator who
  acted, the act, the affected message or member id and the reason the
  moderator gave. A command without a reason does nothing.
- **The trail is the only record.** There is no store of warnings, no note on
  a member and no look-up of a member's history. The operator looks at this
  again after the live test.
- **Until the commands exist, only the operator can moderate.** The commands
  are their own unit, with the moderator guide as its condition (ADR-0005,
  "Written rules come first").

The validator refuses a Moderator role that holds a right, and an entry on a
channel that gives the role a moderation right.

### The bot holds the right to manage the server, for one function

The filter rules and the verification level become data of the
[server definition](../server-definition.md). The tool compares them with the
server and applies a difference like every other change: plan, digest,
confirmation, apply, read-back.

<!-- markdownlint-disable MD013 -->
| Right | The bot needs it to | Discord asks for it in |
| :--- | :--- | :--- |
| `MANAGE_GUILD` | Read, create and change the platform's filter rules | "Auto Moderation", "Permissions": every access to the rules |
| `MANAGE_GUILD` | Set the server's verification level | "Modify Guild" |
<!-- markdownlint-enable MD013 -->

- **The bot's role now holds four rights:** `MANAGE_ROLES`,
  `MANAGE_MESSAGES`, `MODERATE_MEMBERS` and `MANAGE_GUILD`. The validator
  refuses a bot role that lacks one of them or holds another.
- **The right is held for a function that is decided and not built.** The
  definition lists the function as the automation entry `server-settings`
  with the state `decided`. Until it is built, no code of this repository
  uses the right. It lands now so that the operator's server sees one plan
  with one digest for this change and the roles change of
  [ADR-0014](0014-roles-managed-notification-recognition.md), and not a
  second raised run later.
- **The right reaches the bot's role like its other rights.** On a new
  server the address that adds the bot carries it. On a server the bot is
  already on, the operator switches it on at the bot's role; the plan never
  holds a change to the caller's own role.
- **The rules are shown before they are applied.** Which words, which limits
  and which action a rule holds is written as data and put in front of the
  operator before the first apply.
- **The bot still receives no event of the filter.** The filter rule writes
  its alert into the queue channel by itself, as ADR-0010 decided.

### What the right reaches beyond the function

`MANAGE_GUILD` is wider than its two uses. With it, Discord's documentation
lets the holder also change the other fields of the server object, list and
delete integrations, read the invitations, change the welcome screen and the
widget, and pause invitations. The server's two funding integrations are
among what it could delete.

What bounds it:

- The tool calls the filter rules and the one field of the server object,
  and nothing else the right opens. A test holds the list of calls the tool
  may make against the platform. Until the function is built, no call on
  that list needs the right.
- The right does not reach the roles above the bot's own, any channel, a ban
  or a removal of members, or a private thread in the ticket channel.
- The token stays in the workload that runs the bot.

A fault in the bot or a leaked token reaches these settings from the day the
right is given, which is before the function exists. That is the price of one
plan and one digest.

### Members send no polls

The definition gives no member the right to send the platform's own polls.
That is wanted: a vote that counts runs through the wishes and polls path,
which the bot serves as an adapter.

## Consequences

- **Positive**: One way to moderate, and every act on it has a line with a
  name and a reason.
- **Positive**: A set-up can be repeated and compared in full, the filter
  rules and the verification level included, once the function is built.
- **Negative**: Between this record and the moderation unit, nobody but the
  operator can delete a message or time a member out.
- **Negative**: The bot holds a wide right before the function that needs it
  exists.
- **Negative**: The trail gains the moderator's identity. The data inventory
  and the privacy notice name it when the commands are built.
- **Negative**: The numbers of the invitation address and of the bot's
  every-day rights change
  ([Applying the server definition](../server-apply.md)).

## Alternatives considered

- **Leave the rights on the Moderator role and ask moderators to use the
  bot.** A rule nobody can check; the trail would be complete only by good
  will.
- **Give the bot the right to read the audit log** and copy what moderators
  do in the app into the trail. A second right, a stream of events to read,
  and still no reason for an act.
- **Set the filter rules and the verification level by hand and only compare
  them.** Comparing the rules needs the same right as changing them, so the
  bot would hold it and the hand act would stay.
- **Give the right with the function, in a later plan.** The narrower way in
  time. It costs a second raised run on the operator's server. The operator
  can still choose it: the right then leaves this change.
- **Keep settings out of the definition.** The state ADR-0010 chose. A new
  server would then depend on what someone remembers to click.

## Supersedes

This record changes single statements. The records stand as they are.

- [ADR-0005](0005-moderation-and-escalation.md), "Two role templates": the
  Moderator row. A moderator deletes messages and times members out through
  the bot; the role itself holds no permission.
- ADR-0005, "Moderator actions leave a trail the operator can read": the line
  names the moderator who acted and the reason, where the record says "the
  role that acted".
- [ADR-0010](0010-bot-rights-and-apply-path.md), "The bot holds three
  server-wide rights": four, with the two lines above.
- ADR-0010, "What the bot does not hold": the row `MANAGE_GUILD`.
- ADR-0010, "Decided in this record": "No `MANAGE_GUILD` as a standing right:
  the filter's alert goes into the queue channel, and the filter rules are
  set only in the raised run." The alert still goes into the queue channel by
  the rule itself. The rules are data.

## Open points

- **Not tried against Discord.** That a bot holding `MANAGE_GUILD` can create
  each kind of filter rule the data will name, and that the address that
  adds the bot leaves its role with the right.
- **The rule data** is not part of this record. It comes with the function
  and is read by the operator first.
- **What a timed-out or deleted-from member is told,** and how a member
  objects, belongs to the moderator guide.

## Decided in this record

The operator decided: a moderator acts through the bot only, and the
Moderator role holds no right of its own; every act is written to the log
channel with who acted and why; the trail is the only record for now, with no
store of warnings; the filter rules and the verification level become data of
the definition that the bot compares and applies, and the bot's role gains
the right to manage the server for it, named in a record that supersedes
ADR-0010's list; the rule data is shown before the first apply; members keep
no right to send polls.

Derived from those answers and confirmed in the project's planning, open to
the operator's veto:

- The right lands with this change, before its function is built.

The choices below were made here and are accepted with this record:

- The Moderator role keeps its entries on channels: it sees every area and
  room, the queue and the trail.
- A command without a reason does nothing.
- A test holds the list of calls the tool may make against the platform.
- The address that adds the bot to a new server carries the right.

## References

- Operator decisions of 2026-10-12 (paraphrased): moderators act through the
  bot only and the role holds no right; the trail stays the only record until
  after the live test; the bot applies the filter rules and the verification
  level from the definition and gains the right to manage the server through
  a superseding record; members keep no right to send polls.
- Discord developer documentation, repository `discord/discord-api-docs` at
  commit `37148620d232` of 2026-10-09:
  `developers/resources/auto-moderation.mdx` ("Permissions": "Users are
  required to have the `MANAGE_GUILD` permission to access all Auto
  Moderation resources."), `developers/resources/guild.mdx` ("Modify Guild":
  "Requires the `MANAGE_GUILD` permission", field `verification_level`; the
  further endpoints that name the right: integrations, invitations, widget,
  welcome screen, incident actions), `developers/resources/message.mdx`
  ("Delete Message"), `developers/topics/permissions.mdx` (`MANAGE_GUILD`,
  bit 5):
  <https://github.com/discord/discord-api-docs/tree/37148620d2324292b76047541d5f8331f81ea77a>
- Discord Help Center, "Pause Invites FAQ", read 2026-10-12: the switch is
  called "Manage Server", in the German app "Server verwalten":
  <https://support.discord.com/hc/en-us/articles/8458903738647>
