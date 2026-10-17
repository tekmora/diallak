# Compliance documents

These are working drafts. None of them is published to members yet, and none is
legal advice. The operator reviews each one before it is used.

<!-- markdownlint-disable MD013 -->
| Document | What it is | State |
| :--- | :--- | :--- |
| [Role under the EU AI Act](../adr/0006-ai-act-role.md) | Which role the project has and which duties follow | Accepted record; its gates for model-written answers are replaced by [ADR-0013](../adr/0013-guardrail-policy-and-receipt.md) |
| [AI disclosure](ai-disclosure.md) | What the bot tells members about itself, and where | Draft |
| [Data inventory](data-inventory.md) | What the bot receives, keeps, logs and passes on | Draft, checked against the code |
| [Legal research](legal-research.md) | Findings and sources on the nine open legal points of the privacy notice and the support forum answer | Research note, not legal advice; not a launch gate |
| [Imprint](imprint.md) | Who provides the bot and the server | Draft; the identity block is filled |
| [Privacy notice](privacy-notice.md) | What members are told about their data, point by point of GDPR Article 13 | Draft; content decided by the operator |
| [Record of processing](record-of-processing.md) | The record of processing activities that GDPR Article 30 asks for | Draft, built from the data inventory |
| [Server rules](server-rules.md) | The short add-on to the code of conduct that a member reads when joining | Draft wording accepted; final when the operator accepts it |
| [Support forum answer](support-forum-answer.md) | Where the entries for the first answer in the support forum went, and the gates before it is switched on | Built, off by default; [ADR-0009](../adr/0009-support-forum-first-answer.md) and [ADR-0020](../adr/0020-forum-answers-only-opted-in-posts.md) are its accepted records |
<!-- markdownlint-enable MD013 -->

The rules these documents implement are in
[ADR-0002](../adr/0002-data-and-compliance.md). Moderation rests on two more
texts ([ADR-0005](../adr/0005-moderation-and-escalation.md)): the
[moderator guide](../moderator-guide.md), and the
[code of conduct](https://github.com/tekmora/.github/blob/main/CODE_OF_CONDUCT.md),
which is kept once per organisation
([ADR-0008](../adr/0008-code-of-conduct-place.md)). English is the source and
the binding version of every text here; translations are linked and marked as
translations.

## Gates for model-written answers

Model-written answers stay off in the live server until the gates of
[ADR-0013](../adr/0013-guardrail-policy-and-receipt.md), "What may go live,
and when", are passed. They replace the four gates of
[ADR-0006](../adr/0006-ai-act-role.md); the "Supersedes" section of ADR-0013
says what became of each.

<!-- markdownlint-disable MD013 -->
| Gate | State |
| :--- | :--- |
| 1. The check for the guardrail policy, at the start and on every reply, and the receipt are in the code | Built ([issue 63](https://github.com/tekmora/diallak/issues/63)) |
| 2. Each has its planted case in the test suite | Built: `internal/answer/answer_policy_test.go`, `cmd/diallak/main_test.go`, `internal/discord/receipt_test.go`, `internal/discord/receipt_gateway_test.go` |
| 3. The privacy notice, the data inventory and the AI disclosure say what the code does, and none says that personal data is masked without the limit of the masking | Written; the operator checks them before model mode goes live |
<!-- markdownlint-enable MD013 -->

A deployment passes a gate of its own at every start: the bot does not run in
model mode when the reply to its first request does not name the policy. The
record's second step, in which the bot gets the masked text and shows it in the
receipt, is not a gate for going live.

## Gates for the public launch

Nothing here is published to members before these are passed.

<!-- markdownlint-disable MD013 -->
| Gate | State |
| :--- | :--- |
| The identity block is filled: name, address, e-mail | Passed; the address is a c/o address, no address service |
| The ticket channel exists in the server, and a member can open a private thread there | Open; the operator creates it. The server rules and the privacy notice name it as the second path next to e-mail |
| The imprint and the privacy notice are published and linked from the bot's profile, the server rules, the README and Discord's developer portal | Open |
<!-- markdownlint-enable MD013 -->
