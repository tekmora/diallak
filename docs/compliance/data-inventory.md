# Data inventory

Draft for operator review. It lists what the bot receives, keeps, logs and
passes on. [ADR-0002](../adr/0002-data-and-compliance.md) requires it: a change
that stores a new field adds its entry here in the same pull request.

The state below is the code on `main`. Each row names where it can be checked.

## In short

The bot stores nothing about members. It handles a question, or a member's
choice of area roles, in memory for the length of one request, and its logs
carry no question, no answer and no member id.

## What the bot receives

<!-- markdownlint-disable MD013 -->
| Data | From | Used for | Kept |
| :--- | :--- | :--- | :--- |
| The question a member passes to `/ask` | Discord, with the command | Finding passages and, in model mode, writing the answer | In memory until the reply is sent |
| The id of the server the command came from | Discord, with the command | Refusing any server but the configured one | Not kept |
| For `/areas` and its menus: the member's id, the roles the member holds and whether the member has still to pass the server's screening | Discord, with the command or the menu change | Selecting the roles the member holds, working out which roles to give or take, addressing the two role calls, refusing a member in screening | In memory until the reply is sent |
| For a menu change of `/areas`: the options the member chose | Discord, with the menu change | Checking them against the options of the definition and giving or taking the roles | In memory until the reply is sent |
| The server's roles: id and name of each | Discord, read at start and again for each menu change | Finding the role of each option by name | In memory until the next read |
| Other fields Discord sends with a command, such as the member's name and locale, and the member's id and roles with `/ask` | Discord, with the command | Nothing: the bot reads none of them | Not kept |
<!-- markdownlint-enable MD013 -->

Check: the adapter reads the server id and the question, and uses the handle
Discord issues for replying to that one command
([`internal/discord/disgo.go`](../../internal/discord/disgo.go),
`interactionAdapter`). For `/areas` the adapter reads the server id, the
member's id, roles and screening flag, and the chosen options
([`internal/discord/disgo_areas.go`](../../internal/discord/disgo_areas.go),
`areasAdapter`). The gateway session requests no intent, so no message,
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
| For `/areas`: the role of the definition, whether it was given or taken, the outcome, the error class of a failed call, the reason of a refusal | `interest-games`, `given`, `ok`, `not-an-option` | No |
| At start: number of sources and passages, each source with its revision, whether a model is configured, the model alias | `lusoris/SnatchArr@9e70c2e03d22` | No |
| Name of a command or numeric type of an interaction the bot does not serve, at debug level only | `other` | No |
| Type of a recovered program fault | `string` | No |
| Warnings and errors of the Discord library: the message and listed fields only (counters, durations, flags, status codes, type names, the name of the library part); for an error its Go type names, never its text; the number of fields held back | `rate limit exceeded`, `retry_after`, `error_types`, `dropped_attrs` | No |
<!-- markdownlint-enable MD013 -->

The logs never hold a question, an answer, a member id, the bot token or the
text of an error from the model endpoint. The Discord library's own debug
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
| The model endpoint the operator configures | The question and the retrieved documentation passages | Model mode only |
| The model endpoint the operator configures | One fixed word, to learn whether the endpoint answers | Model mode only, once at start |
<!-- markdownlint-enable MD013 -->

- The model endpoint receives no member id and no name.
- It must not keep or train on what it receives (ADR-0002). The operator runs
  it. What it holds for a time anyway, a cached answer and a log line about a
  blocked question, is listed in the [privacy notice](privacy-notice.md),
  "How long data is kept".
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

For `/areas`, Discord holds the role on the member, which other members of the
server can see, and one entry in the server's audit log for each role change.
Discord keeps audit log entries for 45 days and shows them to roles that may
view the audit log ([ADR-0011](../adr/0011-area-roles-from-member-choice.md)).

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
