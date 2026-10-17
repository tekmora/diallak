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
| Last line of every other reply | "Automated reply. A person can help in the support channel." | In the code |
| Last line of a receipt | "This receipt is automated." | In the code |
| Guidelines of the support forum, shown before a member posts | "The automated assistant reads a new post only when you add the tag 'Automated answer' when you create it. It then reads the first message and may reply first with excerpts from the documentation. It reads nothing else here. Do not ask for it on a post that holds health data or other sensitive data, or data about other people. More is in the privacy notice." | Accepted with [ADR-0020](../adr/0020-forum-answers-only-opted-in-posts.md), set by the operator |
| Guidelines of the support forum, a paragraph of its own after the text above | "You can object at any time to the assistant reading your posts here: write to the e-mail address in the privacy notice, or open a private thread in the ticket channel. The privacy notice explains this under 'Your right to object'." | Accepted with [ADR-0018](../adr/0018-forum-guidelines-name-the-right-to-object.md), set by the operator |
| First line of a reply in the support forum | "Automated excerpts from the documentation. No AI model wrote this reply." | In the code |
| Last line of a reply in the support forum | "Automated first reply. A person can help here in this post." | In the code |
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
- The receipt's last line is constant `textReceiptAutomated` in
  [`internal/discord`](../../internal/discord/receipt.go).
- The reply footer and refusal text are produced by `Footer`, `Refusal` and
  `AppendFooter` in [`internal/answer`](../../internal/answer/answer.go), using
  `DefaultPlace` when no support channel is configured.
- Replies rendered through `Render` in
  [`internal/answer`](../../internal/answer/answer.go) append the footer as the
  final line across all answer kinds.
- A reply in the support forum is rendered through `RenderForum` in
  [`internal/answer`](../../internal/answer/answer.go): `LabelExcerpts` first
  and constant `FooterForum` last, which names the post itself as the place
  where a person helps. The support channel setting does not change it.
- The optional setting `DIALLAK_SUPPORT_CHANNEL` replaces the words "the support
  channel" in the footer and in the refusal. It accepts printable text of at
  most 64 characters without "@"
  ([`internal/config`](../../internal/config/config.go)).

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
- `TestRenderForum_Positive_FirstAndLastLine` and
  `TestRenderForum_Boundary_LimitKeepsTheFooter` in
  [`internal/answer`](../../internal/answer/forum_test.go) pin the forum
  footer and verify that it stays the last line when the reply is cut to
  Discord's limit.
- `TestReceiptText_Positive_RepliedReceiptNamesGuardrailsPoliciesAndRetention`
  in [`internal/discord`](../../internal/discord/receipt_test.go) verifies that
  a receipt carries the automated notice.
- `TestHandleAsk_Positive_AnswerRepliesNameTheConfiguredPlace` in
  [`internal/discord`](../../internal/discord/review_test.go) verifies that a
  configured place reaches answers, excerpts and refusals.

## Not covered here

- What the bot does with a member's question: see the [data
  inventory](data-inventory.md).
- The privacy notice, which is a separate document.
