# ADR-0011: The bot gives area roles from the member's own choice

- **Status**: Accepted
- **Date**: 2026-10-11
- **Authors**: Maintainers

## Context

The [server definition](../server-definition.md) has eight roles a member
picks for themselves: four interests and four languages. Each one counts
towards opening its area and shows the area once it is open.

The definition handed these roles out through Discord's onboarding.
[ADR-0010](0010-bot-rights-and-apply-path.md) found that the launch set of
channels is too small for Discord to switch onboarding on, settled the
direction with the operator and left the function to a record of its own:
the platform's onboarding is not used, and the bot gives the roles from the
member's own choice with the right to manage roles it already holds.

This record describes that function, what the bot reads for it, what it
refuses, and what changes in the definition.

## Decision

### One command shows the choice and changes it

A member runs `/areas` on the server. The bot answers at once with a message
only that member sees. It holds one menu for the topic areas and one for the
language rooms. The areas the member already chose are selected.

When the member changes a menu, the bot gives the roles that were added and
takes the roles that were removed, then says in the same message what it did.

- **Nothing is required.** Both menus accept an empty choice, and a member who
  never runs the command loses nothing but the areas.
- **The choice stays open.** The same command changes it at any time. It is
  not tied to the moment of joining.
- **The answer needs no call.** The first message is built from the
  definition and from the roles Discord sends with the command, so it arrives
  inside the three seconds Discord allows for a first response. Giving and
  taking roles takes calls to Discord, so the bot first acknowledges the
  change and edits the message afterwards.

What Discord's documentation says about each step is listed under References.

### The role on the server is the only state

The bot stores nothing for this function. Which areas a member chose is the
set of roles the member holds on the server, and Discord sends that set with
every command.

- **Read from the command, held in memory for that one request:** the member's
  id, to address the two calls; the member's roles, to select what is already
  chosen and to work out the difference; and whether the member has still to
  pass the server's screening.
- **Logged:** the role of the definition, whether it was given or taken, and
  the outcome. No member id, in line with the
  [data inventory](../compliance/data-inventory.md), "What the bot logs".
- **Held by Discord, not by the bot:** the role on the member, and one entry
  in the server's audit log for each change. Discord keeps audit log entries
  for 45 days and shows them to roles that may view the audit log. The bot
  sends a fixed reason with each call, so the entry says the change was the
  member's own choice.

**Other members can see the choice.** A role is part of what Discord shows
about a member on the server. The menu says so before the member chooses. This
matters most for the language roles, which can hint at where someone is from.

The [privacy notice](../compliance/privacy-notice.md) and the data inventory
say today that the bot reads none of the fields Discord sends with a command
besides the question. The pull request that builds this function changes both
texts in the same change, as [ADR-0002](0002-data-and-compliance.md) asks.

### What the bot refuses

Each line is a case the build unit plants in a test and shows refused.

<!-- markdownlint-disable MD013 -->
| Case | What the bot does |
| :--- | :--- |
| The command or the menu comes from another server | Refuses, as it does for `/ask` ([ADR-0001](0001-architecture-and-boundaries.md)) |
| A chosen value is not an option of that menu in the definition | Refuses the whole change and makes no call. The list of options is closed and comes from the definition, never from the request |
| A value appears twice, or there are more values than options | Refuses the whole change |
| The member has not passed the server's screening | Refuses. Discord's documentation says such a member cannot act, so this should not arrive; the bot does not rely on it |
| A call to Discord fails or runs past its deadline | Tells the member which choices were not applied. It does not repeat the call. The next `/areas` shows the true state, because it is read from Discord |
<!-- markdownlint-enable MD013 -->

Two limits hold by construction, not by a check at run time.

- **The member changes only their own roles.** The command takes no argument.
  The member the bot acts on is the one who sent the interaction.
- **Only area roles pass through this path.** The definition's validator
  refuses a menu option that names any other kind of role, and
  [ADR-0010](0010-bot-rights-and-apply-path.md) keeps the area roles below
  the bot's role and the staff roles above it.

One change makes at most one call per option of a menu: four today, never
more than the 25 options a Discord menu holds. Each call carries a deadline.

### The bot finds the roles by name

The definition holds names and no platform id. The bot reads the server's
roles and matches them by name, with the reader the apply path uses for its
snapshot ([ADR-0010](0010-bot-rights-and-apply-path.md), "One apply path").
No role id enters the definition or the configuration.

An option whose role is missing on the server, or whose name exists twice,
is left out of the menu. The apply path's compare step reports both.

### The count of an area is the count of its role

An area opens by the number of members who chose it. That number is the
number of members holding the role, which Discord returns for all roles in one
call ("Get Guild Role Member Counts"). No member is listed, no message is read
and the bot keeps no tally of its own.

The call was made once against the server on 2026-10-11, through the bot's own
identity, and returned the map. Two limits of that reading:

- **It works with the setting the application holds today; it is unproven
  without.** The application has Discord's server members intent switched on.
  Whether the call answers when it is off was not tried, and the
  documentation does not say. The bot's own connection asks for no privileged
  intent either way ([ADR-0001](0001-architecture-and-boundaries.md)).
- **It does not show what a role without members looks like.** The map held
  four roles, each with at least one member. Whether Discord lists a role
  nobody holds with a zero or leaves it out is not documented, and this
  reading cannot tell. The record that builds the opening of areas has to
  decide what a missing role counts as; counting it as zero keeps an area
  closed.

### What changes in the definition

The definition changes in the unit that follows this record, together with
its validator and its page.

<!-- markdownlint-disable MD013 -->
| In the definition today | After this record |
| :--- | :--- |
| Top-level key `onboarding`, done by the platform, with two questions | Top-level key `choices`, done by the bot, with the same two questions and options |
| A question has a field `required` | The field is gone. No choice is required, and the strict loader refuses the old field |
| The eight roles have `assigned_by: onboarding` | `assigned_by: choice`. The value `onboarding` leaves the vocabulary |
| Automation entry `onboarding-questions`, by the platform, planned | Automation entry `area-choices`, by the bot, decided in this record |
<!-- markdownlint-enable MD013 -->

The validator keeps its rules under the new names and gains three:

- Every interest and language role is the option of exactly one question, and
  every option names such a role. A role the bot gives for a fact, such as the
  Supporter roles, can then never be offered in a menu.
- A question holds at most 25 options, and an option title at most 100
  characters. These are the limits of a Discord menu.
- A role assigned by choice stands below the bot's role in `role_order`, as a
  role assigned by onboarding had to.

Renaming a key breaks every reader of the old file. Nothing has applied the
definition yet (`"applied": false`), and the only reader is the validator in
this repository, so the unit changes both in one commit and marks it as a
breaking change.

## Consequences

- **Positive**: The definition can be applied as it stands. Nothing in it
  waits for a platform feature the server does not qualify for.
- **Positive**: There is one source for who chose what. A member who leaves
  the server drops out of the count without any clean-up.
- **Positive**: The function needs no new right, no new intent and no store.
- **Negative**: A new member is not asked. They have to learn that `/areas`
  exists, from the rules channel or from the list of commands.
- **Negative**: The bot now reads the member's id and roles from a command.
  Until now it read neither.
- **Negative**: A choice is public on the server. A member who wants a
  language room without showing it cannot have that.

## Alternatives considered

- **Discord's onboarding.** It asks for seven default channels, five of them
  writable for every member, and for the right to manage the server to set it
  up ("Modify Guild Onboarding"). The launch set has neither
  ([ADR-0010](0010-bot-rights-and-apply-path.md), "Open points"), and the
  operator decided against it.
- **A standing message with buttons in a start channel.** New members would
  find it without being told. The bot would need to post in a channel where
  ADR-0010 gives it no entry. It stays a candidate for when being found turns
  out to be the problem; the command works without it.
- **Reactions on a message.** The bot would have to post a standing message
  and subscribe to the reactions on it. Today it subscribes to no event about
  messages ([ADR-0002](0002-data-and-compliance.md)).
- **A command with one argument per area.** It cannot show what the member
  already chose, and it takes one command per change.
- **Keep the choices in the bot's own store** and bring the roles in step.
  That makes two sources that can disagree, and a store with member ids that
  the function does not need.
- **Discord's Linked Roles.** They tie a role to a fact about an account held
  elsewhere. An interest is not such a fact.

## Supersedes

This record changes single statements. The records and the rest of the
definition stand as they are.

- The server definition: the interest and language roles assigned by
  onboarding, the top-level key `onboarding` and the automation entry
  `onboarding-questions`. They become what the table above lists.
- [ADR-0010](0010-bot-rights-and-apply-path.md), "Open points", "How members
  choose their interests": closed by this record.

## Open points

- **Being found.** Where a new member reads about `/areas` is a text of the
  server, not of the bot. The rules channel is the likely place.
- **The count without the members intent**, and what a role missing from the
  map counts as: see "The count of an area is the count of its role".
- **The window of an opening rule.** The definition opens a topic area at 12
  members within 14 days. A count of role holders says how many hold the role
  now and nothing about when they chose. What the window means is decided in
  the record that builds the opening of areas.
- **Not tried.** Everything about the command, the menus and the two role
  calls is read from Discord's documentation. Not tried against Discord: that
  a role shows on the member for others exactly as described, that a member
  in screening cannot send the command, and that giving a role the member
  already holds succeeds.
- **The legal basis** for reading the member's roles is named in the privacy
  notice by the unit that changes it, and joins that notice's list of points
  for legal review. This record is not legal advice.

## Decided in this record

The operator decided that Discord's onboarding is not used and that the bot
gives the interest and language roles from the member's own choice, through a
menu or a command, with the right to manage roles it already holds, after a
record of its own. The operator also decided that an area opens by the count
of members who chose it and that no message is read for that count.

The choices below were made here and are accepted with this record, open to
the operator's veto:

- One command, `/areas`, answered with a message only the member sees and two
  menus. No standing message in a channel.
- No store: the role on the server is the state.
- The refusals in the table, and no repeated call after a failure.
- Roles found by name through the apply path's reader.
- The definition's key `choices`, the value `assigned_by: choice` and the
  three validator rules.

## References

- Operator decision of 2026-10-11 on how members choose their interests
  (paraphrased): the platform's onboarding is not used; the bot gives the
  interest and language roles from the member's own choice, by a menu or a
  command, with the right to manage roles it already holds, once its own
  decision record exists; the count that opens an area reads these roles.
- Operator decision of 2026-10-11 on how an area opens (paraphrased): a topic
  area opens by the count of members who chose that interest; no message is
  read for it.
- Discord developer documentation, repository `discord/discord-api-docs` at
  commit `37148620d232` of 2026-10-09:
  `developers/interactions/receiving-and-responding.mdx` (the interaction's
  `member` field with the invoking member's data; the callback types "deferred
  update message" and "update message"; a first response within three seconds
  and a token valid for 15 minutes),
  `developers/resources/message.mdx` (the message flag `EPHEMERAL`),
  `developers/components/reference.mdx` (string select: at most 25 options,
  `min_values` and `max_values`, the option's `default`, label and value at
  most 100 characters),
  `developers/resources/guild.mdx` (guild member object with `roles` and
  `pending`; "Add Guild Member Role", "Remove Guild Member Role"; "Get Guild
  Role Member Counts"; "Modify Guild Onboarding"; "Membership Screening
  Object"),
  `developers/topics/permissions.mdx` ("Permission Hierarchy"),
  `developers/resources/audit-log.mdx` (entries kept for 45 days, the
  `X-Audit-Log-Reason` header, the entry type for a member's role change),
  `developers/resources/application.mdx` (application flags for the server
  members intent):
  <https://github.com/discord/discord-api-docs/tree/37148620d2324292b76047541d5f8331f81ea77a>
- The reading of the role counts and of the application's flags on 2026-10-11
  is kept outside this repository.
