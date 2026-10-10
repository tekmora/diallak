# AI disclosure

This page fixes what the bot says about itself and where, so that the wording
in the code can follow it. The text of the command description and the last
line of a reply are approved; the profile text is still a proposal.

## The rule

A member must know that a reply is automated before relying on it. The bot says
so at first contact and on every reply, in plain text.

This implements Article 50(1) and 50(5) of Regulation (EU) 2024/1689: a person
who interacts with an AI system is informed of that, clearly and
distinguishably, at the latest at the first interaction, in an accessible way.
The reading behind it is in [ADR-0006](../adr/0006-ai-act-role.md).

## Where the notice appears

<!-- markdownlint-disable MD013 -->
| Place | Text | State |
| :--- | :--- | :--- |
| Description of the `/ask` command, shown before first use | "Ask the automated assistant a question about the project documentation" | In the code |
| First line of a model-written reply | "AI-generated answer. It can be wrong: check the sources below." | In the code |
| First line of an excerpt reply | "Automated excerpts from the documentation. No AI model wrote this reply." | In the code |
| Second line when the model failed | "The AI model is unavailable right now, so these are excerpts only." | In the code |
| Last line of every reply | "Automated reply. A person can help in the support channel." | In the code |
| The bot's profile text | "Automated assistant of this community. It answers from project documentation and can be wrong." | Proposed, set by the operator |
<!-- markdownlint-enable MD013 -->

The texts in the code are the constants `LabelAnswer`, `LabelExcerpts` and
`LabelModelUnavailable` in [`internal/answer`](../../internal/answer/answer.go).

## Requirements on the wording

- **Plain text.** No emoji, colour or image carries the notice alone, so a
  screen reader reads it.
- **On every reply.** A member's first reply is then always covered, whichever
  it is.
- **Says what kind of reply it is.** A model-written answer and an excerpt reply
  have different first lines, because they deserve different trust.
- **Points to a person.** Every reply names the way to reach a human.
- **Stable.** The first line of a model-written reply is also the marker by
  which a program can detect generated text (ADR-0006, "Marking generated
  text"). Changing it changes that marker.

## Notices in the code

The approved texts are fixed by constants and formatting functions:

- The command description is constant `CommandDescription` in
  [`internal/discord`](../../internal/discord/discord.go).
- The answer markers are constants `LabelAnswer`, `LabelExcerpts` and
  `LabelModelUnavailable` in
  [`internal/answer`](../../internal/answer/answer.go).
- The reply footer and refusal text are produced by `Footer`, `Refusal` and
  `AppendFooter` in [`internal/answer`](../../internal/answer/answer.go), using
  `DefaultPlace` when no support channel is configured.
- Replies rendered through `Render` in
  [`internal/answer`](../../internal/answer/answer.go) append the footer as the
  final line across all answer kinds.

Every text is pinned by tests to prevent accidental edits:

- `TestCommandDescription_Positive_ExactStringPin` in
  [`internal/discord`](../../internal/discord/discord_test.go) pins the command
  description.
- `TestTexts_Positive_ExactStringPins` and
  `TestRender_Positive_ModelWrittenAnswerFirstLineIsMarker` in
  [`internal/answer`](../../internal/answer/answer_test.go) pin the answer
  labels, the default footer, and the refusal text.
- `TestRender_Positive_FooterIsLastLineForAllFiveCases` in
  [`internal/answer`](../../internal/answer/answer_test.go) and
  `TestHandler_Positive_FooterIsLastLineForAdapterTexts` in
  [`internal/discord`](../../internal/discord/discord_test.go) verify that the
  footer is the last line of every reply.

## Not covered here

- What the bot does with a member's question: see the [data
  inventory](data-inventory.md).
- The privacy notice, which is a separate document.
