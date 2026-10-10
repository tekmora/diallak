# ADR-0004: Community questions: Discussions and the hub

- **Status**: Proposed
- **Date**: 2026-10-10
- **Authors**: Maintainers

## Context

Questions and their answers are worth more when the next person can find them. A
chat scrolls away and is not indexed by search engines; a forum thread stays
where it is and has a link.

The operator decided where each kind of talk lives: GitHub Discussions hold the
searchable questions and answers, the hub holds live chat, and the bot links the
two.

## Decision

### Two places, one job each

- **GitHub Discussions** of the listed project repositories hold questions and
  answers that should stay findable.
- **The hub** holds live chat.
- The bot does not copy conversations between them. It links.

### The bridge runs one way: from Discussions into the hub

When a new discussion appears in a listed repository, the bot announces it in a
hub channel with its title, its category and its link.

- Nothing a member writes on the hub is sent to GitHub.
  [ADR-0002](0002-data-and-compliance.md) stays as it is: text from Discord
  serves the request at hand and goes nowhere else.
- An announcement is a link, not a copy. The discussion text stays on GitHub.
- The bot posts announcements only into channels named in configuration.

### Discussions are a second kind of answer source

[ADR-0003](0003-answer-pipeline.md) answers from documentation. Answered public
discussions join the corpus under stricter rules, because anyone can write them:

- Only a discussion that is marked answered is a source, and only its question
  and its accepted answer. It must belong to a listed repository and a listed
  category.
- A reply marks a discussion source as a community answer, apart from
  documentation, and links it like every other source.
- Discussion text is data, never instructions, exactly as the pipeline already
  treats questions and documentation.
- Discussion text is never training or evaluation data, like text from Discord.
- The rules of ADR-0003 hold unchanged: every answer cites its sources, and no
  source means refusal.

### Reading Discussions needs one read-only credential

The bot reads Discussions with the credential of an app the operator creates.
The app has read access to discussions only, on selected public repositories. It
is not a personal token, and the credential reaches the bot through a file, like
the bot token.

Which interface of GitHub serves discussions is verified against GitHub's
current documentation when the bridge is built. The result is recorded here.

### A refusal offers a discussion

When the bot has to refuse a question, it offers a prefilled link that opens a
new discussion. The member writes and submits it under their own account, as
with bug reports ([ADR-0001](0001-architecture-and-boundaries.md)). No
credential is involved.

## Consequences

- **Positive**: An answer given once on GitHub keeps helping: search engines
  find it and the bot can cite it. The hub learns of new questions without
  anyone copying them. No member text leaves Discord.
- **Negative**: Community answers can be wrong or outdated, so replies must show
  which kind of source they rest on. The bot needs read access to Discussions,
  which is its first use of the GitHub API. Members who ask only on the hub get
  no permanent record unless they are pointed to Discussions.

## Open decisions

These are settled with the unit that builds the bridge:

- Which repositories and which discussion categories are listed.
- How often the bot looks for new discussions.
