# ADR-0022: Embeddings for paraphrased questions

- **Status**: Proposed
- **Date**: 2026-10-17
- **Authors**: Maintainers
- **Amends**: [ADR-0003](0003-answer-pipeline.md), "Retrieval decides whether
  there is an answer"

## Context

[ADR-0003](0003-answer-pipeline.md) ("Retrieval decides whether there is an
answer") established the rule that retrieval starts with an in-memory lexical
index built at start, and that "Embeddings are added only when the evaluation
set below shows the need."

That need was evaluated under
[issue 124](https://github.com/tekmora/diallak/issues/124) and committed in
the evaluation suite
([`internal/eval`](../../internal/eval/testdata/snatcharr/README.md),
[`cases.json`](../../internal/eval/testdata/snatcharr/cases.json)). The suite
introduced 12 paraphrase cases (`KindParaphrase` in
[`internal/eval/eval.go`](../../internal/eval/eval.go)), testing whether
questions asking for documented facts in everyday words rather than the corpus
vocabulary retrieve the source chunks that answer them.

Two governance thresholds govern paraphrase evaluation:

- `MaxParaphraseOverlap = 0.5`: a paraphrase case shares at most half of its
  distinct tokens with its sources, enforced by validation during tests
  (source: chosen, not measured).
- `MaxParaphraseMisses = 3`: the maximum number of missed paraphrase cases
  tolerated before an architectural decision record on embeddings is due
  (source: chosen, not measured).

The baseline run
([`TestSnatchArrEval_Paraphrase`](../../internal/eval/eval_snatcharr_test.go))
measured 2 hits out of 12 cases, yielding 10 misses. Because 10 misses exceed
the threshold of 3, `EmbeddingsRecordDue` is true
([RECORD line](../../internal/eval/testdata/snatcharr/README.md):
`hits 2/12, due true, date 2026-10-17`).

The two cases that hit were `para-log-cleanup` ("Are past activity records
purged automatically after three months?") and `para-database-ssl-validation`
("How can I mandate complete TLS certificate verification against our internal
PKI?"). The 10 misses are questions that share only 1 to 3 stemmed terms with
the source documentation. The lexical index requires at least two distinct
matching query terms (`MinMatchedTerms = 2`) and sufficient BM25 coverage
(`DefaultMinScore = 0.20`)
([`internal/retrieval/retrieval.go`](../../internal/retrieval/retrieval.go)),
so phrasing that relies on synonyms or colloquial descriptions fails to
retrieve any source chunk.

## Decision

We propose hybrid retrieval, combining the existing in-memory lexical index
with semantic embeddings for documentation chunks.

### Embedding endpoint from configuration

The embedding model runs behind an HTTP endpoint that speaks the
OpenAI-compatible `/v1/embeddings` API, following the structure of the
chat generation endpoint
([`internal/model/model.go`](../../internal/model/model.go)):

- Configuration sets a base URL and an alias via proposed names
  `DIALLAK_EMBED_BASE_URL` and `DIALLAK_EMBED_ALIAS`.
- Code never contains a model name; the alias is whatever identifier the local
  endpoint routes on.
- When an endpoint requires authentication, the credential is read from a file
  named in configuration (for example, `DIALLAK_EMBED_KEY_FILE`), kept in
  memory only, and redacted in every log output.
- Every HTTP request has an explicit deadline and a bounded body.
- The client follows no redirect, preventing questions and credentials from
  leaving the configured destination.
- The first version does not retry failed requests.

### In-memory chunk vectors

- Corpus chunk embedding vectors are computed once at startup and stored in
  memory next to the lexical index.
- No vector database or external storage is used: the corpus covers a single
  project's documentation (a few hundred chunks), fitting entirely in memory
  within a few megabytes.

### Hybrid scoring and refusal

- A documentation chunk is retrieved as a source candidate when the lexical
  BM25 rule passes OR its cosine similarity to the question vector meets or
  exceeds a semantic threshold.
- The refusal rule of [ADR-0003](0003-answer-pipeline.md) is unchanged: when
  no chunk passes either the lexical or semantic criterion, there is no source,
  and the pipeline refuses before calling the generation model.

### Fallback and failure handling

- **Not configured**: When `DIALLAK_EMBED_BASE_URL` and `DIALLAK_EMBED_ALIAS`
  are not set, the pipeline operates in lexical-only mode, as today.
- **Configured but failing**: When the embedding endpoint fails or times out,
  retrieval falls back to lexical matching for that question. In accordance
  with ADR-0003's rule against silent fallbacks, the failure is counted and
  logged with its error class only (never logging question text or remote error
  bodies), and the reply does not hide the degradation: the reply indicates
  that semantic retrieval was unavailable, and the reply receipt records the
  event.

### Evaluation and threshold selection

- The semantic similarity threshold is determined via an evaluation sweep over
  the test set, mirroring the lexical `MinScore` sweep in
  [`TestSnatchArrEval_Sweep`](../../internal/eval/eval_snatcharr_test.go).
- The hybrid retriever must satisfy all existing acceptance bars: answerable
  hits (`>= 28/32`) and off-topic refusals (`>= 5/6`). The threshold is tuned
  to maximize paraphrase hits without degrading off-topic refusals.
- The selected threshold is committed and recorded in the evaluation suite.
- Continuous integration tests continue to run without network access, using
  a fake embedder implementation.

### Data and compliance

- Under [ADR-0002](0002-data-and-compliance.md), member text from Discord is
  sent only to operator-run endpoints that do not retain or train on the input.
- Sending question text to `/v1/embeddings` is an additional processing step
  within the same recipient class as the chat model (the operator's locally
  run endpoint).
- Upon acceptance of this record,
  [`docs/compliance/data-inventory.md`](../compliance/data-inventory.md) must
  add a corresponding row under "What the bot passes on" for the embedding
  endpoint.

## Consequences

- **Positive**: Members asking questions in everyday language and synonyms
  retrieve relevant documentation passages without needing corpus-specific
  keywords.
- **Positive**: Zero external vector database dependencies or persistent
  vector indexes to manage; all vectors remain in memory.
- **Positive**: CI remains offline, deterministic, and fast through a fake
  embedder.
- **Positive**: Deployments without an embedding endpoint retain full
  backwards compatibility with lexical retrieval.
- **Negative**: Increased startup latency while computing embeddings for all
  corpus chunks.
- **Negative**: The operator must configure and run an additional local
  endpoint (or gateway route) serving embeddings.
- **Negative**: Semantic matching risks false-positive retrieval on off-topic
  questions; this risk is actively guarded by the evaluation bar requiring
  at least 5 of 6 off-topic questions to be refused.

## Alternatives considered

- **A hand-kept synonym list for query expansion**: Query expansion requires
  no additional model or endpoint. However, maintaining synonyms manually is
  labor-intensive, fragile, and cannot cover the open-ended vocabulary of a
  community. It could complement, but cannot replace, semantic retrieval.
- **A vector database**: Not needed for a corpus of a few hundred documentation
  chunks. In-memory linear cosine similarity executes in microseconds and
  avoids operational complexity, additional storage, and failure modes.
- **A hosted embedding service**: Senders of member questions to third-party
  cloud providers violate the data privacy principles of
  [ADR-0002](0002-data-and-compliance.md).
- **Keep lexical only and accept the misses**: Rejected because 10 of 12
  measured paraphrase questions fail to retrieve any sources, leading to
  unnecessary refusals for valid member questions.

## Open for the operator

- **Accept or reject**: Whether to proceed with implementing hybrid retrieval
  and local embeddings.
- **Model selection**: Which local embedding model the alias routes to (for
  example, an open-weights embedding model such as `bge-small-en-v1.5`).
- **Misses threshold**: Whether `MaxParaphraseMisses = 3` remains the permanent
  threshold for future corpus evaluations.

## References

- [ADR-0002: Data and compliance](0002-data-and-compliance.md).
- [ADR-0003: Answer pipeline](0003-answer-pipeline.md).
- [ADR-0006: Role and duties under the EU AI Act](0006-ai-act-role.md).
- [Data inventory](../compliance/data-inventory.md).
- Issue 124: <https://github.com/tekmora/diallak/issues/124>.
- SnatchArr evaluation test data:
  [`internal/eval` README](../../internal/eval/testdata/snatcharr/README.md).
