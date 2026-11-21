# Channel plan

This page is the channel and category plan of the replan of 2026-11-19, step 2
([product](product.md)). It compares the hub as it was read with
[`server/definition.json`](../server/definition.json) and names every channel
it keeps, adds, moves or retires.

**Nothing here is applied.** The plan becomes a change of the definition in
its own pull request; that change goes through the one apply path of
[ADR-0010](adr/0010-bot-rights-and-apply-path.md) only after it was judged.
The path creates, moves and sets access, and it deletes nothing
([Applying the server definition](server-apply.md), "What a plan holds").

## In short

- **The hub already has the shape of the definition.** All six categories and
  15 of its 16 channels are there; the sixteenth, `tickets`, is hidden from the
  bot by design. One channel is there that the definition does not hold:
  `general`.
- **The decided shape stays.** A product gets a tag in the support forum, not
  a channel. Topic areas and language rooms open at a threshold
  ([Server definition](server-definition.md), "In short").
- **Four changes.**
  1. One chat room: the server's system messages go to `lounge`, and
     `general` moves to a new category, Archive.
  2. The support forum is where rule-based answers appear (replan step 3).
     Each automated answer offers the `tickets` channel.
  3. Every channel gets its topic from the purpose the definition gives it;
     today no channel has a topic.
  4. Archive holds what is retired. `general` there is read-only, visible to
     moderators and to the bot with view only, and never deleted by this plan.
- **Order:** create, then move, then archive. A deletion needs its own
  decision and is not part of this plan.

## The read

The hub was read on 2026-11-19 at 15:56 UTC through the bot that is connected
to it, with three read requests (server, roles, channels) and no write. The
read kept names and permission entries only, no id and no member data; it is
kept outside this repository (SHA-256 `dbbd0043e506`).

A read through Discord's channel list returns only the channels the reading
bot can view. A channel the bot cannot see is therefore "not returned", not
"absent".

<!-- markdownlint-disable MD013 -->
| Category | Channel | Type | In the definition | Read |
| :--- | :--- | :--- | :--- | :--- |
| (none) | `general` | text | no | returned; the server's system channel |
| Start | `rules` | text | yes | returned; members cannot write |
| Start | `announcements` | announcement | yes | returned; members cannot write |
| Community | `support` | forum | yes | returned |
| Community | `lounge` | text | yes | returned |
| Community | `tickets` | text | yes | not returned; the definition denies the bot the view |
| Areas | `home-lab`, `media-video`, `games`, `ai-news-tools` | text | yes | returned; each visible to its interest role and moderators |
| Languages | `espanol`, `francais`, `deutsch`, `italiano` | text | yes | returned; each visible to its language role and moderators |
| Events | `Events` | voice | yes | returned; members cannot connect |
| Staff | `mod-queue`, `mod-log` | text | yes | returned; moderators only |
| ? | `moderator-only` | ? | no | not returned |
| ? | `cauda-alerts` | ? | no | not returned |
<!-- markdownlint-enable MD013 -->

No channel carries a topic. The forum tags were not part of the read.

### Differences besides `general`

The read shows three more differences from the definition. A plan run will list
them next to the steps below, and each is judged with them:

- **`tickets` is hidden.** Compare reports `channel-not-visible` for
  `channels[tickets]` with the detail `hidden from the bot by the definition`.
  This is expected because the definition denies the bot the view. The plan
  holds no step for it: only the detail `missing or hidden` leads to a create
  step. Accept the line only when the detail is exactly
  `hidden from the bot by the definition`. A `missing or hidden` detail for
  `tickets` is outside the lists and stops the unit.
- **`announcements` lacks the bot's entry.** The definition lets the bot's
  role view and write there, for releases and advisories; the hub has only the
  entry that keeps members from writing. A plan run holds a `set-access` step
  for it.
- **Three roles have other names or are missing.** The hub's bot role is
  called "Bots"; the definition calls it "Diallak". Compare reports a
  `role-name` difference for a person to settle. "Ko-fi supporter" and
  "Patreon supporter" are missing, so a plan run holds a `create-role` step
  for each. Apply creates them hidden, with no rights and nothing attached,
  as [ADR-0024](adr/0024-supporter-roles-from-funding-platforms.md) decides;
  the operator then maps the Ko-fi and Patreon integrations to them.

## The target layout

"Rule answers" says whether a rule-based answer of replan step 3 appears in
the channel. "Projects" says which projects the channel serves; "all" means
every project in [Projects](#projects).

<!-- markdownlint-disable MD013 -->
| Category | Channel | Type | Purpose | Who writes | Projects | Rule answers | Change |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| Start | `rules` | text | The server rules with links to the code of conduct, the privacy notice and the imprint | operator | all | no | topic |
| Start | `announcements` | announcement | Own releases and security advisories | bot, operator | all | no | topic |
| Community | `lounge` | text | One relaxed room for everything around the projects; receives the system messages | members | all | only to `/ask` | topic, system messages |
| Community | `support` | forum | Questions, one post each | members | each by its tag | yes, on posts that carry the tag "Automated answer" | topic, project tags |
| Community | `tickets` | text | The private path to the operator, one private thread each | nobody in the channel; members in their own thread | all | no; every automated answer points here | topic |
| Areas | four topic rooms | text | As the definition states | members with the interest role | all | only to `/ask` | topic |
| Languages | four language rooms | text | Social rooms; support stays in English | members with the language role | all | only to `/ask` | topic |
| Events | `Events` | voice | Scheduled events only | members, during an event | all | no | none |
| Staff | `mod-queue` | text | Cases the platform's filter flagged | bot | all | no | topic |
| Staff | `mod-log` | text | The trail of moderator actions | bot | all | no | topic |
| Archive (new) | `general` | text | Retired; kept for its history | nobody | none | no | moved, read-only |
<!-- markdownlint-enable MD013 -->

`/ask` answers wherever a member uses it, visible only to that member
(`automation[ask-command]` in the definition). The bot does not answer chat
it was not asked in: an answer nobody asked for is noise in a room meant for
people.

### Archive

The new category holds what the hub no longer uses, so that nothing has to be
deleted to tidy up. The access entry of `general`: `@everyone` may not view;
moderators may view and read the history, and may not write; the bot's role
may view only, with no history or writing. The bot's view entry lets the
read return `general`. Archive has no access entry of its own, so
`@everyone`'s base view lets the read return it too. Compare checks the move
and entry, and no later plan proposes a second Archive. A channel moved here
keeps its messages.

### Channels this plan does not touch

- `moderator-only` and `cauda-alerts` were named in an earlier list and not
  returned by the read. Either they are gone or the reading bot cannot see
  them. They stay as they are until a read that can see them says what they
  are; `cauda-alerts` belongs to the cluster's own bot, not to this
  definition.
- No product channel is added. A product gets a channel only as an area,
  once it passes a threshold.

## Projects

The support forum carries one tag per project. A project here is every
public, active product repository of the fleet's organisations, since the
hub's invite goes into each of their READMEs (replan step 5). A tag also names
the documentation that rule-based answers for that project quote from (step
3): the repository's `docs/` folder on its default branch, or its README where
it has no `docs/` folder.

<!-- markdownlint-disable MD013 -->
| Tag | Repository | Default branch | Docs source for rule answers |
| :--- | :--- | :--- | :--- |
| Diallak | [tekmora/diallak](https://github.com/tekmora/diallak) | `main` | `docs/` |
| Epitrop | [tekmora/epitrop](https://github.com/tekmora/epitrop) | `main` | `docs/` |
| Amussis | [plurano/amussis](https://github.com/plurano/amussis) | `main` | `docs/` |
| Horistes | [plurano/horistes](https://github.com/plurano/horistes) | `main` | `docs/` (decision records only) |
| Saeptum | [plurano/saeptum](https://github.com/plurano/saeptum) | `main` | `docs/` (decision records only) |
| Tornus | [plurano/tornus](https://github.com/plurano/tornus) | `main` | `docs/` (decision records only) |
| Cistula | [plurano/cistula](https://github.com/plurano/cistula) | `main` | `docs/` (decision records only) |
| Peritia | [plurano/peritia](https://github.com/plurano/peritia) | `main` | `docs/` (decision records and research only) |
| VMAFx | [vidiko/vmafx](https://github.com/vidiko/vmafx) | `master` | `docs/` |
| Pelorus | [vidiko/pelorus](https://github.com/vidiko/pelorus) | `master` | `docs/` |
| SnatchArr | [lusoris/SnatchArr](https://github.com/lusoris/SnatchArr) | `main` | `docs/` |
| Rhizomorph | [lusoris/rhizomorph](https://github.com/lusoris/rhizomorph) | `main` | `docs/` |
| Embodied | [lusoris/embodied](https://github.com/lusoris/embodied) | `main` | `docs/` |
<!-- markdownlint-enable MD013 -->

The forge was read on 2026-11-19: all 13 are public and none is archived.
The two VMAFx repositories moved to the vidiko organisation; their old
addresses redirect (forge read 2026-11-20). Forks and supporting repositories
give no tag of their own; they fold into the tag of the product they serve
(the vidiko organisation's patch and contribution repositories into VMAFx).
A private or parked repository gets no tag. The definition carries the 13
tags, and `TestDefinition_Positive_SupportForumHasAProductTagPerRepository`
fails when one is missing. Every trigger repository in `rules/triggers.json` is
one of these 13, and `TestRuleTable_Positive_TriggerRepositoriesInChannelPlan`
fails when one is not.

A forum holds at most 20 tags (Discord API documentation, channel resource,
`available_tags`, read at commit `9b737ec51ee8`). The definition gives the
forum 18 (one "Automated answer", four languages, 13 projects), so two are
left. The validator refuses a 21st (`MaxForumTags` in
`internal/serverdef/serverdef.go`).

## Order of application

The undo steps below name what a person restores and what stays.

1. **System messages to `lounge`.** The operator sets the server's system
   messages channel by hand, once, before `general` is hidden; otherwise
   join messages would land in a channel nobody sees. The apply path has no
   action for it.
2. **Definition change.** One pull request adds the category Archive and the
   channel `general` under it with the access entry above. The same pull request
   extends `botChannelCeiling` in `internal/serverdef/validate_bot.go` to allow
   the bot's role entry `VIEW_CHANNEL` and nothing else in a channel under
   Archive. Its tests show that this view entry passes there, that a second
   right such as `READ_MESSAGE_HISTORY` is refused in that entry, and that the
   same view entry is refused on a plain text channel outside Archive.
3. **Plan.** The plan run of the cluster's run templates reads the hub and
   prints the plan and its digest: `create-category` for Archive,
   `move-channel` for `general`, `set-access` for its entry, and the
   differences listed under [The read](#differences-besides-general). A line
   that is in neither list stops the unit.
4. **Judgement.** The plan and its digest are judged before the run that
   applies it, and the server owner confirms the digest.
5. **Apply.** The apply run takes that digest and nothing else.
6. **Compare.** A compare run afterward shows no difference for Archive and
   `general`, because the read returns both: `general` through the bot's view
   entry and Archive through `@everyone`'s base view. The bot
   role's `role-name` difference under [The read](#differences-besides-general)
   remains until a person settles it. The expected `tickets` line remains too.
7. **By hand.** Channel topics and the forum's project tags, because apply
   does not write them ([Applying the server definition](server-apply.md),
   "How a plan is applied"). Before changing the tags, the operator writes
   down the forum's live tag list, with names and order. A later apply action
   can take this over.

Undoing:

- The operator sets the system messages channel back by hand.
- Revert the definition pull request. A later apply run takes back none of
  the server changes listed below: the reverted definition does not describe
  the old state of `general`. The bot's `announcements` entry stays: the
  definition required it before this change and still requires it after the
  revert.
- Move `general` back into no category and remove its new access entry by
  hand.
- Undo the funding integration mappings by hand. The empty category Archive
  and the two created roles stay unless a separate decision deletes them;
  apply deletes nothing.
- Clear the channel topics set by hand, since the read found none, and restore
  the forum tags by hand from the saved list.

## Open points

- **Docs source of five libraries.** The `docs/` folders of Horistes,
  Saeptum, Tornus, Cistula and Peritia hold decision records only, so a rule
  answer for them would quote a decision record. Whether their README serves
  instead is open until their first trigger is written.
- **Two tags left.** A new product repository past the 18th tag needs a
  decision on which tags to merge.
- **The two unseen channels.** `moderator-only` and `cauda-alerts`, until a
  read can see them.
- **Topics and tags as data.** Whether the apply path learns to set channel
  topics, forum tags and the system channel, or these stay a hand step.
- **The bot role's name.** The server calls the bot's role "Bots"; the
  definition calls it "Diallak". The naming step, set aside for now, settles
  the name once for both.
