# Product definition

What Diallak is when it is finished: who it serves, what version 0.10
contains, and what stays out. Milestones on the forge follow this page; when
the scope changes, this page changes in the same pull request.

Version 1, every release up to and including 1.0.0, carries no AI model: the
bot answers with excerpts from the documentation, and by rules once the
trigger table is built
([ADR-0028](adr/0028-version-1-answers-without-a-model.md),
[#251](https://github.com/tekmora/diallak/issues/251)). The plan of
2026-11-19 that led there is under [The replan](#the-replan-of-2026-11-19).

## Who it serves

- **Members of the one hub server.** They ask about the projects, report bugs,
  choose the areas they follow and read the projects' news. The bot serves a
  single guild, given as configuration
  ([ADR-0001](adr/0001-architecture-and-boundaries.md)).
- **Moderators.** They remove a message or time a member out through the bot,
  which keeps a trail of every act
  ([ADR-0021](adr/0021-moderation-commands-and-trail.md),
  [moderator guide](moderator-guide.md)).
- **The operator.** They run the deployment and apply the server definition on
  the one decided path
  ([ADR-0010](adr/0010-bot-rights-and-apply-path.md),
  [server apply](server-apply.md)).

The bot is not a product for other servers.

## Version 0.10

Version 0.10 is every function built today, live in the hub, plus the
rule-based answers and the redesigned channels. Each row names the record that
decides it and its state in
[`server/definition.json`](../server/definition.json) (`automation`).

<!-- markdownlint-disable MD013 -->
| Function | What a member or moderator sees | Record | State |
| :--- | :--- | :--- | :--- |
| Answers from the docs | `/ask` replies with cited excerpts, visible to the asker only and labelled as automated | [ADR-0003](adr/0003-answer-pipeline.md) | built |
| Rule-based answers | A common question gets the fixed answer of a trigger table, taken from the project's documentation, before any excerpt; every answer and every refusal names the `tickets` channel | [ADR-0028](adr/0028-version-1-answers-without-a-model.md) | planned ([#251](https://github.com/tekmora/diallak/issues/251)) |
| First answer in the support forum | A new post that carries the tag "Automated answer" gets a public first reply with excerpts | [ADR-0009](adr/0009-support-forum-first-answer.md), [ADR-0020](adr/0020-forum-answers-only-opted-in-posts.md) | built |
| Bug report link | `/bug` answers with a link that opens a filled-in new issue; the member submits it under their own account | [ADR-0001](adr/0001-architecture-and-boundaries.md) | built |
| Area roles | `/areas` gives and takes interest, language and notification roles | [ADR-0011](adr/0011-area-roles-from-member-choice.md) | built |
| Moderation through the bot | `Remove message` and `/timeout`, with the trail and the queue | [ADR-0021](adr/0021-moderation-commands-and-trail.md) | built |
| News | Own releases and security advisories as they happen, everything else as one digest per channel per day | [ADR-0019](adr/0019-news-posts.md) | built |
| Discussions bridge | New Q&A and Ideas discussions announced with a link back | [ADR-0004](adr/0004-community-questions.md) | built |
| Server settings as data | Filter rules and verification level compared with the definition and applied | [ADR-0015](adr/0015-moderator-without-rights-and-server-settings-as-data.md) | built |
| Channel layout | Categories and channels redesigned from a read of the hub, each with its purpose, permissions and the projects it serves | [ADR-0010](adr/0010-bot-rights-and-apply-path.md) | planned ([#252](https://github.com/tekmora/diallak/issues/252)) |
| Supporter roles | The two supporter roles given and taken from the funding platforms; off until a deployment switches it on ([#196](https://github.com/tekmora/diallak/issues/196)) | [ADR-0024](adr/0024-supporter-roles-from-funding-platforms.md) | built |
<!-- markdownlint-enable MD013 -->

No answer of version 1 is written by a model
([ADR-0028](adr/0028-version-1-answers-without-a-model.md)). The model and
embedding code of [ADR-0012](adr/0012-model-state.md),
[ADR-0013](adr/0013-guardrail-policy-and-receipt.md) and
[ADR-0022](adr/0022-embeddings-for-paraphrased-questions.md) stays in the
repository, off by default, and is not deleted.

## When 0.10 is done

0.10 is done when the issues of the milestone "0.10.0 — Complete hub bot"
are closed:

- the launch (milestone 0.5.0): live test, cutover, published legal texts and
  the forum answer;
- the server definition applied to the hub
  ([#197](https://github.com/tekmora/diallak/issues/197));
- every built function switched on and checked once in the hub
  ([#198](https://github.com/tekmora/diallak/issues/198), protocol in
  [live test](live-test.md));
- the supporter roles switched on
  ([#196](https://github.com/tekmora/diallak/issues/196));
- the rule-based answers
  ([#251](https://github.com/tekmora/diallak/issues/251));
- the channel layout planned and applied
  ([#252](https://github.com/tekmora/diallak/issues/252));
- the new name ([#253](https://github.com/tekmora/diallak/issues/253)) and the
  hub invite in every project README
  ([#254](https://github.com/tekmora/diallak/issues/254)).

Milestones up to 0.10.0 keep the 0.x.0 sequence. A stable 1.0.0 is the
operator's decision and no milestone carries it yet.

## The replan of 2026-11-19

The operator redirected the project on 2026-11-19 (paraphrased): keep AI out of
the bot for now, redesign the channels, answer common questions by rules, then
rename the bot and publish an invite. The work runs in this order; each step is
one pull request or more.

1. **No model in version 1.** This page and
   [ADR-0028](adr/0028-version-1-answers-without-a-model.md).
2. **Channel layout.** A read-only read of the hub through the bot gives the
   current categories, channels, types, topics and permission entries by role
   name, with no ids, member names or message text. One plan file lists every
   channel with its purpose, permissions and the projects it serves. After
   review it is applied through the bot
   ([server apply](server-apply.md)) in reversible order: create, then move,
   then archive. No channel is deleted without its own decision
   ([#252](https://github.com/tekmora/diallak/issues/252)).
3. **Rule-based answers.** The trigger table, its validator and the matcher,
   with a question that must match and one that must not for every trigger
   ([#251](https://github.com/tekmora/diallak/issues/251)).
4. **The name.** Three or four candidates go to the operator, each with its
   meaning, origin and pronunciation. Nothing is renamed before the pick
   ([#253](https://github.com/tekmora/diallak/issues/253)).
5. **The invite.** After the rename, the bot creates one permanent invite, and
   the organisation profile and every project README link it, one batch per
   organisation ([#254](https://github.com/tekmora/diallak/issues/254)).

Steps 2 and 3 do not wait for each other. Step 5 waits for step 4.

## After 0.10

- **Wishes and polls.** The bot becomes the Discord adapter of a shared core
  that the projects plan to use from every channel; it keeps no count of its
  own ([#100](https://github.com/tekmora/diallak/issues/100),
  [#101](https://github.com/tekmora/diallak/issues/101)). The core does not
  exist yet, so wishes and polls ship in the first minor release after it
  does, not in 0.10.

## Not in 0.10, and why

<!-- markdownlint-disable MD013 -->
| Function | Where it stays | Why |
| :--- | :--- | :--- |
| Opening and closing threshold areas and language rooms | Backlog; the operator does it by hand | No threshold is measured yet ([server definition](server-definition.md)) |
| Contributor role from merged contributions | The cluster's identity provider and Discord | A linked role: the provider verifies the forge account and Discord gives the role; the bot takes no part ([ADR-0026](adr/0026-member-to-forge-account-link.md)) |
| Tickets | Operator-run | The operator reads and answers the private ticket threads; the bot does not, and only names the channel in its answers |
| Events | Operator-run | The operator schedules events and opens the voice room |
| Granting moderator and guide roles | Operator-run | The bot grants neither ([ADR-0005](adr/0005-moderation-and-escalation.md)) |
| Spam filtering | The platform's AutoMod | The bot does not read messages for moderation ([ADR-0005](adr/0005-moderation-and-escalation.md)) |
| Model-written answers and embeddings | Off; the code stays and is not deleted | Version 1 carries no model ([ADR-0028](adr/0028-version-1-answers-without-a-model.md)) |
<!-- markdownlint-enable MD013 -->

## Never

- Functions of any private infrastructure.
- Training on messages: text from Discord answers the request at hand and is
  never used for model training or evaluation data.
- Paid perks: supporters get a role and a plain-text credit, nothing is sold,
  and there is no private supporter channel.
- Serving more than the one hub server.
