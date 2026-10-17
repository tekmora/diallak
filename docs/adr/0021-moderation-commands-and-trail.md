# ADR-0021: The moderation commands and their trail

- **Status**: Proposed
- **Date**: 2026-10-17
- **Authors**: Maintainers
- **Amends**: [ADR-0005](0005-moderation-and-escalation.md), "Moderator
  actions leave a trail the operator can read";
  [ADR-0015](0015-moderator-without-rights-and-server-settings-as-data.md),
  "The Moderator role holds no right"

## Context

[ADR-0015](0015-moderator-without-rights-and-server-settings-as-data.md)
decided that a moderator acts through the bot only: the Moderator role holds
no right, and the bot deletes a message or times a member out with its own
rights when a moderator asks. Every act writes one line of the trail into the
log channel, with the time, the moderator, the act, the affected id and the
reason. A command without a reason does nothing.

It left open how a moderator calls the bot, what the line looks like, how the
trail is kept for its 90 days
([data inventory](../compliance/data-inventory.md)) and what the member is
told. The [moderator guide](../moderator-guide.md)
([issue 88](https://github.com/tekmora/diallak/issues/88)) needs the names of
the commands, and the commands
([issue 89](https://github.com/tekmora/diallak/issues/89)) need the rest.

## Decision

### Two commands

<!-- markdownlint-disable MD013 -->
| Command | Kind | Asks for | Discord call |
| :--- | :--- | :--- | :--- |
| `Remove message` | Message command, in the menu of a message | A reason, in a form | Delete Message |
| `/timeout` | Slash command | The member, the length in minutes, a reason | Modify Guild Member, `communication_disabled_until` |
<!-- markdownlint-enable MD013 -->

- **Both are server commands.** They do not work in direct messages.
- **Every member sees them; the bot checks the role.** The Moderator role
  holds no right ([ADR-0015](0015-moderator-without-rights-and-server-settings-as-data.md)),
  so Discord's own permission on a command cannot tell a moderator from a
  member. The bot checks that the caller holds the Moderator role and refuses
  everyone else with a reply only the caller sees. Nothing else happens on a
  refusal, and no line is written.
- **The reason is required, 1 to 512 characters.** Discord shows a reason of up
  to 512 characters in the audit log, and the bot passes it there too. A blank
  reason is refused before anything is done.
- **A timeout lasts 1 minute to 28 days.** The option takes minutes, from 1 to
  40320. Discord accepts no timeout further than 28 days ahead; the bot
  refuses a longer one before it calls Discord.
- **What Discord refuses is said.** Discord refuses to time out the server's
  owner and a member who holds Administrator. The bot tells the moderator the
  act did not happen and writes no line.
- **One act per call.** No command deletes several messages or acts on several
  members.

### The line of the trail

The bot writes one line into the log channel after the act succeeded:

```text
<time> · <moderator> · <act> · <affected> · reason: <reason>
```

- `<time>` is the time of the act in UTC, to the second.
- `<moderator>` is the mention of the moderator who acted.
- `<act>` is `removed message` or `timed out until <end in UTC>`.
- `<affected>` is, for a removed message, the member id of its author, the
  channel and the message id; for a timeout, the member id.
- The line holds no text of the removed message and no other data about the
  member. A line mentions no one, so nobody is notified by it.

### The trail is kept 90 days

The bot deletes its own lines in the log channel once they are older than the
retention, 90 days unless configuration sets another length. It looks once a
day.

- To find its lines, the bot needs `READ_MESSAGE_HISTORY` in the log channel:
  without it, Discord returns no messages. The server definition gives the bot
  only `VIEW_CHANNEL` and `SEND_MESSAGES` there today. The right is added to
  the bot's entry on the log channel with the commands, as a change of the
  definition that the operator applies like every other.
- The bot deletes its own messages only. Deleting a message the bot sent needs
  no further right.
- Discord deletes messages older than 14 days only one at a time, so the bot
  does not bulk-delete.

### The member hears nothing from the bot

The bot sends the member no message about an act. Discord shows a timeout to
the member while it lasts. The [server rules](../compliance/server-rules.md)
tell a member how to object, and the moderator guide tells moderators when to
say something in the channel.

## Consequences

- **Positive**: A moderator has one way to act, and the trail has one line for
  every act, with a name and a reason.
- **Positive**: Lines leave the log channel after 90 days without anyone's
  hand.
- **Negative**: Every member sees two commands that only moderators can use.
- **Negative**: The bot gains a right in the log channel, and the operator
  applies one more change to the server.
- **Negative**: A member learns of a removed message only by missing it.

## Alternatives considered

- **Hide the commands with Discord's command permission and grant them to the
  Moderator role in the server's integration settings.** Hand work in the
  app that the definition cannot compare; the bot would check the role anyway.
- **A slash command that deletes a message by its id or link.** Moderators
  would copy ids by hand; the message menu needs none.
- **Keep the ids of the trail lines in a store of the bot.** A second place
  that holds trail data, where ADR-0015 keeps the trail the only record.
- **Tell the member by direct message.** It fails when the member closed
  direct messages and would put a text about the member in a place the trail
  does not cover.

## Open points

- **Not tried against Discord.** That a message command opens a form, and that
  a bot with `READ_MESSAGE_HISTORY` lists its own lines in the log channel.
- **The data inventory and the privacy notice** name the trail, with the
  moderator's id, when the commands are built.

## References

- [ADR-0005](0005-moderation-and-escalation.md),
  [ADR-0015](0015-moderator-without-rights-and-server-settings-as-data.md).
- Discord developer documentation, repository `discord/discord-api-docs` at
  commit `f7416a2cf165` of 2026-10-16: `developers/resources/guild.mdx`
  ("Modify Guild Member", `communication_disabled_until`: up to 28 days in the
  future, 403 for a member with Administrator or the owner, needs
  `MODERATE_MEMBERS`), `developers/resources/message.mdx` ("Delete Message";
  "Get Channel Messages": no messages without `READ_MESSAGE_HISTORY`; "Bulk
  Delete Messages": nothing older than 2 weeks),
  `developers/resources/audit-log.mdx` (`X-Audit-Log-Reason`, 1 to 512
  characters), `developers/interactions/application-commands.mdx` (message
  commands, `default_member_permissions`):
  <https://github.com/discord/discord-api-docs/tree/f7416a2cf165455f72a31f5ea4e671a4f047f2f7>
