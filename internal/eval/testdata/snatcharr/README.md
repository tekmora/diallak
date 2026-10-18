# SnatchArr evaluation fixture

This directory is test data for the answer pipeline
([ADR-0003](../../../../docs/adr/0003-answer-pipeline.md), "Evaluation").

- `corpus/` is an unmodified copy of the documentation of
  [lusoris/SnatchArr](https://github.com/lusoris/SnatchArr) at commit
  `9e70c2e03d22d535a9f89c628807aab5813829a6`: its `README.md` and every
  Markdown file under `docs/`. Upstream's `REUSE.toml` declares every Markdown
  file [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/),
  copyright 2026 lusoris; most files also say so in their own header. That
  licence, not this repository's, applies to `corpus/`. Its text is in
  [`LICENSES/CC-BY-SA-4.0.txt`](../../../../LICENSES/CC-BY-SA-4.0.txt), and
  [`REUSE.toml`](../../../../REUSE.toml) records it for every file here in
  machine-readable form.
- `cases.json` holds the questions. They were written by hand from that
  documentation, in the words a community member would use.

## Case kinds

| Kind | Meaning | Expected |
| :--- | :--- | :--- |
| `answerable` | The documentation answers it | A listed source is retrieved |
| `paraphrase` | Documented fact in other words | A listed source is retrieved; measured against the threshold |
| `off_topic` | Nothing to do with the project | Refusal |
| `undocumented` | About the project, but not in the docs | Measured only |

`sources` lists every file that answers the question; retrieving any one of
them counts.

## Paraphrase cases and the embeddings threshold

The `paraphrase` kind tests retrieval against questions that ask for documented
facts using everyday words and synonyms rather than the documentation's own
vocabulary (no file names, headings, or option names).

Two constants govern paraphrase cases:

- `MaxParaphraseOverlap = 0.5`: a paraphrase case shares at most half of its
  distinct terms with its sources.
- `MaxParaphraseMisses = 3`: a decision record on embeddings is due when more
  than this many paraphrase cases miss.

RECORD: hits 2/12, due true, date 2026-10-17

Hybrid retrieval (ADR-0022) measured with `TestSnatchArrEval_EmbedSweep` on
2026-10-17, model `hf.co/unsloth/embeddinggemma-2-GGUF:Q8_0` on a local Ollama
endpoint. The sweep runs only when `DIALLAK_EVAL_EMBED_BASE_URL` and
`DIALLAK_EVAL_EMBED_ALIAS` are set. `DIALLAK_EVAL_EMBED_KEY_FILE` names the
endpoint's key file, and `DIALLAK_EVAL_EMBED_REFUSED_ALIASES` lists models,
comma separated, that the endpoint must refuse: each must make the start-time
embedding step fail, or the sweep fails.
[`tools/embed_route_probe.py`](../../../../tools/embed_route_probe.py) runs the
sweep against a live route with the key on standard input and at least one
refused model.

| Similarity | Answerable hit | Off-topic refused | Undocumented refused | Paraphrase hit | Bars hold |
| ---: | ---: | ---: | ---: | ---: | :--- |
| 0.30 to 0.60 | 30/32 | 0/6 | 0/6 | 12/12 | no |
| 0.65 | 30/32 | 2/6 | 0/6 | 12/12 | no |
| 0.70 | 30/32 | 6/6 | 2/6 | 12/12 | yes |
| 0.75 | 29/32 | 6/6 | 5/6 | 9/12 | yes |
| 0.80 | 29/32 | 6/6 | 5/6 | 2/12 | yes |

`retrieval.DefaultMinSimilarity` is 0.75: the lowest value that keeps the
keyword-only undocumented refusals (5/6). Another embedding model needs its own
sweep and `DIALLAK_EMBED_MIN_SIMILARITY`.

The same sweep against the live on-premises route, alias
`cordana/auto-embedding` through the cluster's gateway, run by the cluster
maintainers with `tools/embed_route_probe.py` on 2026-10-18 (58 endpoint calls
in 66 s, about 53 a minute, no 429; the refused models `gemini-embedding` and
`cordana/qwen3-vl` were refused):

| Similarity | Answerable hit | Off-topic refused | Undocumented refused | Paraphrase hit | Bars hold |
| ---: | ---: | ---: | ---: | ---: | :--- |
| 0.30 | 29/32 | 1/6 | 1/6 | 12/12 | no |
| 0.35 to 0.40 | 29/32 | 4/6 | 1/6 | 12/12 | no |
| 0.45 | 29/32 | 5/6 | 1/6 | 12/12 | yes |
| 0.50 | 29/32 | 6/6 | 2/6 | 11/12 | yes |
| 0.55 | 29/32 | 6/6 | 4/6 | 10/12 | yes |
| 0.60 | 29/32 | 6/6 | 5/6 | 7/12 | yes |
| 0.65 | 29/32 | 6/6 | 5/6 | 4/12 | yes |
| 0.70 to 0.80 | 29/32 | 6/6 | 5/6 | 2/12 | yes |

On this route the rule above gives 0.60, not 0.75. The default is not changed
here; a deployment on this route sets `DIALLAK_EMBED_MIN_SIMILARITY`. How long
the route's first, cold batch takes was not measured: it stayed within the
sweep's 60 second timeout.

Do not edit a question to make a test pass. A failing case is a finding about
retrieval. To refresh the corpus, replace `corpus/` from a newer commit, update
the revision here and in `cases.json`, and review every case against the new
text.

