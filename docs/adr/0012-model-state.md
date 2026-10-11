# ADR-0012: The model's state: a start probe, a gate and a health path

- **Status**: Accepted
- **Date**: 2026-10-11
- **Authors**: Maintainers

## Context

In model mode the bot sends each question to an endpoint that the deployment
names with three variables: a base URL, an alias and a key file. This
repository holds no default for any of them.

Before this record a wrong alias, a wrong URL or a refused key showed only when
the first member asked. Every question then waited for the endpoint and came
back as excerpts, and nothing outside the log said that the model had never
worked. A deployment had no way to ask the bot whether its model answers.

[ADR-0003](0003-answer-pipeline.md) already says what a member gets when the
endpoint fails: the retrieved excerpts with the reason. That stays. What was
missing is the difference between a configuration that can never work and an
endpoint that is away for a moment, such as during its own restart.

## Decision

### One request at start

When both model variables are set, the bot sends one request to the endpoint
before it prints its status line and before it opens the gateway
([`cmd/diallak`](../../cmd/diallak/main.go), `probeModel`).

- The request goes to the chat completions path, under the configured alias,
  with the configured key. It holds one user message with a fixed word and asks
  for one output token ([`internal/model`](../../internal/model/model.go),
  `Probe`). It holds no member data.
- Its deadline is 10 seconds, or the configured timeout when that is shorter.
- A reply with status 200 passes. The reply's text is not read for meaning.
- `-register-commands` sends no probe, and without the model variables there is
  nothing to probe.

### A refusal stops the start, an absence does not

<!-- markdownlint-disable MD013 -->
| Class | What counts | At start |
| :--- | :--- | :--- |
| Rejected | Every status other than 200 that the next row does not name: 400, 401, 403 and 404 among them, and a redirect, which the client does not follow | The bot ends with exit code 1. The error names the status and the three variables to check |
| Unavailable | Status 408, 429 and 500 to 599; no reply at all, such as a refused connection or a deadline that passes, also while the reply's body is read | The bot starts. The model's state is "not ready" |
<!-- markdownlint-enable MD013 -->

A rejected probe means that waiting will not help: the configuration is wrong
and a person has to change it. An unavailable endpoint says nothing about the
configuration, so it must not take the bot down. Status 408 and 429 are in the
second row for that reason.

Check: `Classify` in [`internal/model`](../../internal/model/model.go) with
`TestClassify_Boundary_Table`; `TestRun_Negative_UnknownAliasStopsTheStart`,
`TestRun_Negative_RejectedProbeStopsTheStart` and
`TestRun_Positive_ProbeUnavailableAllowsStart` in
[`cmd/diallak`](../../cmd/diallak/main_test.go).

### A gate in front of the model

A gate holds one of two states and stands between the answer pipeline and the
endpoint ([`internal/model`](../../internal/model/gate.go), `Gate`).

<!-- markdownlint-disable MD013 -->
| State | A question | The state changes when |
| :--- | :--- | :--- |
| ready | goes to the model | a request ends as unavailable: the state becomes "not ready" |
| not ready | is not sent. The member gets the excerpts with the reason that the model is unavailable, as ADR-0003 says, without waiting | one question is sent as a trial once 30 seconds have passed since the last failure. If it succeeds the state becomes "ready"; if it fails the 30 seconds start again |
<!-- markdownlint-enable MD013 -->

- **One trial at a time.** Of several questions that arrive when a trial is
  due, exactly one is sent. The others get excerpts.
- **Only an absence closes the gate.** A rejected request, a reply that cannot
  be read and a member who is gone leave the state as it is. Each of them is
  one failed answer under ADR-0003.
- **One failure is enough.** A single request that ends as unavailable, a
  timeout included, closes the gate. The questions of the next 30 seconds then
  get their excerpts at once instead of each waiting for the deadline.
- **No request of the bot's own after the start.** The bot sends the endpoint
  the start probe and members' questions and nothing else. An endpoint that
  comes back is noticed with the next question after the wait, not before.
- The 10 seconds and the 30 seconds are chosen, not measured.

Check: `TestGate_Boundary_RetryInterval`,
`TestGate_Boundary_ConcurrentCompletesTrialDue`,
`TestGate_Positive_ReadyTransitions` and `TestGate_Positive_TrialOutcomes` in
[`internal/model`](../../internal/model/gate_test.go);
`TestBuild_Positive_PipelineWithGateNotReadyAnswersExcerpts` in
[`internal/app`](../../internal/app/app_test.go).

### The state has its own path on the health listener

`GET /modelz` answers 200 with one line: `model: not configured`,
`model: ready` or `model: not ready` ([`internal/health`](../../internal/health/health.go)).
The status line at start ends with the same three wordings.

- The path reads the gate. It sends nothing to the endpoint.
- `/healthz` and `/readyz` keep the meaning of
  [ADR-0007](0007-health-listener.md). A bot whose model is not ready still
  answers with excerpts, so a platform must not take it out of service for
  that. A deployment that wants to know reads `/modelz`.

Check: `TestHandler_Positive_IndependenceOfReadyzAndModelz` in
[`internal/health`](../../internal/health/health_test.go) and
`TestRun_Positive_HealthEndpointReportsTheModelState` in
[`cmd/diallak`](../../cmd/diallak/main_test.go).

### What is logged

One line for each change of state. A line for "not ready" carries the class of
the failure: the status code, a timeout or a transport failure. It never
carries text the endpoint sent, and a status reply's body is discarded unread.
A question that meets a closed gate writes the line of ADR-0003 as before.

## Consequences

- **Positive**: A wrong alias, URL or key fails at start with the names of the
  variables, not at the first question. A restart of the endpoint does not
  restart the bot. While the endpoint is away members get their excerpts
  without waiting for a deadline. A deployment can read the model's state.
- **Negative**: Each start costs one request with one output token. An
  endpoint that fails once is not asked again for 30 seconds, so a single slow
  reply turns the answers of that half minute into excerpts. A recovery shows
  on `/modelz` only after a member's next question. The health listener now
  says whether a model is configured.

## Alternatives considered

- **Stop the start on every failed probe.** A restart of the endpoint would
  then keep the bot from starting, although it can answer with excerpts.
- **Never stop the start.** A wrong alias would show at the first question, as
  before.
- **Let readiness follow the model.** The platform would take a bot out of
  service that still answers every question with cited excerpts.
- **Probe on a timer.** It would show a recovery sooner. It also sends requests
  nobody asked for to an endpoint that counts and may bill each one, and the
  next question finds the same fact.
- **Ask the endpoint for its list of models.** A list does not show that a
  request under the alias is routed, and a key that is limited to the
  completions path cannot ask for it.

## Supersedes

This record changes single statements. The records stand as they are
otherwise.

- [ADR-0007](0007-health-listener.md), "Two paths" and, under "Decided in this
  record", "the two paths and what they mean": the listener has three paths.
  The two keep their meaning.
- ADR-0007, "It reveals and records nothing": "answers with a fixed word. It
  gives no version and no configuration." `/modelz` answers with one of three
  fixed lines, and the line says whether a model is configured. It gives no
  alias, no address and no version.
- [ADR-0003](0003-answer-pipeline.md): "When the configured endpoint fails or
  times out, the reply shows the retrieved excerpts with their sources and says
  that the model is unavailable." This stands and now also holds while the gate
  is closed, when the request is not sent at all. "Is not retried" stands: a
  trial is a member's next question, never a repeat of one that failed.

## Open points

- **Not tried against a real endpoint.** Every test runs against a local test
  server. Which status a given router returns for an unknown alias was not
  measured here.
- **A router that answers an unknown alias with a 5xx status** would let the
  start pass with the model "not ready" and stay so. `/modelz` and the log line
  with the status code show it; the start does not stop.
- **The two durations** have no measurement behind them. A deployment's own
  numbers can change them in a later record.

## Decided in this record

The operator decided that a start probe which meets a client error, such as an
unknown alias or a refused key, stops the start, and that a server error or a
timeout lets the bot start with the model reported as not ready and nothing
falling back silently. The operator also decided that ADR-0003 holds while the
model is not ready: the question gets the cited excerpts with the stated
reason, the model is not called, the model's state is its own field in the
health output and the readiness path keeps its meaning.

The choices below were made here and are accepted with this record, open to
the operator's veto:

- Status 408 and 429 count as unavailable, a redirect and every other status
  that is not 200 as rejected.
- A reply whose body does not arrive in time counts as unavailable.
- The probe is one chat completion with one output token, with a deadline of
  10 seconds.
- The gate closes after one unavailable request and tries again with a
  member's question after 30 seconds. The bot sends no probe on a timer.
- The path is `/modelz`, always answers 200 and carries one of three lines.

## References

- Operator decision of 2026-10-11 on the start probe (paraphrased): a probe
  that fails with a client error stops the start; a server error or a timeout
  lets the bot start and the health output reports the model as not ready;
  wrong configuration fails loudly at start, a restart of the endpoint does
  not take the bot down, and nothing falls back silently. The alias is
  deployment configuration and this repository holds no default.
- Operator decision of 2026-10-11 on questions while the model is not ready
  (paraphrased): the accepted record on the answer pipeline holds; a question
  gets the cited excerpts with the stated reason and the model is not called;
  the model's state is its own field in the health output and the readiness
  path keeps its meaning.
