# Data inventory

Draft for operator review. It lists what the bot receives, keeps, logs and
passes on. [ADR-0002](../adr/0002-data-and-compliance.md) requires it: a change
that stores a new field adds its entry here in the same pull request.

The state below is the code on `main`. Each row names where it can be checked.

## In short

The bot stores nothing about members. It handles a question in memory for the
length of one request, and its logs carry no question, no answer and no member
id.

## What the bot receives

<!-- markdownlint-disable MD013 -->
| Data | From | Used for | Kept |
| :--- | :--- | :--- | :--- |
| The question a member passes to `/ask` | Discord, with the command | Finding passages and, in model mode, writing the answer | In memory until the reply is sent |
| The id of the server the command came from | Discord, with the command | Refusing any server but the configured one | Not kept |
| Other fields Discord sends with a command, such as the member's id, name, roles and locale | Discord, with the command | Nothing: the bot reads none of them | Not kept |
<!-- markdownlint-enable MD013 -->

Check: the adapter reads the server id and the question, and uses the handle
Discord issues for replying to that one command
([`internal/discord/disgo.go`](../../internal/discord/disgo.go),
`interactionAdapter`). The gateway session requests no intent, so no message,
member or presence event reaches the bot (`gatewayIntents` in the same file).
When it is switched on, the health listener answers the deployment's probes.
It reads no request body, writes no log line per request and holds no member
data ([`internal/health`](../../internal/health/health.go)).

## What the bot stores

Nothing. It has no database and writes no file.

## What the bot logs

<!-- markdownlint-disable MD013 -->
| Field | Example | Personal data |
| :--- | :--- | :--- |
| Outcome of a request | `answer`, `excerpts`, `refusal`, `failure` | No |
| Reason | `no_source`, `model_unavailable` | No |
| Error class | `timeout`, `error` | No |
| A change of the model's state, with the class of the failure and the status code when there is one | `model not ready`, `status`, `503` | No |
| Duration | `1.2s` | No |
| At start: number of sources and passages, each source with its revision, whether a model is configured, the model alias | `lusoris/SnatchArr@9e70c2e03d22` | No |
| Name of a command or numeric type of an interaction the bot does not serve, at debug level only | `other` | No |
| Type of a recovered program fault | `string` | No |
| Warnings and errors of the Discord library | connection problems | Not expected; not audited line by line |
<!-- markdownlint-enable MD013 -->

The logs never hold a question, an answer, a member id, the bot token or the
text of an error from the model endpoint. The Discord library's own debug
output, which would include message bodies, is cut off.

Check: `TestHandleAsk_Negative_LogCaptureNeverContainsQuestionOrError`,
`TestHandleAsk_Positive_SuccessLogsOutcomeWithoutContent` and
`TestLibraryLogger_Negative_BodiesNeverReachTheLog` in
[`internal/discord`](../../internal/discord), and
`TestAsk_Negative_ModelErrorTextIsNeverLogged` in
[`internal/answer`](../../internal/answer), and
`TestGate_Negative_ErrorTextNeverReachesTheLog` in
[`internal/model`](../../internal/model/gate_test.go).

## What the bot passes on

<!-- markdownlint-disable MD013 -->
| To | What | When |
| :--- | :--- | :--- |
| Discord | The reply, visible only to the member who asked | Every request |
| The model endpoint the operator configures | The question and the retrieved documentation passages | Model mode only |
| The model endpoint the operator configures | One fixed word, to learn whether the endpoint answers | Model mode only, once at start |
<!-- markdownlint-enable MD013 -->

- The model endpoint receives no member id and no name.
- It must not keep or train on what it receives (ADR-0002). The operator runs
  it.
- In excerpt mode the question goes nowhere: the reply holds documentation text,
  not the question.

Check: the request body is built in
[`internal/model`](../../internal/model/model.go) from the prompt alone; the
prompt is built in `buildUserPrompt` in
[`internal/answer`](../../internal/answer/answer.go). The request at start is
built in `Probe` in the same package (`TestProbe_Positive_RequestShape`).

## What Discord holds

Discord records that a member used the command, with its text, and holds the
reply, under its own terms. The bot has no control over that and no copy of it.

## Not yet built

These will add entries here when they are built:

- Support tickets, which need stored state. Their retention is decided: 30 days
  after a ticket is closed, as a default that configuration can change.
- The moderation trail ([ADR-0005](../adr/0005-moderation-and-escalation.md)).
  Its retention is decided: 90 days, as a default that configuration can
  change.
- Reading GitHub Discussions ([ADR-0004](../adr/0004-community-questions.md)).
- Supporter roles.
- A first answer in the support forum, for which the bot reads the title and
  the first message of a new post
  ([ADR-0009](../adr/0009-support-forum-first-answer.md)). The
  entries are drafted in [Support forum answer](support-forum-answer.md).
