# ADR-0028: Version 1 answers without a model

- **Status**: Accepted
- **Date**: 2026-11-19
- **Authors**: Maintainers
- **Supersedes for version 1**: [ADR-0003](0003-answer-pipeline.md),
  "Generation: an OpenAI-compatible endpoint from configuration";
  [ADR-0006](0006-ai-act-role.md), "What may go live, and when", for model
  mode; [ADR-0012](0012-model-state.md);
  [ADR-0013](0013-guardrail-policy-and-receipt.md);
  [ADR-0022](0022-embeddings-for-paraphrased-questions.md)

## Context

The answer pipeline of [ADR-0003](0003-answer-pipeline.md) has four stages:
corpus, retrieval, generation and answer assembly. Generation sends the
question and the retrieved chunks to a model endpoint. Four records build on
that stage:

- [ADR-0006](0006-ai-act-role.md) reads the project's role under the EU AI Act
  and keeps model mode off until its gates are passed;
- [ADR-0012](0012-model-state.md) adds a start probe, a gate and the health
  path `/modelz`;
- [ADR-0013](0013-guardrail-policy-and-receipt.md) requires a guardrail policy
  on every model call and gives the member a receipt;
- [ADR-0022](0022-embeddings-for-paraphrased-questions.md) adds an embedding
  endpoint for paraphrased questions.

All of it is built, and all of it is off unless a deployment sets the
`DIALLAK_MODEL_*` or `DIALLAK_EMBED_*` variables
([`internal/config`](../../internal/config/config.go)). No deployment has set
them; the live server has never seen a model answer
([README](../../README.md), "Run").

Without those variables the bot already answers with no model: `/ask` and the
support forum reply with cited excerpts from a lexical index, labelled "No AI
model wrote this reply" (`LabelExcerpts` in
[`internal/answer`](../../internal/answer/answer.go)). The receipt button is
attached only when a request reached a model or an embedding endpoint
(`receiptKeyFor` in
[`internal/discord/receipt.go`](../../internal/discord/receipt.go)), so a
deployment without the variables shows none.

Excerpts answer with the nearest passage of the documentation. A common
question, such as how to install a project or where to report a bug, has one
answer a maintainer would give, and the nearest passage is not always that
answer.

On 2026-11-19 the operator redirected the project (paraphrased): keep AI out of
the bot for now, and let it answer common questions by rules, as bots did
before language models.

## Decision

### Version 1 has no model path

Version 1 is every release up to and including 1.0.0
([product definition](../product.md)).

- No deployment of version 1 sets a `DIALLAK_MODEL_*` or `DIALLAK_EMBED_*`
  variable. The bot then sends no request to a model, a guardrail route or an
  embedding endpoint.
- The model and embedding code stays in the repository, off by default as
  today, with its tests. It is not deleted. Switching it on again needs a new
  record that supersedes this one.
- `/modelz` keeps answering `model: not configured`
  ([ADR-0012](0012-model-state.md)). It stays as a health path, and the
  receipt stays built and unused.

### Answers come from rules first

A question is answered in this order:

1. **A rule.** A trigger table, kept as data in this repository, names per
   project repository the keywords, slash commands, forum tags and
   documentation links that a common question carries. A question that
   matches a trigger gets that trigger's fixed answer.
2. **Excerpts.** A question that matches no trigger gets the cited excerpts of
   [ADR-0003](0003-answer-pipeline.md), as today.
3. **The refusal.** A question with neither gets the refusal of ADR-0003. In
   the support forum the bot posts nothing instead, as
   [ADR-0009](0009-support-forum-first-answer.md) decides.

Every rule-based answer keeps these properties:

- **Its text is documentation.** The answer is text from a project
  repository's documentation, stored with its source link and revision like a
  corpus chunk ([ADR-0003](0003-answer-pipeline.md), "Corpus: project
  documentation only"). The bot writes nothing of its own at run time.
- **It is labelled.** The reply says that it is automated and that no AI model
  wrote it. In the support forum the tag "Automated answer" stays the member's
  opt-in ([ADR-0020](0020-forum-answers-only-opted-in-posts.md)), and the reply
  ends with the forum footer of [ADR-0009](0009-support-forum-first-answer.md).
- **It offers the ticket path.** Every rule-based answer and every refusal
  names the `tickets` channel, where a member opens a private thread to the
  operator ([server definition](../../server/definition.json), channel
  `tickets`), beside the place where a person helps.
- **It is decided by the table, not by a score.** A trigger matches or does
  not. The same question and the same table give the same answer.
- **It is tested both ways.** Every trigger carries at least one question that
  must match it and one that must not, and a test replays both.

Excerpt answers also name the `tickets` channel.

The shape of the table, its validator and the matcher come with the change
that builds them. This record fixes the properties above.

### What stays as it is

- The corpus, the lexical retrieval and the excerpt answers of
  [ADR-0003](0003-answer-pipeline.md): they use no model, and excerpt answers
  gain the tickets sentence.
- The label and footer rules of [ADR-0002](0002-data-and-compliance.md),
  [ADR-0009](0009-support-forum-first-answer.md) and
  [ADR-0020](0020-forum-answers-only-opted-in-posts.md).
- The duties [ADR-0006](0006-ai-act-role.md) lists and its open point on
  whether excerpt mode is an AI system: the notices are given either way.

## Supersedes

For version 1 this record replaces single statements. The records stand as
they are otherwise and apply again only through a later record.

- [ADR-0003](0003-answer-pipeline.md), "Generation: an OpenAI-compatible
  endpoint from configuration", and the model parts of "Every answer cites its
  sources; no source means refusal": version 1 has no generation stage in use.
  "Retrieval only remains the mode when no endpoint is configured", under
  "Alternatives considered", is the version 1 mode, with the trigger table
  before it.
- [ADR-0006](0006-ai-act-role.md), "What may go live, and when": model mode
  stays off in version 1 whichever gates are passed.
- [ADR-0012](0012-model-state.md): the start probe and the gate do not run,
  because no model is configured.
- [ADR-0013](0013-guardrail-policy-and-receipt.md): the guardrail policy, the
  bot's own masking and the receipt are not used in version 1. Its gates are
  no longer a step toward version 1.
- [ADR-0022](0022-embeddings-for-paraphrased-questions.md): retrieval is
  keyword only in version 1.

## Consequences

- **Positive**: Version 1 needs no model endpoint, no gateway policy and no
  embedding service, and none of the duties that come with model mode. A
  common question gets the answer a maintainer chose, with its source. Every
  answer can be replayed from the table and the corpus.
- **Negative**: A question worded in a way no trigger and no excerpt covers
  gets the refusal. The table has to be kept next to the documentation it
  quotes; a moved heading or file breaks its link until the table is updated.
  The model code is kept and tested without being used.

## Alternatives considered

- **Delete the model and embedding code.** Rejected: the operator asked to keep
  it off and not to delete it, and its tests prove refusal paths a later
  record can rely on.
- **Keep model mode as a gated option in version 1.** Rejected by the
  operator's redirection of 2026-11-19.
- **Excerpts only, without a trigger table.** Rejected: excerpts give the
  nearest passage, not the answer a maintainer chose for a common question.

## Open points

- **A second defence.** A version 1 build refuses every model and embedding
  variable at start. The [start check](../../cmd/diallak/main.go) and its
  [tests](../../cmd/diallak/model_refusal_test.go) provide this defence.
- **Which projects the first table covers.** That is a change to the table,
  like the corpus manifest of [ADR-0003](0003-answer-pipeline.md).

## Decided in this record

The operator decided on 2026-11-19 (paraphrased) that version 1 carries no AI,
that common questions are answered by rules, and that the model code stays off
and is not deleted.

The choices below were made here and are accepted with this record, open to
the operator's veto:

- version 1 means every release up to and including 1.0.0;
- rules first, then excerpts, then the refusal;
- every rule-based answer and every refusal names the `tickets` channel;
- `/modelz` and the receipt stay built and unused.

## References

- [ADR-0003](0003-answer-pipeline.md), the answer pipeline.
- [ADR-0006](0006-ai-act-role.md), [ADR-0012](0012-model-state.md),
  [ADR-0013](0013-guardrail-policy-and-receipt.md),
  [ADR-0022](0022-embeddings-for-paraphrased-questions.md), the model path.
- [ADR-0009](0009-support-forum-first-answer.md),
  [ADR-0020](0020-forum-answers-only-opted-in-posts.md), the forum answer and
  its tag.
- [Product definition](../product.md), version 1 and its functions.
