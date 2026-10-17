# Support forum answer: where the entries went

The first answer in the support forum is built and off by default.
[ADR-0009](../adr/0009-support-forum-first-answer.md) is its accepted record,
as amended by [ADR-0020](../adr/0020-forum-answers-only-opted-in-posts.md).
A deployment switches it on by setting `DIALLAK_FORUM_CHANNEL_ID` and
`DIALLAK_FORUM_OPT_IN_TAG_ID` together ([README](../../README.md), "Run").
It is not legal advice; [Legal research](legal-research.md), points 7 to 9,
has the findings on these entries.

## In short

- In the support forum the bot reads the title and the first message of a new
  post, once, and may reply first with excerpts from the documentation.
- It reads no other message, in the forum or anywhere else.
- Only a post that carries the opt-in tag "Automated answer" from its
  creation is read and answered.
- It keeps the text in memory until the reply is sent, stores nothing and logs
  no text and no id.
- The text is sent to nobody. No model writes the reply.

## Where each entry is now

This page held the drafted entries until the feature was built. Each now
stands in its document:

<!-- markdownlint-disable MD013 -->
| Entry | Document and section |
| :--- | :--- |
| Points for members, the right to object, data, recipients, periods, what not to send | [Privacy notice](privacy-notice.md): "In short", "Your right to object", "What is processed", "Who receives your data", "How long data is kept", "What you should not send" |
| What the bot receives, logs and passes on, with the checks in the code | [Data inventory](data-inventory.md) |
| Purposes, data subjects, recipients, time limits, security measures | [Record of processing](record-of-processing.md) |
| Guidelines text of the forum, first and last line of a reply | [AI disclosure](ai-disclosure.md), "Where the notice appears" |
| Rule 5 | [Server rules](server-rules.md) |
<!-- markdownlint-enable MD013 -->

## Before launch

No deployment sets the forum variables before these are passed.

- The findings of [Legal research](legal-research.md), points 7 to 9, are
  settled:
  - the guidelines text names the right to object (#145; accepted with
    ADR-0018);
  - posts with data of the kinds Article 9(1) lists have an answer (#146;
    ADR-0020 has the bot read only posts a member opted in, and whether the
    tag meets Article 9(2)(a) is still to be checked).
- The privacy notice with the forum entries is published
  ([compliance](README.md), "Gates for the public launch").
- The implementation passes the cases of ADR-0009, cases 17 to 19 for the
  opt-in tag of ADR-0020. The tests that name these cases are in
  `internal/discord/forum_test.go`, `internal/discord/forum_gateway_test.go`,
  `internal/answer/forum_test.go`, `internal/config/forum_test.go` and
  `cmd/diallak/forum_test.go`; the operator checks the list against the table
  of ADR-0009 before launch.
- The operator has created the opt-in tag "Automated answer", has set the
  guidelines text of the forum and has switched the right to message content
  on in Discord's developer portal. Without that right the bot does not start
  with the forum configured.
- The live test has shown the two points ADR-0009 lists as not tried.
