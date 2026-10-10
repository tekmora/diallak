# ADR-0003: Answer pipeline

- **Status**: Proposed
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

- The corpus is built from sources listed in configuration. Each entry names a
  repository, a path pattern and a revision.
- Sources are public documentation only: a project's README and the Markdown
  files under its `docs/` directory. Issues, discussions and private
  repositories are never sources.
- Every chunk keeps its source link, its heading and the revision it was read
  from.
- The corpus never contains text from Discord
  ([ADR-0002](0002-data-and-compliance.md)).

The corpus starts with one project. A second project is added once the
evaluation set for the first exists. Which projects, and in which order, is the
operator's choice and a change to the configuration list only.

### Retrieval decides whether there is an answer

Retrieval returns a bounded number of chunks with scores. When no chunk passes
the threshold, there is no source, and the pipeline refuses before it calls the
model.

The first slice uses a lexical index held in memory and built at start. It needs
no vector store and no embedding model. Embeddings are added only when the
evaluation set below shows the need.

### Generation: an OpenAI-compatible endpoint from configuration

The model runs behind an HTTP endpoint that speaks the OpenAI-compatible chat
API.

- Configuration gives a **base URL** (`DISCORD_BOT_MODEL_BASE_URL`) and a
  **model alias** (`DISCORD_BOT_MODEL_ALIAS`). The alias is whatever name the
  endpoint routes on. Code never contains a model name.
- When the two are not set, the bot answers without a model: the reply shows the
  best excerpts with their sources and says that no model wrote it. The refusal
  rule below still holds.
- A credential, when the endpoint needs one, comes from a file, like the bot
  token.
- Every request has a deadline, a bounded prompt (the retrieved chunks only) and
  a bounded output length. Retries are bounded.

### Every answer cites its sources; no source means refusal

- The model receives the retrieved chunks with ids and must cite the ids it
  used.
- Answer assembly maps each cited id back to its source link. An id that was not
  in the request is invalid.
- An answer with no valid citation is discarded. The member gets a refusal
  instead: the bot says it found no documented answer and points to human
  support.
- An endpoint error or a timeout produces a failure message. The pipeline never
  falls back to an uncited answer or to another model without saying so.

Answer assembly also adds the AI label of ADR-0002.

### Acknowledge first, answer after

The bot acknowledges the command at once with a deferred response and edits the
answer in when the pipeline finishes. The pipeline's total deadline stays below
the lifetime of the interaction token.

### Evaluation

A fixed set of questions written from the documentation, each with its expected
sources, measures two things: whether citations are valid and whether the
pipeline refuses when it should.

- In CI the set runs with a fake model and needs no network.
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

## References

- Discord, receiving and responding to interactions (token lifetime, deferred
  responses):
  <https://docs.discord.com/developers/interactions/receiving-and-responding>
