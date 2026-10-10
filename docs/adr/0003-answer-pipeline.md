# ADR-0003: Answer pipeline

- **Status**: Accepted
- **Date**: 2026-10-10
- **Authors**: Maintainers

## Context

Members ask questions about the projects. The bot should answer from the
projects' documentation, with a locally run model, and a member must be able to
check every answer against its source.

Three risks drive the design:

- **An answer without a source.** A model can produce fluent text that no
  document supports.
- **A model name baked into code.** Models change often. A fixed name turns
  every change into a code change and hides which model actually answered.
- **Slow generation.** A local model can take many seconds. Discord's
  interaction token is valid for 15 minutes and expects a prompt first response,
  so the bot must acknowledge first and answer after.

## Decision

### Four stages, each behind an interface

```text
docs corpus -> retrieval -> generation -> answer assembly
```

Each stage is an interface with a fake for tests. The pipeline package does not
import the Discord library ([ADR-0001](0001-architecture-and-boundaries.md)).

### Corpus: project documentation only

- The bot reads the corpus from a local directory, named by
  `DIALLAK_CORPUS_DIR`. The manifest in that directory lists the sources: each
  entry names a directory, a repository, a revision and path patterns
  ([`internal/corpus`](../../internal/corpus/manifest.go)).
- The bot holds no code and no credential that fetches documentation. Filling
  the directory is the deployment's job.
- Sources are public documentation: a project's README and the Markdown files
  under its `docs/` directory. Answered public discussions are a second kind of
  source under [ADR-0004](0004-community-questions.md). Issues and private
  repositories are never sources.
- Every chunk keeps its source link, its heading and the revision it was read
  from.
- The corpus never contains text from Discord
  ([ADR-0002](0002-data-and-compliance.md)).

The corpus starts with one project. A second project is added once the
evaluation set for the first exists. Which projects, and in which order, is the
operator's choice and a change to the manifest only.

### Retrieval decides whether there is an answer

Retrieval returns a bounded number of chunks with scores. When no chunk passes
the threshold, there is no source, and the pipeline refuses before it calls the
model.

A chunk is a source only when it shares at least two words with the question and
covers enough of it. One overlapping word is not a source
([`internal/retrieval`](../../internal/retrieval/retrieval.go)).

The first slice uses a lexical index held in memory and built at start. It needs
no vector store and no embedding model. Embeddings are added only when the
evaluation set below shows the need.

### Generation: an OpenAI-compatible endpoint from configuration

The model runs behind an HTTP endpoint that speaks the OpenAI-compatible chat
API.

- Configuration gives a **base URL** (`DIALLAK_MODEL_BASE_URL`) and a
  **model alias** (`DIALLAK_MODEL_ALIAS`). The alias is whatever name the
  endpoint routes on. Code never contains a model name.
- When the two are not set, the bot answers without a model: the reply shows the
  best excerpts with their sources and says that no model wrote it. The refusal
  rule below still holds.
- A credential, when the endpoint needs one, comes from a file, like the bot
  token.
- Every request has a deadline, a bounded prompt (the retrieved chunks only) and
  a bounded output length. The first version does not retry.
- The client follows no redirect, so the prompt and the credential reach the
  configured endpoint only ([`internal/model`](../../internal/model/model.go)).

### Every answer cites its sources; no source means refusal

- The model receives the retrieved chunks with ids and must cite the ids it
  used.
- Answer assembly maps each cited id back to its source link. An id that was not
  in the request is invalid.
- An answer with no valid citation is discarded. The member gets a refusal
  instead: the bot says it found no documented answer and points to human
  support.
- When the model says that the sources do not answer the question, the reply is
  a refusal too.
- When the configured endpoint fails or times out, the reply shows the retrieved
  excerpts with their sources and says that the model is unavailable. The
  failure is counted and logged with its class only, never with the question or
  the endpoint's error text.
- The pipeline never falls back to an uncited answer or to another model.

Answer assembly also adds the AI label of ADR-0002 and names the revision of
every source it lists, so a reply stays traceable to the text it was written
from ([`internal/answer`](../../internal/answer/answer.go)).

### Acknowledge first, answer after

The bot acknowledges the command at once with a deferred response and edits the
answer in when the pipeline finishes. The pipeline's total deadline stays below
the lifetime of the interaction token.

In the first version the reply is visible only to the member who asked, so a
wrong answer is not broadcast. A way to share a reply with the channel comes
later.

### Evaluation

A fixed set of questions written from the documentation, each with its expected
sources, measures two things: whether citations are valid and whether the
pipeline refuses when it should.

- In CI the set runs with a fake model and needs no network. The set for the
  first project and the bar it is held to are in
  [`internal/eval`](../../internal/eval/testdata/snatcharr/README.md).
- A run against a real endpoint records the base URL and the alias that
  answered, so a result is never attributed to a model that did not produce it.

## Consequences

- **Positive**: A member can check every answer. A wrong model answer without
  support is dropped instead of shown. Changing the model is a configuration
  change. CI tests the contract without a model.
- **Negative**: The bot refuses questions the documentation does not cover, even
  when the model would have known the answer. Answer quality is bounded by
  documentation quality and by retrieval. A small local model may cite poorly
  and so refuse often; the evaluation set makes that visible.

## Alternatives considered

- **Retrieval only.** Returning the best documentation excerpts without
  generated text is cheaper and cannot invent content. The first version
  generates answers by operator decision, and the refusal path keeps the same
  safety property. Retrieval only remains the mode when no endpoint is
  configured.
- **A fixed model.** Rejected: it couples code to one model and hides
  substitutions.
- **Answers without citations, with a disclaimer.** Rejected: a disclaimer does
  not let a member check the answer.

## Decided in this record

The operator decided that the bot answers from project documentation with a
locally run model, that every answer cites its sources and carries the AI
label, and that answered discussions are a second kind of source. These follow
from recorded decisions: public documentation only, one project first, a
lexical index, the corpus as a local directory with a manifest, a model alias
instead of a model name, excerpts when no endpoint is set, and replies visible
to the asker only.

The choices below have no decision of their own outside this file. They were
made here and are accepted with this record:

- Four stages, each behind an interface with a fake for tests.
- The pipeline refuses before it calls the model when no chunk passes. A chunk
  is a source only when it shares at least two words with the question and
  covers enough of it.
- Embeddings are added only when the evaluation set shows the need.
- A request to the model has a deadline, a bounded prompt and a bounded output,
  is not retried and follows no redirect.
- An answer with no valid citation is discarded, and the model's own statement
  that the sources do not answer is a refusal.
- A failing endpoint yields excerpts with the reason stated. The pipeline never
  falls back to an uncited answer or to another model.
- The bot acknowledges first and answers after, within the lifetime of the
  interaction token.
- The evaluation set is written from the documentation and runs in CI with a
  fake model.

## References

- Discord, receiving and responding to interactions (token lifetime, deferred
  responses):
  <https://docs.discord.com/developers/interactions/receiving-and-responding>
