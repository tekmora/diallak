# ADR-0013: A guardrail policy on every model call, and a receipt for the member

- **Status**: Accepted
- **Date**: 2026-10-11
- **Authors**: Maintainers

## Context

In model mode the bot sends a member's question to a model endpoint
([ADR-0003](0003-answer-pipeline.md)). A question can hold personal data: a
name, an e-mail address, a token pasted by mistake. Today the bot cannot tell
whether anything guarded that text on its way to the model, and the member has
no way to see what happened to it.

[ADR-0006](0006-ai-act-role.md) kept model mode off behind four gates. Two of
them were readings of legal texts and one was a list of gaps in the operator's
deployment. The operator replaced that list on 2026-10-11 (paraphrased): the
bot needs less legal apparatus and more openness. A guardrail layer belongs in
the path to the model, and the member is shown what was processed and what was
redacted.

The model endpoint of the operator's deployment is a LiteLLM gateway. Its
documentation, read on 2026-10-11, describes the things this record builds on:

- Policies that attach guardrails to requests, and two reply headers:
  `x-litellm-applied-policies` for the policies that matched the request and
  `x-litellm-applied-guardrails` for the guardrails that ran
  ([guardrail policies](https://docs.litellm.ai/docs/proxy/guardrails/guardrail_policies),
  "Response Headers").
- Masking of personal data, in which a found passage is replaced by a
  placeholder that names its kind, such as `<PERSON>` or `<EMAIL_ADDRESS>`
  ([PII masking](https://docs.litellm.ai/docs/proxy/guardrails/pii_masking_v2)).
  The gateway's built-in content filter writes a second form by default, such
  as `[EMAIL_REDACTED]`, and lets a deployment change it
  ([content filter](https://docs.litellm.ai/docs/proxy/guardrails/litellm_content_filter),
  "Customizing Redaction Tags").
- A route that applies one guardrail to a text and returns the changed text
  ([`/guardrails/apply_guardrail`](https://docs.litellm.ai/docs/apply_guardrail)).
  The request names the guardrail and holds the text; the reply holds
  `response_text` and nothing else.

The operator's deployment read and measured its installed gateway, LiteLLM
1.104.0, on 2026-10-11. Two of its results shape this record:

- Both headers were on a chat reply with status 200, also on a reply of one
  token. `x-litellm-applied-policies` was absent when no policy applied.
- The deployment's masking, as configured that day, missed most planted
  telephone numbers and street addresses and masked too much in German text.

Because of the second result it was decided for the deployment on the same day
(paraphrased): the bot's key reaches the chat route only, and the receipt names
the guardrails and policies from the reply headers. A receipt that shows "the
text as processed" while telephone numbers and addresses slip through would
tell a member that their data was masked when part of it was not. The route
that returns the masked text opens for the bot after the masking is remedied.

Nothing of this was measured from this repository. The bot assumes none of it:
the start checks what the bot depends on (see "The start proves the policy").

## Decision

There are two steps. Step one is built now and is what model mode needs to go
live. Step two is decided here and is built when the deployment opens the
guardrail route for the bot's key.

### A reply must name the policy

The chat completion is sent with a key to which the deployment attaches a
guardrail policy. The policy's guardrails run inside the gateway, on the
request, before a model sees it; masking of personal data is one of them. The
bot does not see what they changed.

The gateway names the policies it applied in the reply header
`x-litellm-applied-policies`, as names separated by commas. The bot reads that
header on every chat reply and does not depend on the order of the names. A
reply whose header is absent, or does not hold the policy named in
configuration, is discarded: nothing of the model's text is shown, and the
member gets the excerpts with the notice that the model is unavailable, as
ADR-0003 says for a failed endpoint.

After such a reply the bot sends no further question to the model until it is
restarted. The state of [ADR-0012](0012-model-state.md) becomes "not ready"
with a class of its own, and no trial is made: a key that lost its policy is a
fault a person has to repair, and a trial would send one more member's text
unguarded.

The check costs one header read. It is there because the bot cannot see the
deployment's configuration: a key that lost its policy would otherwise send
text unguarded without any sign.

### One name in configuration, and no start without it

Model mode needs one more variable beside the three of ADR-0003:

- `DIALLAK_MODEL_POLICY`: the policy every chat reply must name.

With the model endpoint configured and this variable missing, the bot does not
start. This repository holds no default for it, as for the alias.

### The start proves the policy

ADR-0012 sends one chat completion at start. Its reply must name the policy. A
reply with status 200 that does not name it stops the start, as a refused key
does.

The start therefore proves, against the real deployment, that the policy is
attached to the bot's key. It proves nothing about what the policy's guardrails
find.

### The member can ask for a receipt

A reply for which text left the bot carries one button. That holds for a
model-written answer and for an excerpt reply after a failed model call. A
reply in excerpt mode has no button, because nothing was sent anywhere.

Pressing the button shows a receipt to that member only. It holds:

- the guardrails that ran on the request, as the gateway named them in
  `x-litellm-applied-guardrails`, sorted by name;
- the policies that applied, or "none" when the header was absent, together
  with the sentence that the model's answer was discarded for that reason;
- the sentence that these guardrails can mask or change the text before the
  model, that they do not find everything, and that the bot does not see their
  result;
- where the text went: the question with documentation passages to the
  operator's gateway, and from there to the model behind it;
- how long anything is kept: by the bot, by the gateway and by the model
  endpoint, in the words of the
  [privacy notice](../compliance/privacy-notice.md);
- the notice that the receipt is automated.

When no reply came back, the receipt says that the question was sent and that
the bot does not know which guardrails ran.

The receipt does not say that the member's data was masked. The bot cannot know
it in step one.

The receipt is built from what the bot holds in memory. No request is made for
it, so it answers inside the three seconds Discord gives a first response
([`discord/discord-api-docs`](https://github.com/discord/discord-api-docs/tree/37148620d2324292b76047541d5f8331f81ea77a)
at commit `37148620d232`:
`developers/interactions/receiving-and-responding.mdx`).

### A direct message only when the member asks for one

The receipt carries a second button that sends the same text as a direct
message. The bot opens a direct message for no other reason and never on its
own. When Discord refuses the message because the member's direct messages are
closed (error 50007, "Cannot send messages to this user", in
`developers/topics/opcodes-and-status-codes.mdx` of the same commit), the bot
says so in the channel, visible to that member only, and the receipt stays
where it is.

The operator's direction named a switch from "sent to me" to "on request".
Every receipt is on request here, so there is nothing to switch and no choice
to store per member.

### What the bot holds, and for how long

- The bot holds, per reply with a button: the names of the guardrails and of
  the policies, the time, and whether the model's answer was shown. It holds
  no text of the member.
- It holds them in memory for 15 minutes and for a fixed number of replies at
  most; the oldest goes first. Nothing is written to disk, and a restart
  empties it.
- The entry's key is a random value carried in the button. It is not derived
  from the member, and no member id is held with it. The reply is visible to
  the member who asked only, so only that member can press its button.
- For a direct message the bot uses the member id that Discord sends with the
  button press, for that one request.

After 15 minutes, and after a restart, the button answers that the receipt is
no longer held and says why.

### Step two: the bot's own masking request

Step two starts when the deployment gives the bot's key the guardrail route and
a second variable is set:

- `DIALLAK_MODEL_GUARDRAIL`: the guardrail the masking request names.

With that variable unset the bot works as in step one. With it set:

1. The bot sends the question to the guardrail route first and builds the
   prompt from the masked text that comes back. The question as written goes
   to that route and no further. Retrieval runs on the question as written,
   inside the bot. The masking request has its own deadline of 5 seconds inside
   the deadline of the whole question.
2. Nothing falls back to an unmasked request:

   | Case | The model is called | The gate of ADR-0012 |
   | :--- | :--- | :--- |
   | The masking request times out or meets a server error | No | Closes |
   | The masking request is refused, or its reply holds no text | No | Stays |

3. The start sends a fixed text to the guardrail route before the chat probe.
   The text holds a made-up e-mail address on a domain reserved for
   documentation. A reply that still holds the address stops the start, and so
   does a refusal. A server error or a timeout leaves the model "not ready",
   and the next trial starts with the same fixed text, never with a member's
   question.
4. The receipt gains the text as the bot sent it on for the model, and the
   kinds of masked data with how many of each. The kinds are counted from the
   placeholders in both default forms, `<KIND>` and `[KIND_REDACTED]`. For
   each kind the bot counts the placeholders in the masked text and subtracts
   those the member's own text already held, so a question that quotes
   `<PERSON>` is not counted as a masked name. When the text was changed and
   no placeholder of either form is found, the receipt says that the text was
   changed and that the kinds are not known.
5. The receipt still says what was replaced and never that the text holds no
   personal data. A passage the guardrail did not find stands in the text
   shown, and the member can see it there.
6. The bot then holds the masked text with the receipt's other entries, for
   the same 15 minutes. It does not hold the question as written. This is the
   one change to "message content stays in memory for one request"
   ([ADR-0002](0002-data-and-compliance.md)), and it comes with step two only.
7. The bot puts nothing back. An answer can hold a placeholder where the model
   repeats one.

The gateway's own guardrails still run on the chat request in step two. They
see the whole prompt, the documentation passages included, and can change it
again; the receipt keeps the sentence that says so.

### What may go live, and when

This replaces the four gates of ADR-0006. Model mode may go live when:

1. the policy check, at the start and on every reply, and the receipt of step
   one are in the code;
2. each has its planted case in the test suite: a chat reply without the
   policy shows no model text and no further question is sent; a start without
   the policy's name stops; a start whose probe reply does not name the policy
   stops; a receipt shows the names from its reply's headers, and "none" for a
   discarded reply; an expired receipt says that nothing is held;
3. the privacy notice, the [data inventory](../compliance/data-inventory.md)
   and the [AI disclosure](../compliance/ai-disclosure.md) say what the code
   does, changed in the same pull request as the code. None of them says that
   personal data is masked without saying that the masking does not find
   everything.

Step two goes live with planted cases of its own: a question with a name and
an e-mail address reaches the test model masked, and the receipt shows the same
text and counts; a start whose planted address comes back unmasked stops; a
masking request that fails calls no model.

The deployment has gates of its own, which this repository cannot prove: that
its policy is attached to the bot's key, which the start shows, and how much
its masking finds, which it shows with planted cases on its side.

Excerpt mode is not touched. No text leaves the bot there.

## Consequences

- **Positive**: No model text is shown that the deployment's policy did not
  guard, and a deployment that lost its policy is found at the next start or
  the next reply, not by an audit. The member can see which guardrails ran on
  their own question. The gates are things a test can fail. Step one adds no
  request and holds no member text.
- **Negative**: In step one the member sees names of guardrails, not what they
  did; the bot cannot show more than it knows. Model mode now needs a gateway
  with policies and these headers; an endpoint that only speaks the chat
  protocol cannot be used. The masking is the gateway's: what it misses, the
  model sees. Step two costs a second request for every question and holds
  masked text for up to 15 minutes.

## Alternatives considered

- **The masked text in the receipt from the first day.** The member would see
  what slipped through. With the masking as measured on 2026-10-11 the receipt
  would also present text as processed that was masked only in part. Deferred
  to step two, by the decision named under "Context".
- **Only the policy on the key, for good.** One request and no route to open.
  The bot would never see the masked text and could never show it. Rejected as
  the end state; it is step one.
- **Masking inside the bot.** No dependency on the gateway. It would be a
  second, weaker implementation of what the gateway already does, and the
  operator asked for the guardrail layer in the model path. Rejected.
- **The receipt as a direct message by default.** The bot would open a private
  channel that the member did not ask for, and a member can close direct
  messages, which Discord then refuses. Rejected; the direct message stays
  the member's own choice.
- **A stored receipt that outlives a restart.** It would need a store keyed by
  something that finds the member's reply again. Rejected.
- **A trial after a reply without the policy**, as ADR-0012 makes after an
  absence. The trial is a member's question, and it would go out unguarded
  when the fault is still there. Rejected; a restart ends the state.

## Supersedes

This record changes single statements. The records stand as they are
otherwise.

- [ADR-0006](0006-ai-act-role.md), "What may go live, and when": the list of
  four gates for model mode. The list in this record takes its place. Of the
  old gates, the notices are in the code; reading the codes of practice is no
  gate any more and stays an open point of ADR-0006; the privacy notice has to
  say what the code does; and the three gaps of the operator's endpoint are
  the deployment's to close and to report. This record does not say that they
  are closed. The duties ADR-0006 lists stay as they are.
- [ADR-0003](0003-answer-pipeline.md), "When the configured endpoint fails or
  times out, the reply shows the retrieved excerpts": this now also holds for
  a reply that does not name the policy.
- [ADR-0012](0012-model-state.md), "One request at start": the start also
  reads the policy header from the probe's reply. "Only an absence closes the
  gate": a reply without the policy closes it too, without a trial.
- [ADR-0002](0002-data-and-compliance.md), "message content stays in memory
  for one request": unchanged in step one. In step two the masked text of a
  question stays in memory for up to 15 minutes. "It is sent to one place
  only: the model endpoint named in configuration" stands in both steps.

## Open points

- **Not tried from this repository.** What this record says about the gateway
  comes from its documentation and from the report of the operator's
  deployment of 2026-10-11 on LiteLLM 1.104.0, which this repository cannot
  replay. The report, from the installed source and three chat requests of
  one token without streaming:
  - `x-litellm-applied-guardrails` was on every reply with status 200, as
    names separated by commas without spaces, in an order that changed
    between requests;
  - `x-litellm-applied-policies` was there only when a policy applied, also on
    a reply of one token;
  - the guardrail route exists and takes an ordinary key;
  - a masked reply of the model is not unmasked in that deployment.
- **Not measured there either**: a key that is limited to named routes, a
  policy attached to a key (the measurement named the policy in the request),
  a request that a guardrail refuses, and a streamed reply. The bot does not
  stream. The first measurement with the bot's own key is the deployment's.
- **How much the deployment's masking finds** is named under "Context". It is
  the reason the receipt claims nothing about masked data, and its remedy is
  the condition for step two.
- **Guardrails on the whole prompt** also read the documentation passages.
  Whether masking there costs the answer a name it needed was not measured.
- **Attaching a policy to a key** needs the gateway's Enterprise licence
  according to the policies page. The deployment reported that this holds for
  a policy written into a key's metadata, and that a policy attached in the
  gateway's configuration file passes no licence check in 1.104.0. How the
  deployment attaches its policy is not this repository's matter; the bot sees
  the header or it does not.
- **No policy template for the EU AI Act** was found in the gateway's
  [policy templates](https://docs.litellm.ai/docs/proxy/guardrails/policy_templates)
  on 2026-10-11; the page lists two templates for personal data. Which checks
  the deployment's policy holds is decided there.
- **The names of the deployment's guardrails and policies become visible** to
  a member who asks for a receipt, because the receipt repeats the headers.
- **The first answer in the support forum**
  ([ADR-0009](0009-support-forum-first-answer.md)) is not built. Its reply
  is public, so who may open its receipt is decided with that unit.
- **The two limits**, 15 minutes and the number of replies held, have no
  measurement behind them.

## Decided in this record

The operator decided that a guardrail layer stands in the path to the model,
that personal data is masked before the model, that a reply without the policy
is refused, that the member is shown what was processed and what was redacted,
and that this, proven by planted cases, replaces the gate list of ADR-0006.
The operator also decided that legal research is no gate for model mode.

The order of the two steps was decided for the deployment on 2026-10-11 and is
open to the operator's veto: the receipt from the reply headers first, the
masked text after the masking is remedied.

The choices below were made here and are accepted with this record, open to
the operator's veto as well:

- That a reply without the policy ends model mode until a restart.
- The receipt on a button, shown to the member only, with a direct message as
  a second button.
- 15 minutes in memory, a fixed number of replies, a random key and no member
  id.
- That the receipt claims nothing about what was not found.
- For step two: the bot's own masking request before the chat request, its
  deadline of 5 seconds, the planted e-mail address at every start, the
  counting rule, and that the bot puts no masked passage back into an answer.
- The two variable names.

## References

- Operator direction of 2026-10-11 on guardrails and openness (paraphrased):
  less legal apparatus; a guardrail layer with an EU AI Act profile in the
  model path; openness toward the member, who is shown what was processed and
  what was redacted, how long anything is kept and where the text went; a
  member whose direct messages are closed gets a short notice visible only to
  them; legal research is no gate any more, and the privacy notice and the
  rules stay short, honest texts.
- Decision of 2026-10-11 for the deployment on the bot's key (paraphrased):
  the key stays limited to the chat route; the receipt names guardrails and
  policies from the reply headers; the route that returns masked text is added
  after the masking is remedied.
- Issue [#63](https://github.com/tekmora/diallak/issues/63), which holds the
  work that follows this record.
- LiteLLM documentation, read on 2026-10-11: the five pages linked above.
- Report of the operator's deployment of 2026-10-11 on the installed gateway
  (LiteLLM 1.104.0). Its evidence is held with the deployment, outside this
  repository.
- Discord developer documentation, repository `discord/discord-api-docs` at
  commit `37148620d232` of 2026-10-09:
  `developers/interactions/receiving-and-responding.mdx` (a first response
  within 3 seconds), `developers/topics/opcodes-and-status-codes.mdx` (error
  50007), `developers/components/reference.mdx` (a button's `custom_id`).
