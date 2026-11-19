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
  4. Archive holds what is retired: read-only, visible to moderators, never
     deleted by this plan.
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

The read shows two more differences from the definition. A plan run will list
them next to the steps below, and each is judged with them:

- **`announcements` lacks the bot's entry.** The definition lets the bot's
  role view and write there, for releases and advisories; the hub has only the
  entry that keeps members from writing. A plan run holds a `set-access` step
  for it.
- **Three roles have other names or are missing.** The hub's bot role is
  called "Bots"; the definition calls it "Diallak". The hidden roles
  "Ko-fi supporter" and "Patreon supporter", which the funding integrations
  give ([ADR-0024](adr/0024-supporter-roles-from-funding-platforms.md)), are
  not on the hub. The plan never creates or changes a role another party
  manages; compare reports them for a person to settle.

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
deleted to tidy up. Its single access entry: `@everyone` may not view;
moderators may view and read the history, and may not write. A channel moved
here keeps its messages.

### Channels this plan does not touch

- `moderator-only` and `cauda-alerts` were named in an earlier list and not
  returned by the read. Either they are gone or the reading bot cannot see
  them. They stay as they are until a read that can see them says what they
  are; `cauda-alerts` belongs to the cluster's own bot, not to this
  definition.
- No product channel is added. A product gets a channel only as an area,
  once it passes a threshold.

## Projects

The support forum carries one tag per project. A tag also names the
documentation that rule-based answers for that project quote from (step 3).

<!-- markdownlint-disable MD013 -->
| Tag | Repository | Docs source for rule answers |
| :--- | :--- | :--- |
| Diallak | [tekmora/diallak](https://github.com/tekmora/diallak) | `README.md`, `docs/` |
| Epitrop | [tekmora/epitrop](https://github.com/tekmora/epitrop) | `README.md`, `docs/` |
| Amussis | [plurano/amussis](https://github.com/plurano/amussis) | `README.md`, `docs/` |
| VMAFx | [VMAFx/vmafx](https://github.com/VMAFx/vmafx) | `README.md`, `docs/` |
| Pelorus | [VMAFx/pelorus](https://github.com/VMAFx/pelorus) | `README.md`, `docs/` |
<!-- markdownlint-enable MD013 -->

These are the five products the definition names today. Whether further
repositories of the same organisations join is an open point. A forum holds
at most 20 tags (Discord API documentation, channel resource,
`available_tags`, read at commit `9b737ec51ee8`); the definition gives the
forum ten (one "Automated answer", four languages, five projects), so ten
are left.

## Order of application

Each step is a pull request or a run, and each can be undone by the next.

1. **System messages to `lounge`.** The operator sets the server's system
   messages channel by hand, once, before `general` is hidden; otherwise
   join messages would land in a channel nobody sees. The apply path has no
   action for it.
2. **Definition change.** One pull request adds the category Archive and the
   channel `general` under it with the access entry above. The validator and
   its tests cover both.
3. **Plan.** The plan run of the cluster's run templates reads the hub and
   prints the plan and its digest: `create-category` for Archive,
   `move-channel` for `general`, `set-access` for its entry, and the
   differences listed under [The read](#differences-besides-general). A line
   that is in neither list stops the unit.
4. **Judgement.** The plan and its digest are judged before the run that
   applies it, and the server owner confirms the digest.
5. **Apply.** The apply run takes that digest and nothing else.
6. **Compare.** A compare run afterwards shows no difference for these
   channels.
7. **By hand.** Channel topics and the forum's project tags, because apply
   does not write them ([Applying the server definition](server-apply.md),
   "How a plan is applied"). A later apply action can take this over.

Undoing: moving `general` back into no category and removing its access
entry restores the hub as it was read.

## Open points

- **Which projects.** Whether the plurano libraries and other public fleet
  repositories get a tag.
- **The two unseen channels.** `moderator-only` and `cauda-alerts`, until a
  read can see them.
- **Topics and tags as data.** Whether the apply path learns to set channel
  topics, forum tags and the system channel, or these stay a hand step.
- **The bot role's name.** The server calls the bot's role "Bots"; the
  definition calls it "Diallak". The rename of replan step 4 settles the name
  once for both.
