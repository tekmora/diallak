# Data inventory

Draft for operator review. It lists what the bot receives, keeps, logs and
passes on. [ADR-0002](../adr/0002-data-and-compliance.md) requires it: a change
that stores a new field adds its entry here in the same pull request.

The state below is the code on `main`. Each row names where it can be checked.

## In short

The bot stores nothing about members. It handles a question, or a member's
choice of area roles, in memory for the length of one request, and its logs
carry no question, no answer and no member id. For 15 minutes it holds the
receipt of a reply: names of guardrails and policies, and, only with the bot's
own masking, the masked text of the question; never the question as written.
When a moderator removes a message or times a member out, the bot writes one
line of the moderation trail into the server's log channel, with the ids of
the moderator and the affected member, and deletes the line after 90 days.
When the support forum is configured, the bot reads the title and the first
message of a new post that carries the opt-in tag, once, and holds them in
memory until its reply is sent.

## What the bot receives

<!-- markdownlint-disable MD013 -->
| Data | From | Used for | Kept |
| :--- | :--- | :--- | :--- |
| The question a member passes to `/ask` | Discord, with the command | Finding passages and, in model mode, writing the answer | In memory until the reply is sent |
| The id of the server the command came from | Discord, with the command | Refusing any server but the configured one | Not kept |
| For `/areas` and its menus: the member's id, the roles the member holds and whether the member has still to pass the server's screening | Discord, with the command or the menu change | Selecting the roles the member holds, working out which roles to give or take, addressing the two role calls, refusing a member in screening | In memory until the reply is sent |
| For a menu change of `/areas`: the options the member chose | Discord, with the menu change | Checking them against the options of the definition and giving or taking the roles | In memory until the reply is sent |
| The server's roles: id and name of each | Discord, read at start and again for each menu change | Finding the role of each option by name | In memory until the next read |
| A press of a receipt button: the random key in the button | Discord, with the button press | Finding the receipt the bot holds | Not kept beyond the answer |
| A press of the receipt's direct message button: the member's id | Discord, with the button press | Opening the direct message to that member, for that one request | In memory until the direct message is sent |
| For `Remove message`, its form and `/timeout`: the calling member's id and roles | Discord, with the command or the form | Refusing a caller without the Moderator role; naming the moderator in the trail line | In memory until the reply is sent; the id stays in the trail line |
| For `Remove message`: the id of the message, its channel and the id of its author | Discord, with the command | Deleting the message; naming it in the trail line | In memory until the form is sent back; the ids stay in the trail line |
| For `/timeout`: the id of the member, the length and the reason | Discord, with the command | Timing the member out; naming the member and the reason in the trail line | In memory until the reply is sent; the id and the reason stay in the trail line |
| For the form of `Remove message`: the reason | Discord, with the form | The audit log reason and the trail line | In memory until the reply is sent; the reason stays in the trail line |
| The server's roles and channels, and the bot's own user id | Discord, read at start and once a day | Finding the Moderator role and the log channel by name; telling the bot's own trail lines apart | In memory until the next read |
| The lines in the log channel: id, author id and time of each | Discord, read once a day | Finding the bot's own lines older than the retention | Not kept beyond the sweep |
| Title of a new post in the support forum | Discord, with the thread event | Finding passages | In memory until the reply is sent or the attempt ends |
| Text of the first message of that post, up to the question limit | Discord, on the bot's request for that one message | Finding passages | In memory until the reply is sent or the attempt ends |
| Whether the author of that message is an application | Discord, with the message | Leaving such a post without a reply | Not kept |
| Whether a new post carries the opt-in tag | Discord, with the thread event | Reading only a tagged post: the first message of any other post is not requested | Not kept |
| Events about channels, roles and threads of the server, with the title of each new thread and the id of its creator | Discord, with the standard intent that carries thread events, only when the support forum is configured | Nothing, except telling a new opted-in post in the configured forum from everything else by the id of the parent channel, the kind of thread, the mark of a new thread and its tags | Not kept |
| Other fields of the first message of a post: author id and name, attachments, embeds, polls, components | Discord, with the message | Nothing: the bot reads none of them | Not kept |
| Other fields Discord sends with a command or a button press, such as the member's name and locale, the member's id and roles with `/ask`, and the text of the message a `Remove message` names | Discord, with the command | Nothing: the bot reads none of them | Not kept |
<!-- markdownlint-enable MD013 -->

Check: the adapter reads the server id and the question, and uses the handle
Discord issues for replying to that one command
([`internal/discord/disgo.go`](../../internal/discord/disgo.go),
`interactionAdapter`). For `/areas` the adapter reads the server id, the
member's id, roles and screening flag, and the chosen options
([`internal/discord/disgo_areas.go`](../../internal/discord/disgo_areas.go),
`areasAdapter`). For a receipt button it reads the key in the button, and the
member's id only to open the direct message
([`internal/discord/disgo_receipt.go`](../../internal/discord/disgo_receipt.go),
`receiptAdapter`). For the moderation commands it reads the server id, the
caller's id and roles, the ids of the message, its channel and its author, and
the options and the reason
([`internal/discord/disgo_moderation.go`](../../internal/discord/disgo_moderation.go),
`moderationAdapter`, `removeMessageTarget`, `timeoutRequest`). The daily sweep
reads only the id, the author and the time of a line (`ListLines` in the same
file). The gateway session requests no intent, so no message,
member or presence event reaches the bot (`gatewayIntents` in
[`internal/discord/disgo.go`](../../internal/discord/disgo.go)). Only when
`DIALLAK_FORUM_CHANNEL_ID` and `DIALLAK_FORUM_OPT_IN_TAG_ID` are set does it
request the one standard intent that carries thread events, still no message
intent and no privileged intent
([ADR-0009](../adr/0009-support-forum-first-answer.md),
[ADR-0020](../adr/0020-forum-answers-only-opted-in-posts.md)). For the support
forum the bot compares the parent channel, the kind of thread, the mark of a
new thread and the tags of each created thread, and only for a new opted-in
post requests the first message through the HTTP API, one request for one
message ([`internal/discord/forum.go`](../../internal/discord/forum.go),
`skipReason`; [`internal/discord/disgo_forum.go`](../../internal/discord/disgo_forum.go),
`forumPostOf`, `restForumCalls`). The Discord library's caches keep their
default, which stores nothing, so no thread title is kept
(`TestRunForum_Positive_OptedInPostIsAnsweredOverREST`). The start stops when the application lacks the right to
message content (`TestRunForum_Negative_StartStopsWithoutTheRight`), and
`TestRunForum_Negative_OnlyNewOptedInPostsAreRead` shows that an old post, a
post without the tag, a message event and an edit are not read.
When it is switched on, the health listener answers the deployment's probes.
It reads no request body, writes no log line per request and holds no member
data ([`internal/health`](../../internal/health/health.go)).

## What the bot stores

Nothing on disk. It has no database and writes no file.

In memory it holds one receipt per reply for which text left the bot
([ADR-0013](../adr/0013-guardrail-policy-and-receipt.md)): the names of the
guardrails and policies the gateway named in its reply headers, whether the
reply named the deployment's policy, the time, and whether the model's answer
was shown. It holds no member id; the key is random and carried in the
button. A receipt is held for 15 minutes, at most 1000 at once with the oldest
going first, and a restart empties the store.

With the bot's own masking (`DIALLAK_MODEL_GUARDRAIL` set,
[ADR-0013](../adr/0013-guardrail-policy-and-receipt.md) step two) the receipt
also holds the masked text of the question, as the gateway's masking route
returned it and at most 8 KiB, and the kinds of data the masking replaced with
their counts. It is held with the receipt for the same 15 minutes. The
question as the member wrote it is never held: a passage the masking missed
stands in the masked text as written, and the receipt says so. Without the
bot's own masking the receipt holds no text of the member.

Check: `Receipts` in
[`internal/discord/receipt.go`](../../internal/discord/receipt.go);
`TestReceipts_Boundary_ReceiptIsHeldForExactlyFifteenMinutes` and
`TestReceipts_Boundary_OldestGoesFirstAtTheMaximum` in
[`internal/discord`](../../internal/discord/receipt_test.go);
`TestAskReceipt_Positive_PlantedQuestionReachesTheModelMaskedAndTheReceiptShowsIt`
in [`internal/discord`](../../internal/discord/receipt_mask_e2e_test.go);
`MaxMaskedBytes` in [`internal/model`](../../internal/model/mask.go).

The moderation trail is not stored by the bot: each line is a message in the
server's log channel, which Discord holds
([ADR-0021](../adr/0021-moderation-commands-and-trail.md)). A line reads
`<time> · <moderator> · <act> · <affected> · reason: <reason>`: the time in
UTC, the mention of the moderator, `removed message` or `timed out until` the
end, the member id of the affected member (for a removed message also the
channel and the message id), and the reason the moderator gave. It holds no
text of the removed message and notifies no one. Once a day the bot deletes
its own lines older than the retention, 90 days unless
`DIALLAK_TRAIL_RETENTION` sets 1 to 365 days, one at a time.

Check: `line` and `Sweep` in
[`internal/discord/moderation.go`](../../internal/discord/moderation.go);
`TestModerationSweep_Positive_DeletesOnlyTheBotsOwnOldLines` and
`TestModerationSweep_Boundary_LineAtTheRetentionStays` in
[`internal/discord`](../../internal/discord/moderation_sweep_test.go).

## What the bot logs

<!-- markdownlint-disable MD013 -->
| Field | Example | Personal data |
| :--- | :--- | :--- |
| Outcome of a request | `answer`, `excerpts`, `refusal`, `failure` | No |
| Reason | `no_source`, `model_unavailable` | No |
| Error class | `timeout`, `error` | No |
| A change of the model's state, with the class of the failure and the status code when there is one | `model not ready`, `status`, `503` | No |
| Duration | `1.2s` | No |
| For `/areas`: the role of the definition, whether it was given or taken, the outcome, the error class of a failed call, the reason of a refusal | `interest-games`, `given`, `ok`, `not-an-option` | No |
| For a receipt: the outcome, and the error class of a failed call | `shown`, `gone`, `sent`, `closed`, `failure` | No |
| For a post in the support forum: the outcome, the reason, the error class of a failed call; at debug level, why a thread was skipped | `excerpts`, `no_source`, `application_author`, `busy`, `not_opted_in` | No |
| For a moderation command: the reason of a refusal, the error class of a failed act or trail line; for the daily sweep, the number of lines deleted | `not-moderator`, `timeout`, `deleted` | No |
| At start: number of sources and passages, each source with its revision, whether a model is configured, the model alias | `lusoris/SnatchArr@9e70c2e03d22` | No |
| Name of a command or numeric type of an interaction the bot does not serve, at debug level only | `other` | No |
| Type of a recovered program fault | `string` | No |
| Warnings and errors of the Discord library: the message and listed fields only (counters, durations, flags, status codes, type names, the name of the library part); for an error its Go type names, never its text; the number of fields held back | `rate limit exceeded`, `retry_after`, `error_types`, `dropped_attrs` | No |
<!-- markdownlint-enable MD013 -->

The logs never hold a question, an answer, a member id, the bot token or the
text of an error from the model endpoint. For the support forum they hold no
title, no text, no post id and no member id. The Discord library's own debug
output, which would include message bodies, is cut off. Its warnings and errors
pass a fixed list of fields; a reply body, a request address, an error text and
the value of a program fault are held back, also in a fault of the reply call
and in the one-shot commands. A field a later library version adds stays out
until it is put on the list
([`internal/discord/disgo.go`](../../internal/discord/disgo.go),
`libraryAllowedAttrs`).

Check: `TestHandleAsk_Negative_LogCaptureNeverContainsQuestionOrError`,
`TestHandleAsk_Positive_SuccessLogsOutcomeWithoutContent`,
`TestLibraryLogger_Negative_BodiesNeverReachTheLog`,
`TestLibraryLogger_Positive_EndToEndRestUnmarshallingError` and
`TestNewRestClient_Negative_LibraryRecordsOfOneShotCommandsAreFiltered` and
`TestAreasChange_Positive_GivesAndTakesOnlyTheDifference` in
[`internal/discord`](../../internal/discord), and
`TestForum_Negative_LogHoldsNoContent` in
[`internal/discord`](../../internal/discord/forum_test.go), and
`TestAsk_Negative_ModelErrorTextIsNeverLogged` in
[`internal/answer`](../../internal/answer), and
`TestGate_Negative_ErrorTextNeverReachesTheLog` in
[`internal/model`](../../internal/model/gate_test.go).

## What the bot passes on

<!-- markdownlint-disable MD013 -->
| To | What | When |
| :--- | :--- | :--- |
| Discord | The reply, visible only to the member who asked | Every request |
| Discord | A role given to or taken from the member, with the fixed reason `The member's own choice through /areas` | Each menu change of `/areas`, one call per role that changes |
| Discord | The receipt, visible only to the member who pressed its button | Each press of a receipt button |
| Discord | The deletion of a message or the timeout of a member, with the moderator's reason for the audit log | Each act of a moderator |
| Discord | The trail line, into the log channel, mentioning no one | Each act that Discord carried out |
| Discord | The deletion of a trail line of the bot's own past the retention | Once a day |
| Discord | A public reply in a post of the support forum, with excerpts from the documentation, mentioning no one | A new opted-in post for which a passage is found; at most one reply per post |
| Discord | The receipt as a direct message to the member who pressed for it | Only when the member presses the direct message button; never on the bot's own |
| The model endpoint the operator configures | The question and the retrieved documentation passages | Model mode only |
| The model endpoint the operator configures | One fixed word, to learn whether the endpoint answers | Model mode only, once at start |
| The model endpoint's masking route | The question as the member wrote it, before anything goes to the model | Only with `DIALLAK_MODEL_GUARDRAIL` set, every request in model mode |
| The model endpoint's masking route | One fixed sentence with an invented address, to learn whether the masking works | Only with `DIALLAK_MODEL_GUARDRAIL` set, at start and before each retry |
<!-- markdownlint-enable MD013 -->

- The model endpoint receives no member id and no name.
- It must not keep or train on what it receives (ADR-0002). The operator runs
  it. What it holds for a time anyway, a cached answer and a log line about a
  blocked question, is listed in the [privacy notice](privacy-notice.md),
  "How long data is kept".
- The model endpoint receives nothing from the support forum, whatever the
  model settings are: posts are answered with excerpts only
  (`TestExcerpts_Positive_NeverCallsTheGenerator` in
  [`internal/answer`](../../internal/answer/forum_test.go)).
- In excerpt mode the question goes nowhere: the reply holds documentation text,
  not the question, and carries no receipt button.
- The endpoint's guardrails can mask or change the text before the model. They
  do not find everything. Without the bot's own masking the bot does not see
  their result, so no document and no receipt says that personal data is
  masked.
- With the bot's own masking, the model receives only the masked text. When
  the masking route times out, refuses or returns no text, the model receives
  nothing and the member gets excerpts
  (`TestCompleteMasked_Negative_NoMaskedTextCallsNoModel` in
  [`internal/model`](../../internal/model/gate_mask_test.go)). A start whose
  invented address comes back unmasked stops
  (`TestGateStart_Negative_UnmaskedProbeStopsTheStart`).

Check: the request body is built in
[`internal/model`](../../internal/model/model.go) from the prompt alone; the
prompt is built in `buildUserPrompt` in
[`internal/answer`](../../internal/answer/answer.go). The request at start is
built in `Probe` in the same package (`TestProbe_Positive_RequestShape`).

## What Discord holds

Discord records that a member used the command, with its text, and holds the
reply, under its own terms. The bot has no control over that and no copy of it.

For `/areas`, Discord holds the role on the member, which other members of the
server can see, and one entry in the server's audit log for each role change.
Discord keeps audit log entries for 45 days and shows them to roles that may
view the audit log ([ADR-0011](../adr/0011-area-roles-from-member-choice.md)).

For the moderation commands, Discord holds the trail lines in the log channel
until the bot deletes them, and an audit log entry for each act with the
moderator's reason. A timeout shows on the member while it lasts.

For the support forum, Discord holds the post and the bot's public reply under
its own terms. The bot keeps no copy of the reply and no reference to it.

## Not yet built

These will add entries here when they are built:

- Support tickets, which need stored state. Their retention is decided: 30 days
  after a ticket is closed, as a default that configuration can change.
- Reading GitHub Discussions ([ADR-0004](../adr/0004-community-questions.md)).
- Supporter roles.
