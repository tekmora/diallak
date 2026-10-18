# Product definition

What Diallak is when it is finished: who it serves, what version 0.10
contains, and what stays out. Milestones on the forge follow this page; when
the scope changes, this page changes in the same pull request.

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

Version 0.10 is every function built today, live in the hub, plus the supporter
roles. Each row names the record that decides it and its state in
[`server/definition.json`](../server/definition.json) (`automation`).

<!-- markdownlint-disable MD013 -->
| Function | What a member or moderator sees | Record | State |
| :--- | :--- | :--- | :--- |
| Answers from the docs | `/ask` replies with cited excerpts, visible to the asker only and labelled as automated | [ADR-0003](adr/0003-answer-pipeline.md) | built |
| First answer in the support forum | A new post that carries the tag "Automated answer" gets a public first reply with excerpts | [ADR-0009](adr/0009-support-forum-first-answer.md), [ADR-0020](adr/0020-forum-answers-only-opted-in-posts.md) | built |
| Bug report link | `/bug` answers with a link that opens a filled-in new issue; the member submits it under their own account | [ADR-0001](adr/0001-architecture-and-boundaries.md) | built |
| Area roles | `/areas` gives and takes interest, language and notification roles | [ADR-0011](adr/0011-area-roles-from-member-choice.md) | built |
| Moderation through the bot | `Remove message` and `/timeout`, with the trail and the queue | [ADR-0021](adr/0021-moderation-commands-and-trail.md) | built |
| News | Own releases and security advisories as they happen, everything else as one digest per channel per day | [ADR-0019](adr/0019-news-posts.md) | built |
| Discussions bridge | New Q&A and Ideas discussions announced with a link back | [ADR-0004](adr/0004-community-questions.md) | built |
| Server settings as data | Filter rules and verification level compared with the definition and applied | [ADR-0015](adr/0015-moderator-without-rights-and-server-settings-as-data.md) | built |
| Supporter roles | The two supporter roles given and taken from the funding platforms | record to write ([#196](https://github.com/tekmora/diallak/issues/196)) | planned |
<!-- markdownlint-enable MD013 -->

Model-written answers are part of the answer function but stay off until the
gates of [ADR-0006](adr/0006-ai-act-role.md) and
[ADR-0013](adr/0013-guardrail-policy-and-receipt.md) are passed. Version 0.10
does not wait for them: excerpt answers are the 0.10 mode.

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
- the supporter roles ([#196](https://github.com/tekmora/diallak/issues/196)).

Milestones up to 0.10.0 keep the 0.x.0 sequence. A stable 1.0.0 is the
operator's decision and no milestone carries it yet.

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
| Tickets | Operator-run | The operator reads and answers the private ticket threads; the bot does not |
| Events | Operator-run | The operator schedules events and opens the voice room |
| Granting moderator and guide roles | Operator-run | The bot grants neither ([ADR-0005](adr/0005-moderation-and-escalation.md)) |
| Spam filtering | The platform's AutoMod | The bot does not read messages for moderation ([ADR-0005](adr/0005-moderation-and-escalation.md)) |
<!-- markdownlint-enable MD013 -->

## Never

- Functions of any private infrastructure.
- Training on messages: text from Discord answers the request at hand and is
  never used for model training or evaluation data.
- Paid perks: supporters get a role and a plain-text credit, nothing is sold,
  and there is no private supporter channel.
- Serving more than the one hub server.
