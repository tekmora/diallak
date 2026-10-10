# SnatchArr evaluation fixture

This directory is test data for the answer pipeline
([ADR-0003](../../../../docs/adr/0003-answer-pipeline.md), "Evaluation").

- `corpus/` is an unmodified copy of the documentation of
  [lusoris/SnatchArr](https://github.com/lusoris/SnatchArr) at commit
  `9e70c2e03d22d535a9f89c628807aab5813829a6`: its `README.md` and every
  Markdown file under `docs/`. Upstream's `REUSE.toml` declares every Markdown
  file [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/),
  copyright 2026 lusoris; most files also say so in their own header. That
  licence, not this repository's, applies to `corpus/`.
- `cases.json` holds the questions. They were written by hand from that
  documentation, in the words a community member would use.

## Case kinds

| Kind | Meaning | Expected |
| :--- | :--- | :--- |
| `answerable` | The documentation answers it | A listed source is retrieved |
| `off_topic` | Nothing to do with the project | Refusal |
| `undocumented` | About the project, but not in the docs | Measured only |

`sources` lists every file that answers the question; retrieving any one of
them counts.

Do not edit a question to make a test pass. A failing case is a finding about
retrieval. To refresh the corpus, replace `corpus/` from a newer commit, update
the revision here and in `cases.json`, and review every case against the new
text.
