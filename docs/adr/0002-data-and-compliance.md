# ADR-0002: Data and compliance

- **Status**: Accepted
- **Date**: 2026-10-10
- **Authors**: Maintainers

## Context

The bot reads what members write to it and answers with text a model generated.
That puts it under three sets of rules:

- **Discord's developer terms and policy**, which bind every application that
  uses the API.
- **The GDPR**, because the community includes people in the EU.
- **The EU AI Act's transparency duty** (Article 50), because members interact
  with an AI system.

This record fixes the engineering rules that follow. It is not legal advice. The
public texts (privacy notice, server rules, AI disclosure) are separate
documents under [`docs/compliance`](../compliance/README.md).

## Decision

### Text from Discord is never training data

Text from Discord is used to answer the request at hand, and for nothing else.

- It is never used to train or fine-tune a model.
- It is never collected into an evaluation set. Evaluation questions are written
  from the documentation ([ADR-0003](0003-answer-pipeline.md)).
- It is sent to one place only: the model endpoint named in configuration. That
  endpoint must not keep or train on what it receives.

### Store as little as possible

By default the bot keeps message content in memory only for the duration of one
request.

- A stored record holds the identifiers, timestamps and state a feature needs,
  and no message text unless the feature cannot work without it.
- Logs never contain message content or the bot token.
- The [data inventory](../compliance/data-inventory.md) lists every stored
  field with its purpose and retention period. A change that stores a new field
  adds its inventory entry in the same pull request.
- The first version requests no privileged gateway intent
  ([ADR-0001](0001-architecture-and-boundaries.md)) and subscribes to no message
  events. It sees only what a member passes to a command.

### Retention and deletion

- Every class of stored record has a retention period. The period is
  configuration with a documented default, and a scheduled job deletes records
  past it.
- A member can ask the bot for a copy of their data and for its deletion. The
  member is identified by the user id of the interaction, never by a name they
  type.
- When the bot is retired or removed from the server, its stored data is
  deleted.

The retention periods themselves are decided together with the storage choice.
That choice is open (ADR-0001).

### Every AI answer is labelled

Every message that contains model-generated text says so in the message body.

- The label is plain text. It does not rely on an emoji, a colour or the bot's
  profile alone.
- It appears on each answer, so a member sees it at their first interaction
  whatever that is.
- It says the answer can be wrong, and the answer shows its sources (ADR-0003)
  and a way to reach a person.
- The description of the command says it before first use, and the last line
  of every reply says it again and names where a person helps. The name of that
  place comes from configuration.
- The bot's profile also states that it is automated.

The wording of each notice and whether it is in the code yet are in the [AI
disclosure](../compliance/ai-disclosure.md).

This implements Article 50(1) and 50(5) of Regulation (EU) 2024/1689: a person
who interacts with an AI system is informed of that, clearly and at the latest
at the first interaction. Article 50 has applied since 2 August 2026 (Article
113).

### The role assessment comes before launch

Before auto-answers are switched on in the live server, a written assessment
records:

- whether the project acts as provider or as deployer of the AI system under the
  AI Act;
- whether the marking duty for generated text (Article 50(2)) applies, and how
  it is met if so;
- the current state of amendments and Commission guidance, re-read from EUR-Lex.

The assessment is [ADR-0006](0006-ai-act-role.md). It is accepted with gates:
model-written answers stay off in the live server until every gate it lists is
passed. Excerpt replies, which no model writes, may go live after the live test.

## Consequences

- **Positive**: Members can see what the bot keeps and for how long, and can
  have it deleted. The no-training rule holds by construction, because member
  text has one destination and no store. The label does not depend on a later
  legal reading: it is always present.
- **Negative**: Member conversations cannot be used to improve answers; quality
  work depends on the documentation and a hand-written evaluation set. Each new
  stored field costs an inventory entry. Launch of auto-answers waits for the
  role assessment.

## Decided in this record

The operator decided that text from Discord is never training data, that the
bot says it is automated at first contact and on every reply, that a written
role assessment comes before model-written answers go live, and that excerpt
replies may go live before its gates are passed.

The choices below have no decision of their own outside this file. They were
made here and are accepted with this record:

- Text from Discord is never collected into an evaluation set.
- Member text has one destination, the configured model endpoint, which must
  not keep or train on it.
- Message content stays in memory for one request. A stored record holds
  identifiers, timestamps and state, and no message text unless the feature
  cannot work without it.
- Logs never contain message content or the bot token.
- Every stored field has an entry in the data inventory, added in the same pull
  request.
- Every class of stored record has a retention period that is configuration,
  and a scheduled job deletes records past it.
- A member can ask for a copy of their data and for its deletion, identified by
  the user id of the interaction.
- Stored data is deleted when the bot is retired or removed from the server.
- The label is plain text and says that the answer can be wrong.

## References

- Regulation (EU) 2024/1689 (AI Act), Articles 50 and 113:
  <https://eur-lex.europa.eu/eli/reg/2024/1689/oj>
- Regulation (EU) 2016/679 (GDPR), Articles 5, 13, 14 and 17:
  <https://eur-lex.europa.eu/eli/reg/2016/679/oj>
- Discord Developer Policy:
  <https://support-dev.discord.com/hc/en-us/articles/8563934450327>
- Discord Developer Terms of Service:
  <https://support-dev.discord.com/hc/en-us/articles/8562894815383>

The Discord pages are re-read from the primary source before launch; the clauses
that apply are listed in the role assessment.
