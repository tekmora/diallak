# Compliance documents

These are working drafts. None of them is published to members yet, and none is
legal advice. The operator reviews each one before it is used.

<!-- markdownlint-disable MD013 -->
| Document | What it is | State |
| :--- | :--- | :--- |
| [Role under the EU AI Act](../adr/0006-ai-act-role.md) | Which role the project has and which duties follow | Accepted record, with gates for model-written answers |
| [AI disclosure](ai-disclosure.md) | What the bot tells members about itself, and where | Draft |
| [Data inventory](data-inventory.md) | What the bot receives, keeps, logs and passes on | Draft, checked against the code |
| [Privacy notice](privacy-notice.md) | What members are told about their data, point by point of GDPR Article 13 | Draft with nine open points for the operator |
| [Server rules](server-rules.md) | The short list of rules a member reads when joining | Draft with six open points for the operator |
<!-- markdownlint-enable MD013 -->

The rules these documents implement are in
[ADR-0002](../adr/0002-data-and-compliance.md). Still to come: the code of
conduct and the moderator guide that
[ADR-0005](../adr/0005-moderation-and-escalation.md) requires before moderation
starts.

## Gates for model-written answers

Model-written answers stay off in the live server until the four gates of
[ADR-0006](../adr/0006-ai-act-role.md) are passed.

<!-- markdownlint-disable MD013 -->
| Gate | State |
| :--- | :--- |
| 1. The notices of the AI disclosure are in the code | Passed: [AI disclosure](ai-disclosure.md), "Notices in the code", names the texts and the tests that pin them |
| 2. The codes of practice under Article 50(7) are read for the marking of text | Open |
| 3. The privacy notice is published | Open: the [privacy notice](privacy-notice.md) is a draft and waits for the operator's open points |
| 4. The gaps of the operator's model endpoint are closed | Open; they are closed in the operator's deployment, not in this repository |
<!-- markdownlint-enable MD013 -->
