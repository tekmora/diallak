# Compliance documents

These are working drafts. None of them is published to members yet, and none is
legal advice. The operator reviews each one before it is used.

<!-- markdownlint-disable MD013 -->
| Document | What it is | State |
| :--- | :--- | :--- |
| [Role under the EU AI Act](../adr/0006-ai-act-role.md) | Which role the project has and which duties follow | Accepted record, with gates for model-written answers |
| [AI disclosure](ai-disclosure.md) | What the bot tells members about itself, and where | Draft |
| [Data inventory](data-inventory.md) | What the bot receives, keeps, logs and passes on | Draft, checked against the code |
<!-- markdownlint-enable MD013 -->

Still to come: the privacy notice and the server rules. The rules these
documents implement are in [ADR-0002](../adr/0002-data-and-compliance.md).

## Gates for model-written answers

Model mode stays disabled until the four gates in
[ADR-0006](../adr/0006-ai-act-role.md) are satisfied.

<!-- markdownlint-disable MD013 -->
| Gate | Description | State |
| :--- | :--- | :--- |
| 1 | Notices in the code | Done: command description and reply footer added in `feat/notices-in-code` |
| 2 | Codes of practice read | Open |
| 3 | Privacy notice published | Open |
| 4 | Endpoint gaps closed | Open (operator's deployment) |
<!-- markdownlint-enable MD013 -->

Gate 1 is verified in the test suite by the exact string pins in
[`internal/answer`](../../internal/answer/answer_test.go) and
[`internal/discord`](../../internal/discord/discord_test.go).
The remaining three gates belong to documentation review and the operator's
deployment environment.
