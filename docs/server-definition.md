# Server definition

[`server/definition.json`](../server/definition.json) describes the community
server as data: its roles, channels and permissions, the rooms that open later
and who does what. A validator checks the file with every test run.

**The file has not been applied to a server.** It says so itself (`"applied":
false`), and the validator refuses a file that claims otherwise. The path that
applies it is decided
([ADR-0010](adr/0010-bot-rights-and-apply-path.md), "One apply path") and
built: reading the server, comparing it with the file, planning the changes
and applying one plan. None of it was run against Discord
([Applying the server definition](server-apply.md)).

## In short

- **From the first day:** the rules, announcements, one lounge, the support
  forum, a ticket channel, a voice room for scheduled events, and two channels
  for the moderators.
- **Later, at a threshold:** four topic areas (home lab and self-hosting, media
  and video, games, AI news and tools) and social rooms for languages. Each
  opens when enough members asked for it and closes when it stays empty. A
  language room also needs a moderator or subject guide who reads the
  language; there is none at the start.
- **Who sees what** is computed from the file, not written by hand. The
  validator refuses, among others, a role that could mention everyone and a
  role that could read other members' ticket threads without being a
  moderator.
- **Thresholds are proposals.** Each one names what is measured, over which
  window and why, and none is measured yet.
- **Three roles belong to another party:** the role Discord gives to members
  who boost the server, and the roles of two funding integrations. The file
  lists them so that a run can check that they are there; the tool never
  creates, changes or deletes one
  ([ADR-0014](adr/0014-roles-managed-notification-recognition.md)).
- **How a role is shown is data where the file states it:** whether its
  members form a group of their own in the member list (`shown_apart`) and
  whether members can mention it (`mentionable`). A field the file does not
  state is not compared and not changed.
- **The bot holds four rights, each for a decided function:** managing
  roles, deleting messages, timing members out and managing the server. The
  order of roles keeps the first of them away from the staff roles
  ([ADR-0010](adr/0010-bot-rights-and-apply-path.md)). The fourth is held for
  applying the server's settings
  ([ADR-0015](adr/0015-moderator-without-rights-and-server-settings-as-data.md)).
- **The Moderator role holds no right.** A moderator deletes a message or
  times a member out through a command of the bot, and every such act lands
  in the moderators' log channel (ADR-0015,
  [ADR-0021](adr/0021-moderation-commands-and-trail.md)).
- **The platform's filter rules and the verification level are data.**
  The file proposes three filter rules and a verification level for the
  operator to read before anything is applied
  ([Filter rules and verification level](#filter-rules-and-verification-level)).
  The validator checks them against the platform's limits; plan and apply
  compare and set them ([Applying the server definition](server-apply.md)),
  not yet tried against Discord.
- **Members send no polls.** No role and no entry on a channel gives the
  right `SEND_POLLS`; this is wanted, not an omission (ADR-0015,
  `TestDefinition_Boundary_NoSendPolls`).

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
| `roles` | Every role with its kind, who assigns it, its server-wide permissions and, where stated, how it is shown | 32 |
| `role_order` | The roles from the highest to the lowest, without the role every member holds and the roles another party manages | 28 |
| `categories` | The groups channels are shown in | 6 |
| `channels` | Every channel with type, purpose, owner and the permissions it changes per role | 16 |
| `areas` | The rooms that open at a threshold, each with an opening and a closing rule | 8 |
| `choices` | The questions the bot's areas command offers and the role each option gives | 3 questions |
| `automation` | Who does what: the bot, the platform or the operator, and whether it is built | 17 |
| `settings` | The platform's filter rules and the server's verification level; absent, nothing of it is compared or changed | 3 filter rules and the verification level |
| `news` | Own repositories, whose releases and advisories go to the announcement channel, the other feeds, each with the area it serves, and the text channel of the Discussions bridge; read by the bot, not applied to the server | 2 repositories and 0 feeds |
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
| `area-channel` | An area without a channel, an area channel that every member sees, or one its own role or the bot does not see |
| `area-condition` | A language room without the condition that someone who reads the language watches it |
| `first-answer` | A support forum without exactly one opt-in tag (`opt_in`) that every member can set, or in which the bot cannot read and reply |
| `staff-visibility` | A moderators' channel that a role other than operator, moderator or bot can see |
| `trail` | More than one trail channel, the channel that holds the lines of the moderation commands; a trail channel that is not a moderators' channel; a trail channel whose history the bot cannot read, since the bot lists its own lines there to delete them |
| `moderator-visibility` | A channel that a moderator cannot see |
| `voice-closed` | A voice channel that members can join between events or use without push-to-talk |
| `operator-only`, `moderation-only` | Administration rights outside the operator role; moderation rights outside the operator role; any right on the Moderator role itself. The bot's role passes for its four rights only |
| `bot-rights` | A bot role that lacks one of its four rights or holds another; an entry of the bot's role that allows more than its kind of channel needs; a moderators' channel the bot cannot write in; a ticket channel the bot can see |
| `role-order` | A role missing from the order or listed twice; a managed role in the order; a role the operator assigns that does not stand above the bot's role; a role the bot assigns, by itself or from a member's choice, that does not stand below it |
| `nothing-attaches` | A permission on a recognition, interest, language, notification or managed role; a channel entry that allows something to a recognition or notification role |
| `role-grant` | A role assigned by the wrong party, such as a member's choice handing out the moderator role, or a managed role not assigned by the platform or an integration; an interest, language or notification role that is not the option of exactly one question; an option that gives a role of another kind; a notification role that does not state `mentionable: true`; `mentionable` on a managed role; `shown_apart` or `mentionable` on the bot's role or on the role every member holds |
| `choice-limit` | A question with more than 25 options, or an option title longer than 100 characters |
| `filter-rule` | More rules of one trigger than a server may hold; a field the rule's trigger or the action's type does not read; a keyword rule without keywords or patterns, a preset rule without presets; a list or an entry over the platform's limit; a mention limit outside 1 to 50; a rule without an action or with one action twice; an alert without a channel or into a channel that is not a staff channel; a timeout on a trigger other than keyword or mention spam, or outside one second to four weeks; a block message over 150 characters; more than 20 exempt roles or 50 exempt channels, or one listed twice; two rules with one name |
| `news` | A feed address that is not an absolute `https` address; a feed without an area or for an area the file does not hold; more than 20 feeds; more than 10 own repositories, one not written `owner/name` or listed twice; own repositories without exactly one announcement channel, or with one where the bot cannot see or send; a `discussions_channel` that is no channel of the file or not of type `text` |
| `bug-reports` | A bug reports repository that is not written `owner/name` |
| `automation-record` | Something the bot or the platform does without a decision record, a planned bot feature not marked as needing one, or the state `observed` on anything but the platform or together with a record |
| `id`, `text`, `enum`, `ref`, `version`, `singleton`, `permission-name`, `access-shape` | Malformed ids, empty texts, unknown values, references to nothing or to a managed role from a channel entry or an area, a wrong version, a missing or doubled base role, unknown or doubled permissions |
<!-- markdownlint-enable MD013 -->

Every condition of every rule has a test that plants exactly that defect
(`review_test.go` and `review_shape_test.go` in
[`internal/serverdef`](../internal/serverdef)).

## What the bot may do

[ADR-0010](adr/0010-bot-rights-and-apply-path.md) ties the first three rights
to a function each and to the line of Discord's documentation that asks for
it. [ADR-0015](adr/0015-moderator-without-rights-and-server-settings-as-data.md)
does the same for the fourth.

<!-- markdownlint-disable MD013 -->
| Right of the bot's role | Label in the role's settings | For |
| :--- | :--- | :--- |
| `MANAGE_ROLES` | Manage Roles | Giving and taking the Supporter roles; giving and taking the area roles a member chooses with `/areas` ([ADR-0011](adr/0011-area-roles-from-member-choice.md)); opening and closing an area by the entry of its role on its channel |
| `MANAGE_MESSAGES` | Manage Messages | Deleting a message when a moderator acts through the bot |
| `MODERATE_MEMBERS` | Time Out Members | Timing a member out when a moderator acts through the bot |
| `MANAGE_GUILD` | Manage Server | Comparing and applying the platform's filter rules and the server's verification level as data of the definition. The data, its validator, compare and apply are built; the tool calls the filter rules and the one change of the server that sets the level, and nothing else the right opens ([Applying the server definition](server-apply.md)) |
<!-- markdownlint-enable MD013 -->

The labels are the English ones of Discord's settings; the German ones, and
where both were read, are in
[Applying the server definition](server-apply.md#the-rights-by-their-name-in-the-settings).

`role_order` puts every role the operator gives above the bot's role: the
Operator, the Moderator, Maintainer, Helper, the three subject guide roles,
Translator, Tester, Creator, Partner and Honorary supporter. Every role the bot
gives stands below it. Discord lets a bot give and edit only roles below its own, so the bot
cannot hand out a staff role. Nothing attaches to the roles below it.

A new role the bot gives is best added at the end of `role_order`: Discord
creates it below every role on the server, so it stands right without an
order step. A new role between two others needs a place by hand; the plan
names it
([The order of roles](server-apply.md#the-order-of-roles)).

The three managed roles stand outside the order. A run checks that each is on
the server and managed, and that the two funding roles stand below the
Moderator role; the operator moves one that does not.

"Ko-fi supporter" and "Patreon supporter" are given by the integrations, not by
the bot, and stand at the end of `role_order`, below both integrations' roles,
which must stand above the roles they give. The bot only reads who holds them
([ADR-0024](adr/0024-supporter-roles-from-funding-platforms.md)).

In channels the bot's role has an entry only where a function needs one: the
support forum, the two moderators' channels, the eight area channels and the
announcement channel. On the announcement channel it may see and send, which
posting and crossposting its own release and advisory posts need
([ADR-0019](adr/0019-news-posts.md), point 4). It is denied the ticket
channel.

## Thresholds

<!-- markdownlint-disable MD013 -->
| Rule | Measured | Proposed value | Window |
| :--- | :--- | :--- | :--- |
| A topic area opens | Members who hold the area's interest role | At least 12 | On every daily reading for 14 days |
| A language room opens | Members who hold the language role, and a person who reads the language has agreed to watch the room | At least 5 | On every daily reading for 14 days |
| An area or room closes | Days since the last message in its channel | At least 30 | 30 days |
<!-- markdownlint-enable MD013 -->

- **The measure is decided, the numbers are not measured.** The operator
  decided that a topic area opens by the count of members who chose its
  interest when joining. The values are first guesses; the reasoning stands
  next to each rule in the file. They are corrected from measured joins once
  the server has members, in the file and not in code. The window is read as
  standing demand: the count has to hold on every daily reading, so a short
  burst does not open a room and a slowly growing server still can.
- **No message is read.** Both measurements come from counts and times the
  platform reports. That the platform reports them as the file assumes is read
  from Discord's documentation and not tried (`"verified": false`).
- **Closing hides a room.** It does not delete it, so that opening it again
  brings its history back.

## Filter rules and verification level

`settings` holds what [ADR-0015](adr/0015-moderator-without-rights-and-server-settings-as-data.md)
makes data: the platform's own filter rules and the server's verification
level. Every value in it is a proposal (`"basis": "proposal"`), chosen and not
measured; the reasoning stands next to each in the file. The operator reads
them before the first apply.

<!-- markdownlint-disable MD013 -->
| Rule | Trigger | Catches | Actions |
| :--- | :--- | :--- | :--- |
| `spam` | `spam` | What the platform recognises as spam | Block the message; alert in `mod-queue` |
| `mention-spam` | `mention_spam` | A message with more than 10 different mentions, and a raid of such messages | Block the message; alert in `mod-queue` |
| `abuse-presets` | `keyword_preset` | Words from the platform's lists of slurs and of sexual content | Block the message; alert in `mod-queue` |
<!-- markdownlint-enable MD013 -->

- **Verification level `medium`:** an account has to be older than five
  minutes before it can write. Membership Screening already holds a new member
  back until they completed the form.
- **No rule times a member out** and none blocks their interactions. ADR-0005
  leaves spam and obvious abuse to the platform; every further step is a
  moderator's, through the bot.
- **The swearing list is left out.** Swearing alone is not abuse on this
  server.
- **The alert goes into the moderators' queue by the rule itself** (ADR-0005).
  The validator refuses an alert into a channel that is not a staff channel,
  since the alert carries the flagged message.

The limits the validator holds are Discord's: rules per trigger and server,
list lengths and entry lengths, the mention limit, the timeout length, the
block message and the exemptions, read in
`developers/resources/auto-moderation.mdx` of `discord/discord-api-docs` at
commit `37148620d232`. The five verification levels are read in
`developers/resources/guild.mdx` of the same commit. None of it was tried
against Discord. The names the tool reads and writes for each number are in
[`server_read_settings.go`](../internal/discord/server_read_settings.go).

## News

The part `news` says where the news function posts
([ADR-0019](adr/0019-news-posts.md), points 2 and 3):

- Own releases and security advisories of the repositories in
  `news.repositories` go to the channel of type `announcement`.
- Every other feed in `news.feeds` names the area it serves, and its daily
  digest goes to that area's channel. News follows the areas: AI news to
  `ai-news-tools`, video news to `media-video`. A topic without an area, such
  as framework news today, has no feed; the validator refuses a feed for an
  area the file does not hold.
- When the area's channel is not open on the server, the day's items of its
  feeds are marked as seen and dropped, so an area that opens later starts
  with the next day's news (`HoldForPlace` in
  [`internal/news`](../internal/news)).

A feed or a repository is added by a pull request to this file. The own
repositories are `tekmora/diallak` and `tekmora/epitrop`; the list of feeds
stays empty until the operator names the sources for AI and video news.

## Who does what

The `automation` part names the party for each task and its state: built,
decided, planned, or observed on the server.

- **The bot, built:** the `/ask` command; the `/bug` command, which answers
  with a prefilled new-issue link of the repository in `bug_reports` for the
  member to submit ([ADR-0001](adr/0001-architecture-and-boundaries.md)); the `/areas` command, which gives
  and takes the interest, language and notification roles from the member's
  own choice ([ADR-0011](adr/0011-area-roles-from-member-choice.md)); and
  what a moderator does through the bot, `Remove message` and `/timeout`,
  together with the trail of it
  ([ADR-0021](adr/0021-moderation-commands-and-trail.md)); the first answer
  in the support forum, off until its two settings are set
  ([support-forum-answer.md](compliance/support-forum-answer.md)); and the
  server's settings as data: the platform's filter rules and the
  verification level, compared and applied through plan and apply
  ([ADR-0015](adr/0015-moderator-without-rights-and-server-settings-as-data.md)).
  `TestDefinition_Negative_BotAutomationStateMatchesCode` in
  [definition_test.go](../server/definition_test.go) names the code file of
  each and fails when a state and the code disagree.
- **The bot, built and off until its app is set:** the bridge from GitHub
  Discussions, which posts each new Q&A or Ideas discussion into
  `news.discussions_channel`, the lounge
  ([ADR-0004](adr/0004-community-questions.md), issue #99), and the news
  posts: own releases and advisories as they happen, every other feed as one
  digest per channel per day; off until `DIALLAK_NEWS_STATE_FILE` is set
  ([ADR-0019](adr/0019-news-posts.md)).
- **The bot, planned:** opening and closing the areas. It needs its own
  decision record first; until then the operator opens and closes areas by
  hand.
- **The platform:** the spam filter, which writes each flagged case into the
  moderators' queue channel by itself, and the Contributor role, a linked role
  that Discord gives on the value the cluster's identity provider writes
  ([ADR-0026](adr/0026-member-to-forge-account-link.md)).
- **The operator:** inviting people and granting the moderator, the subject
  guide and the other recognition roles above the bot's role, reading the
  ticket threads, scheduling events and opening the voice room for them.
- **Observed on the server, decided nowhere:** Discord's Membership
  Screening, which holds a new member back from every action until they
  completed the server's form. It was switched on by hand in the server's
  settings. The definition records that it is on (`"state": "observed"`) so
  that the file describes the server as it is; no decision record stands
  behind it, and the validator accepts the state for the platform only.
- **Applying the definition** is one path with four steps: plan, dry run,
  apply, read-only compare. Every run uses the bot's identity. For the first
  set-up the operator raises the bot's role for the run and lowers it
  afterwards; with its four rights the bot runs the same path for its own
  changes. It is decided in ADR-0010. All four steps are built and tested
  against a local server only
  ([Applying the server definition](server-apply.md)).

## Not decided and not verified

- **Language rooms.** The operator decided that a language room opens at a
  threshold and needs a person who reads the language, and that none exists at
  the start ([server rules](compliance/server-rules.md), rule 2). The numbers
  of the threshold are proposals. That a member asks for a language through
  the bot's `/areas` command is decided in
  [ADR-0011](adr/0011-area-roles-from-member-choice.md).
- **The contributor ladder** is decided: one role, Contributor, after one
  merged contribution, and the subject guide roles above it. There is no third
  step. It is a linked role: the member links a forge account through the
  cluster's identity provider, which writes whether a contribution was merged,
  and Discord gives the role. The bot takes no part and keeps no link
  ([ADR-0026](adr/0026-member-to-forge-account-link.md)).
- **Supporter roles** are decided: one role for supporters and one cosmetic
  role after a year of support. Nothing attaches to either, and there is no
  channel for supporters only (ADR-0014). The bot gives "Supporter" to a
  member who holds "Ko-fi supporter" or "Patreon supporter", the hidden roles
  each platform's integration gives, and takes it after a grace period once
  the member holds neither. No member data is stored. "Supporter for a year"
  has no source yet
  ([ADR-0024](adr/0024-supporter-roles-from-funding-platforms.md)).
- **Honorary supporter** is decided: a role the operator gives by hand, for
  good and tied to no funding platform. It stands above the bot's role, so the
  bot can neither give nor take it, and no file holds who has it. Whatever is
  attached to the supporter roles is attached to it; a test holds that
  ([ADR-0016](adr/0016-honorary-supporter-and-third-subject-guide.md)).
- **Boosting** shows the member in a group of their own in the member list
  and gates nothing. The operator sets that display by hand; a run reports
  while it is off and holds no step for it (ADR-0014).
- **The boost level of the server.** The definition sets no banner, no role
  icon, no audio bitrate and no vanity address; Discord's documentation ties
  each of these to a server feature or a boost level. By that documentation
  the announcement channel needs the server feature `NEWS`. Both were read
  from the platform on 2026-10-11: the feature list holds `NEWS` and
  `COMMUNITY`, and the level was 1 (ADR-0010, "What was read from the
  server"). The plan step reads them again on every run and refuses a change
  the server lacks the feature for; no list of perks is taken from memory.
- **How members choose their interests.** Discord's documentation asks for at
  least seven default channels, five of them writable for every member, before
  its onboarding can be switched on. The launch set has six channels every
  member sees and three in which every member can write. The platform's
  onboarding is therefore not used: the bot gives the interest and language
  roles from the member's own choice, made through its `/areas` command
  ([ADR-0011](adr/0011-area-roles-from-member-choice.md)). The two
  notification roles are a third question of the same command (ADR-0014). The
  file holds the three questions under `choices`, and the ten roles carry
  `"assigned_by": "choice"`.
- **Discord's onboarding** and the two measurements are described as its
  documentation gives them
  ([`discord/discord-api-docs`](https://github.com/discord/discord-api-docs/tree/37148620d2324292b76047541d5f8331f81ea77a)
  at commit `37148620d232`: `developers/resources/guild.mdx`,
  `developers/topics/permissions.mdx`). Nothing was tried against Discord.
- **Names of channels and roles** are the file's suggestion. The code names no
  channel; names reach the bot through configuration
  ([ADR-0005](adr/0005-moderation-and-escalation.md)).
