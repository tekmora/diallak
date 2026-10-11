# Server definition

[`server/definition.json`](../server/definition.json) describes the community
server as data: its roles, channels and permissions, the rooms that open later
and who does what. A validator checks the file with every test run.

**Nothing applies the file to a server.** It says so itself (`"applied":
false`), and the validator refuses a file that claims otherwise. Applying it is
a later unit together with the operator.

## In short

- **From the first day:** the rules, announcements, one lounge, the support
  forum, a ticket channel, a voice room for scheduled events, and two channels
  for the moderators.
- **Later, at a threshold:** four topic areas (home lab and self-hosting, media
  and video, games, AI news and tools) and one social room per language. Each
  opens when enough members asked for it and closes when it stays empty.
- **Who sees what** is computed from the file, not written by hand. The
  validator refuses, among others, a role that could mention everyone and a
  role that could read other members' ticket threads without being a
  moderator.
- **Thresholds are proposals.** Each one names what is measured, over which
  window and why, and none is measured yet.

## Check the file

```bash
go test ./internal/serverdef/ ./cmd/diallak/ -run 'ServerDefinition|Validate|Load|Effective'
```

`TestServerDefinition_Positive_CommittedDefinition` in
[`cmd/diallak`](../cmd/diallak/server_definition_test.go) loads the committed
file, expects no finding and expects every record the file names to exist.
`make verify-all` runs it with the other tests.

The loader is strict: an unknown key, a key that appears twice, a second value
after the definition and a file over one mebibyte are refused
([`internal/serverdef`](../internal/serverdef/serverdef.go), `Load`).

## What the file holds

<!-- markdownlint-disable MD013 -->
| Part | Content | In the file today |
| :--- | :--- | :--- |
| `metrics` | What a threshold rule may measure, and from which data | 2 |
| `roles` | Every role with its kind, who assigns it and its server-wide permissions | 19 |
| `categories` | The groups channels are shown in | 6 |
| `channels` | Every channel with type, purpose, owner and the permissions it changes per role | 16 |
| `areas` | The rooms that open at a threshold, each with an opening and a closing rule | 8 |
| `onboarding` | The questions a new member is asked and the role each answer gives | 2 questions |
| `automation` | Who does what: the bot, the platform or the operator, and whether it is built | 13 |
<!-- markdownlint-enable MD013 -->

A role or a rule carries `"basis": "decided"` or `"basis": "proposal"`. Decided
means the operator decided it; a proposal is this repository's suggestion and
can change without a new decision record.

## How permissions are read

A channel lists only what it changes: per role, the permissions it allows and
denies. The validator computes what a member who holds one role can do in a
channel (`Effective` in
[`internal/serverdef`](../internal/serverdef/effective.go)), in the order
Discord documents:

1. the permissions of the role every member holds, plus those of the role;
2. the channel's entry for every member, deny before allow;
3. the channel's entry for the role, deny before allow.

An administrator can do everything, and a channel a role cannot see leaves it
nothing.

A member can hold several roles, and one of them can make a channel visible
that another cannot see. The rules that refuse a right therefore read what a
role is granted in a channel whether or not it sees the channel (`granted` in
the same file). A member with several roles can then do at most what the roles
are granted one by one.

## What the validator refuses

Each finding has a code, a path into the file and a message.

<!-- markdownlint-disable MD013 -->
| Code | Refused |
| :--- | :--- |
| `channel-owner` | A channel without an owner, or with an owner that is not the operator or a moderator |
| `mention-everyone` | The right to mention everyone for any role but the operator, as a role permission, in a channel entry or in effect |
| `ticket-thread-reader` | A role that is neither operator nor moderator and can read all private threads in the ticket channel |
| `ticket-shape` | A ticket channel in which a member cannot open a private thread or write in it, or can write in the channel itself |
| `area-opening-rule`, `area-closing-rule` | A threshold area without its opening or its closing rule |
| `rule-shape` | A rule with a value below 1, a window outside 1 to 365 days, or a subject that does not exist |
| `area-channel` | An area without a channel, an area channel that every member sees, or one its own role does not see |
| `area-condition` | A language room without the condition that someone who reads the language watches it |
| `first-answer` | A support forum without exactly one opt-out tag that every member can set, or in which the bot cannot read and reply |
| `staff-visibility` | A moderators' channel that a role other than operator, moderator or bot can see |
| `moderator-visibility` | A channel that a moderator cannot see |
| `voice-closed` | A voice channel that members can join between events or use without push-to-talk |
| `operator-only`, `moderation-only` | Administration rights outside the operator role; moderation rights outside the operator and moderator roles |
| `nothing-attaches` | A permission on a recognition, interest or language role |
| `role-grant` | A role assigned by the wrong party, such as onboarding handing out the moderator role |
| `automation-record` | Something the bot or the platform does without a decision record, or a planned bot feature not marked as needing one |
| `id`, `text`, `enum`, `ref`, `version`, `singleton`, `permission-name`, `access-shape` | Malformed ids, empty texts, unknown values, references to nothing, a wrong version, a missing or doubled base role, unknown or doubled permissions |
<!-- markdownlint-enable MD013 -->

Every condition of every rule has a test that plants exactly that defect
(`review_test.go` and `review_shape_test.go` in
[`internal/serverdef`](../internal/serverdef)).

## Thresholds

<!-- markdownlint-disable MD013 -->
| Rule | Measured | Proposed value | Window |
| :--- | :--- | :--- | :--- |
| A topic area opens | Members who hold the area's interest role | At least 12 | On every daily reading for 14 days |
| A language room opens | Members who hold the language role, and a person who reads the language has agreed to watch the room | At least 5 | On every daily reading for 14 days |
| An area or room closes | Days since the last message in its channel | At least 30 | 30 days |
<!-- markdownlint-enable MD013 -->

- **Not measured.** The values are first guesses; the reasoning stands next to
  each rule in the file. They are replaced by measurements once the server has
  members.
- **No message is read.** Both measurements come from counts and times the
  platform reports. That the platform reports them as the file assumes is read
  from Discord's documentation and not tried (`"verified": false`).
- **Closing hides a room.** It does not delete it, so that opening it again
  brings its history back.

## Who does what

The `automation` part names the party for each task and whether it is built.

- **The bot, built:** the `/ask` command.
- **The bot, decided and not built:** the first answer in the support forum,
  the bridge from GitHub Discussions, the moderators' queue and trail.
- **The bot, planned:** opening and closing the areas, the news posts and the
  supporter roles. Each needs its own decision record first; until then the
  operator opens and closes areas by hand.
- **The platform:** the spam filter and the onboarding questions.
- **The operator:** inviting people and granting the moderator, subject guide
  and contributor roles, reading the ticket threads, scheduling events and
  opening the voice room for them, and applying this definition.

## Not decided and not verified

- **Language rooms** open at a threshold here. The server rules say that each
  language has a social room; whether the first rooms exist from the first day
  is the operator's decision.
- **The contributor ladder** has one step in the file, the contributor role.
  Further steps are not decided.
- **The steps of the supporter roles** (3, 6 and 12 months) are a proposal.
- **Onboarding** and the two measurements are described as Discord's
  documentation gives them
  ([`discord/discord-api-docs`](https://github.com/discord/discord-api-docs/tree/37148620d2324292b76047541d5f8331f81ea77a)
  at commit `37148620d232`: `developers/resources/guild.mdx`,
  `developers/topics/permissions.mdx`). Nothing was tried against Discord.
- **Names of channels and roles** are the file's suggestion. The code names no
  channel; names reach the bot through configuration
  ([ADR-0005](adr/0005-moderation-and-escalation.md)).
