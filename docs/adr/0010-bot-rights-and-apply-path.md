# ADR-0010: Bot rights from decided functions, and one apply path

- **Status**: Accepted
- **Date**: 2026-10-11
- **Authors**: Maintainers

## Context

The [server definition](../server-definition.md) gave the bot's role no
server-wide right, and its validator refused any. That was written while the
bot only answered commands.

Since then the bot was given work that Discord lets no identity do without a
right: giving a role to a member, changing who sees a channel, deleting a
message for a moderator, timing a member out. A definition in which the bot
holds nothing and a list of functions in which it does these things cannot both
be applied.

The operator was asked how the definition reaches the server and answered with
a correction: a bot without rights contradicts the functions already decided
for it. The bot holds the smallest set of rights those functions need and
nothing beyond, and one path applies the definition, for the operator's first
set-up and for the bot's own changes alike.

This record derives the set from the functions, adds the role order that
bounds it, describes the apply path and names what it supersedes.

## Decision

### The bot holds three server-wide rights

Each right is tied to a function that is decided, and to the place where
Discord's documentation asks for it.

<!-- markdownlint-disable MD013 -->
| Right | The bot needs it to | Decided in | Discord asks for it in |
| :--- | :--- | :--- | :--- |
| `MANAGE_ROLES` | Give and take the Supporter roles and the Contributor role | Operator decisions on the supporter roles, the contributor ladder and the bot's functions (References) | "Add Guild Member Role", "Remove Guild Member Role" |
| `MANAGE_ROLES` | Open and close a threshold area: set or remove the entry of the area's role on the area's channel | Operator decisions on how an area opens and on the bot's functions (References) | "Edit Channel Permissions", "Delete Channel Permission" |
| `MANAGE_MESSAGES` | Delete a message when a moderator acts through the bot | [ADR-0005](0005-moderation-and-escalation.md), "Two role templates" and "Moderator actions leave a trail the operator can read" | "Delete Message" |
| `MODERATE_MEMBERS` | Time a member out when a moderator acts through the bot | ADR-0005, the same two sections | "Modify Guild Member", field `communication_disabled_until` |
<!-- markdownlint-enable MD013 -->

The bot's role holds exactly these three. The validator refuses a bot role
that lacks one of them or holds another (finding code `bot-rights`), so the set
can only change together with a record that names the function.

A right is not a design. Where the definition marks a function as needing its
own record (`needs_record`), that record still comes before the function is
built: how the bot learns of a merged contribution or of a supporter, and how
an area opens and closes.

### The role order bounds the right to manage roles

`MANAGE_ROLES` is the strongest of the three. Discord bounds it by the order of
roles ("Permission Hierarchy"): a bot gives members only roles below its own
highest role, and it edits only such roles.

The definition therefore gains the order of roles as data (`role_order`, from
the highest to the lowest):

1. Operator
2. Moderator
3. The subject guide roles
4. The bot's role
5. Contributor, Supporter, Supporter for a year
6. The interest roles and the language roles

The validator holds the order to two rules (finding code `role-order`):

- A role the operator assigns stands above the bot's role. The bot can then
  give nobody the Moderator role or a subject guide role, and it cannot change
  those roles. That part of ADR-0005 now holds by the platform's own rule and
  no longer by the bot's code alone.
- A role the bot or onboarding assigns stands below the bot's role. Otherwise
  the bot could not give it.

Every role except the one every member holds is listed exactly once; the
platform keeps that role at the bottom.

Nothing attaches to the roles below the bot's role (finding code
`nothing-attaches`). A member who is given one of them gains no right.

### Channel entries of the bot's role

<!-- markdownlint-disable MD013 -->
| Channel | Entry of the bot's role | For | In the definition |
| :--- | :--- | :--- | :--- |
| Support forum | Allow `VIEW_CHANNEL`, `READ_MESSAGE_HISTORY`, `SEND_MESSAGES_IN_THREADS` | The first answer ([ADR-0009](0009-support-forum-first-answer.md)) | Unchanged |
| Moderators' queue and log | Allow `VIEW_CHANNEL`, `SEND_MESSAGES` | Queue and trail (ADR-0005) | Unchanged |
| Ticket channel | Deny `VIEW_CHANNEL` | The bot reads no ticket | Unchanged |
| The eight area channels | Allow `VIEW_CHANNEL` | Read the time of the last message for the closing rule, and change the entry of the area's role | New |
<!-- markdownlint-enable MD013 -->

The area channels are hidden from every member until they open, and the bot
is a member. A role that is denied `VIEW_CHANNEL` on a channel loses its other
rights there ("Implicit Permissions"), the right to change the channel's
entries among them. So the bot needs its own entry on each area channel.

The validator bounds the entries by the kind of channel (`bot-rights`): the
bot's entry allows at most what the table lists for the support forum, a
moderators' channel or an area channel, and nothing in any other channel. It
also refuses a moderators' channel in which the bot cannot write, an area
channel the bot cannot see (`area-channel`), and a ticket channel the bot can
see.

The announcement channel gets no entry. No accepted record names the bot as
the one who posts there; the news function is planned and brings its own
record.

### What the bot does not hold

<!-- markdownlint-disable MD013 -->
| Right | Why not |
| :--- | :--- |
| `ADMINISTRATOR` | No function needs it, and it would make every other line of this record meaningless |
| `MANAGE_GUILD` | It covers every server setting and the filter rules themselves. Both uses the bot would have for it have another way: see the next two sections |
| `MANAGE_CHANNELS` | The area channels are created hidden at the first set-up. The bot changes entries on them and creates no channel |
| `KICK_MEMBERS`, `BAN_MEMBERS` | ADR-0005: a moderator may not ban, and no function of the bot removes a member |
| `MANAGE_THREADS` | It would let the bot read every private thread in the ticket channel (finding code `ticket-thread-reader`) |
| `VIEW_AUDIT_LOG` | The trail records what was done through the bot; the bot knows that itself |
| `MENTION_EVERYONE`, `MANAGE_WEBHOOKS` | No function needs them |
<!-- markdownlint-enable MD013 -->

The validator keeps refusing all of these for the bot's role: its rules on
administration and moderation rights now let the bot's role pass for exactly
the three rights above and for nothing else.

### The filter's alert reaches the moderators without the bot

ADR-0005 left open what the platform's rules can do: "What the platform's
rules can do is verified against Discord's current documentation before the
build. The result is recorded here." This is that result.

- Discord sends the events of its filter only to a bot that holds
  `MANAGE_GUILD` (gateway events, "Auto Moderation").
- A filter rule can write the case into a channel by itself (action
  `SEND_ALERT_MESSAGE`, with the channel in the action's metadata).

The filter rule therefore writes its alert into the moderators' queue channel.
The bot does not receive the filter's events and does not copy them. Its part
in moderation is what a moderator does through it, and the trail of that.

`MANAGE_GUILD` can be added by a later record if the moderation unit shows that
the alert in the queue is not enough.

### One apply path

One implementation brings the definition to the server. It has four steps:

1. **Plan.** Read the server, compare it with the definition and list the
   changes in order. Each change names the right it needs. A change the caller
   does not hold the right for is refused here, before anything is tried.
2. **Dry run.** Show the plan and change nothing.
3. **Apply.** Make the changes in the order of the plan, each call with a
   deadline, and stop at the first one that fails.
4. **Compare.** Read the server again, without changing it, and report every
   difference from the definition.

Who runs it decides what the plan can hold:

- **The operator runs the path for the first set-up** and for everything the
  bot does not hold the right for: creating roles and channels, the
  permissions of roles, the order of roles, the filter rules and the
  onboarding questions.
- **The bot runs the same path for its own changes:** a role given to or taken
  from a member, and the entry of an area's role on an area's channel. A plan
  that holds anything else is refused for the bot.

The path creates and changes. It deletes nothing: what the server has and the
definition does not is reported by the compare step and left alone.

The compare step tells "missing" apart from "not visible to the reader". From
2026-11-16 Discord's list of channels leaves out every channel the reading
identity cannot see ("Get Guild Channels"). The bot does not see the ticket
channel, by design.

A credential reaches the process the way the bot's token does today: as the
path of a file that only the process reads (`DIALLAK_TOKEN_FILE`,
[README](../../README.md)). It is never a command-line argument, never in the
repository and never logged.

Nothing in this record is built. The path is a later unit; until it exists,
nothing applies the definition.

### What was read from the server

The server's boost level and feature list were read from the platform on
2026-10-11 through the bot's own identity; the reading is kept outside this
repository.

- The feature list holds `COMMUNITY` and `NEWS`. Discord's documentation names
  `NEWS` as what allows announcement channels, so the announcement channel of
  the definition can be created.
- The boost level was 1. The definition needs nothing that depends on a boost
  level, so it still applies when the level drops.

A reading goes stale. The plan step reads both again on every run and refuses
a change the server lacks the feature for.

## Consequences

- **Positive**: The definition and the list of functions agree. Every right of
  the bot has a function and a documented reason, and a change of the set
  fails the test run until a record names why. The platform itself stops the
  bot from giving out a staff role.
- **Positive**: One path means the first set-up and the bot's later changes
  cannot drift apart, and every run ends with a compare that anyone can
  repeat.
- **Negative**: `MANAGE_ROLES` is a strong right. Below its own role the bot
  could, by the platform's rules, also edit a role and add one of its own
  three rights to it. Only the bot's code and the compare step stand against
  that; the role order does not.
- **Negative**: `MANAGE_ROLES` and `MANAGE_MESSAGES` are among the rights for
  which Discord requires two-factor sign-in on the account that owns the
  application, on a server that has two-factor sign-in switched on
  server-wide (footnote to the table of permission flags).
- **Negative**: The bot now sees the eight area channels. It still subscribes
  to no message events there and reads no message
  ([ADR-0002](0002-data-and-compliance.md)).
- **Negative**: A filter alert in the queue is the platform's text in the
  platform's form. The bot cannot add to it without `MANAGE_GUILD`.

## Alternatives considered

- **Leave the bot without rights and do its work by hand.** This is the state
  the operator corrected: it contradicts the functions already decided.
- **Give the bot `ADMINISTRATOR`.** The shortest definition. One fault in the
  bot or one leaked token would then reach the whole server.
- **Give the bot `MANAGE_GUILD`** to receive the filter's events and to change
  the onboarding questions. The right is far wider than both uses, and each
  has another way.
- **Two apply paths**, a script for the first set-up and code in the bot for
  later changes. Two implementations of one behaviour drift apart, and only
  one of them would be tested with every run.
- **Keep the role order out of the definition** and set it by hand. The order
  is what keeps `MANAGE_ROLES` away from the staff roles; left to memory it
  would be the one unchecked part of the set.

## Supersedes

This record changes single statements. The records and the rest of the
definition stand as they are.

- The server definition as merged on 2026-10-11: the bot's role without any
  server-wide right, and the validator rules that refused one. The role now
  holds the three rights above.
- The server definition: the Contributor role assigned by the operator. The
  bot assigns it.
- [ADR-0005](0005-moderation-and-escalation.md), "The bot assigns no role".
  The section stands for what it names: inviting a person, the Moderator role
  and the subject guide roles are the operator's own acts. Its heading no
  longer holds for the Supporter roles and the Contributor role, which the bot
  gives and takes.
- ADR-0005: "It handles only what those rules flag: it puts the case in front
  of the moderators and records what is done." The platform's rule puts the
  flagged case in front of the moderators. The bot records what moderators do
  through it.
- ADR-0005, consequence: "Roles have to be kept in step with this record by
  hand, since the bot does not manage them." This stands for the staff roles.
  The bot keeps the Supporter roles and the Contributor role in step.

## Open points

- **The link between a member and a forge account.** The bot can give the
  Contributor role only when it knows which member a merged contribution
  belongs to. That is member data and gets its own record before the function
  is built.
- **Onboarding at launch.** Discord's documentation asks for at least seven
  default channels, five of them writable for every member, before onboarding
  can be switched on ("Modify Guild Onboarding"). The launch set of the
  definition has six channels every member sees and three in which every
  member can write. Whether members choose their interests through the
  platform's onboarding or another way is not decided; the definition keeps
  the two questions as they are until it is.
- **Whose credential the operator's run uses.** The path is the same for both
  callers; which identity the operator runs it with is not decided.
- **Not tried.** Everything in this record is read from Discord's
  documentation. Not tried against Discord: that a timeout obeys the role
  order (the documentation names kick, ban and nickname changes), that
  changing a role's entry on a channel obeys it, and that the filter's alert
  holds what a moderator needs to decide.

## Decided in this record

The operator decided that the bot holds the smallest set of rights its decided
functions need and nothing beyond, that the statement giving the bot no rights
is superseded by a new record while the old one stays, that the validator
enforces exactly the derived set, and that one path with plan, dry run, apply
and read-only compare serves the operator's first set-up and the bot's own
changes.

Derived from that answer and confirmed in the project's planning, open to the
operator's veto:

- The three rights, the channel entries and the role order as listed.
- No `MANAGE_GUILD`: the filter's alert goes into the queue channel, and the
  onboarding questions are set only through the operator's run.
- No entry on the announcement channel until a record names the bot as its
  poster.

The choices below were made here and are accepted with this record:

- The role order is data in the definition, checked by two rules.
- The validator bounds the bot's channel entries by the kind of channel.
- The apply path deletes nothing.
- A change outside the caller's rights is refused at the plan step.
- The compare step reports "not visible to the reader" apart from "missing".

## References

- Operator decision of 2026-10-11 on how the definition is applied
  (paraphrased): the bot holds the smallest set of rights its decided
  functions need, among them moderation, keeping the supporter and contributor
  roles in step, opening areas at their threshold and onboarding; one path
  applies the definition, run by the operator for the first set-up and by the
  bot for its own changes.
- Operator decisions of 2026-10-11 on the server definition (paraphrased): an
  area opens by the count of members who chose its interest; the contributor
  ladder is one Contributor role below the subject guide roles; supporters get
  one role and a second after a year.
- Discord developer documentation, repository `discord/discord-api-docs` at
  commit `37148620d232` of 2026-10-09: `developers/topics/permissions.mdx`
  (the table of flags and its footnotes, "Permission Hierarchy", "Implicit
  Permissions"),
  `developers/resources/guild.mdx` ("Add Guild Member Role", "Remove Guild
  Member Role", "Modify Guild Member", "Get Guild Channels", "Modify Guild
  Onboarding", guild features), `developers/resources/channel.mdx` ("Edit
  Channel Permissions", "Delete Channel Permission"),
  `developers/resources/message.mdx` ("Delete Message"),
  `developers/resources/auto-moderation.mdx` (action types, action metadata,
  "Auto Moderation Permission Requirements"),
  `developers/events/gateway-events.mdx` ("Auto Moderation"):
  <https://github.com/discord/discord-api-docs/tree/37148620d2324292b76047541d5f8331f81ea77a>
