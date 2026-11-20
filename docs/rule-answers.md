# Rule answers

Common questions can receive a fixed answer from project documentation. The
bot checks the trigger table first, then uses the existing excerpt pipeline,
then refuses when retrieval has no source. Rule answers do not call a model.

The embedded table lives at [`rules/triggers.json`](../rules/triggers.json).
Each trigger has these fields:

- `id`: unique lowercase slug, at most 64 characters.
- `repository`: repository name using the corpus format.
- `keywords`: `all`, `any`, and `none` lists. at least one of `all` or `any` must be non-empty. Each entry is a word or phrase.
- `commands`: registered slash command names, such as `ask`.
- `aliases`: product-name phrases; required for commands. A command question must contain one alias; a forum question uses its product tag instead.
- `forum_tags`: forum tag names that opt a post into this answer.
- `answer`: Markdown `path` and exact heading to quote.
- `links`: optional relative corpus paths shown as source links.
- `must_match` and `must_not_match`: replay examples required for validation.

Question and keyword text use the same tokenizer as retrieval. A phrase matches
when its resulting tokens occur next to each other. Every `all` phrase must be
present, at least one `any` phrase must be present when that list is non-empty,
and no `none` phrase may be present. The first matching trigger in table order
wins. Validation replays every positive example through each command and forum
tag and rejects ambiguity. It also replays every negative example. Each trigger must include a negative
example containing a keyword phrase and, for command triggers, an alias phrase.
The same tokenizer and phrase matching rules apply to this near-miss check.
A keyword beginning with `where is` must also appear in the question with those
words intact (case insensitive), so it cannot match a bare capability question.

To add an answer, find a project documentation section with a stable heading,
add one trigger to the JSON table, and include a positive and negative question
for each supported entry point. Add the matching support forum tag and command
to the table only when they already exist. The validator checks known names,
paths, headings, and examples at startup. A trigger whose repository is not in
the loaded corpus or whose answer section is missing is disabled and logged.
A missing link is omitted and logged.

Examples for each trigger are in `must_match` and `must_not_match`. Unit tests
also cover matching, non-matching, ambiguity, stale examples, malformed input,
unknown entry points, path resolution, and rendering length limits.

Support triggers use help-seeking phrases only. Sponsorship blocks are removed
from answers; a section containing only sponsorship disables its trigger and is
logged as `sponsorship_only`.
