# ADR-0006: Role and duties under the EU AI Act

- **Status**: Accepted
- **Date**: 2026-10-10
- **Authors**: Maintainers

## Context

[ADR-0002](0002-data-and-compliance.md) keeps model-written answers off in the
live server until a written assessment says which role the project has under the
EU AI Act and which duties follow. This record is that assessment.

It is an engineering reading of the regulation, not legal advice. Every
statement about the law names the article it rests on. The text was read on
2026-10-10 from the EU Publications Office: Regulation (EU) 2024/1689 as
published, and Regulation (EU) 2026/1744, which amends it.

### What the system is

- The project develops the bot and runs it in its own community, free of charge
  and under its own name.
- A member asks a question with the `/ask` command. The bot retrieves passages
  from project documentation ([ADR-0003](0003-answer-pipeline.md)).
- In **excerpt mode** the reply shows those passages unchanged. No model is
  involved.
- In **model mode** a language model that the operator runs writes an answer
  from the passages. The project does not train or change that model.

## Decision

### The bot is treated as an AI system in both modes

Article 3(1) defines an AI system as a machine-based system that infers from its
input how to generate outputs such as content. In model mode the bot does that.
Whether ranking passages by word statistics is such inference is doubtful, so
excerpt mode may not be an AI system at all.

The project does not rely on that doubt. Both modes tell the member that the
reply is automated.

### The project is the provider, and also the deployer

- **Provider.** Article 3(3) names as provider whoever develops an AI system and
  puts it into service under its own name, whether for payment or free of
  charge. Article 3(11) counts supply for own use as putting into service. The
  project does both.
- **Deployer.** Article 3(4) names as deployer whoever uses an AI system under
  its authority, except in a personal non-professional activity. A public
  community with sponsors is not treated as purely personal here.
- **Not a model provider.** The project neither develops nor places on the
  market a general-purpose AI model. It uses one that others published.

### The open-source exclusion does not apply

Article 2(12) takes AI systems released under free and open-source licences out
of the regulation, unless they fall under Article 5 or Article 50. The bot
interacts directly with people and writes text, so it falls under Article 50.
The licence does not remove the duties below.

### Duties that apply

<!-- markdownlint-disable MD013 -->
| Duty | Source | Measure | State |
| :--- | :--- | :--- | :--- |
| Tell people they deal with an AI system | Art. 50(1) | Notice in the text of every reply | In place |
| Tell them clearly and accessibly, at the first interaction at the latest | Art. 50(5) | The notice is plain text and is on the first reply too; the command description will say it before first use | Reply: in place. Description: approved, not yet in the code |
| Mark generated text as artificially generated, machine-readable and detectable | Art. 50(2) | Model mode only: a fixed marker string, see below | In place, sufficiency open |
| Ensure AI literacy of the people who operate the system | Art. 4 | A moderator guide that explains what the bot does and where it fails | Planned |
<!-- markdownlint-enable MD013 -->

The wording and the places of the notices are in the [AI disclosure
draft](../compliance/ai-disclosure.md).

### Marking generated text

Article 50(2) asks providers of systems that generate synthetic text to mark the
output as artificially generated, in a machine-readable format, as far as this
is technically feasible.

- **Excerpt mode** generates nothing. It shows documentation text unchanged, so
  there is no generated content to mark.
- **Model mode** generates text. A chat message carries no metadata field that
  the bot can set and others can read. The measure available on this platform is
  a fixed marker: every model-written reply starts with the same documented
  string, the constant `LabelAnswer` in
  [`internal/answer`](../../internal/answer/answer.go), which a program can
  detect.

Whether a fixed marker string satisfies "machine-readable format" is not settled
by the article itself. Article 50(7), as replaced by Regulation (EU) 2026/1744,
provides for codes of practice on detection, marking and labelling. They are
checked before model mode goes live, and the result is recorded here.

The transitional period of Regulation (EU) 2026/1744 until 2 December 2026
covers systems placed on the market before 2 August 2026. The bot was not, so it
gets no such period.

### Duties that do not apply

- **Deep fakes and public-interest text** (Art. 50(4)): the bot makes no image,
  audio or video, and its replies answer one member about project documentation.
  They are not published to inform the public on matters of public interest.
- **High-risk rules** (Chapter III): answering questions about software
  documentation is none of the areas listed in Annex III.
- **Prohibited practices** (Art. 5): none is intended or possible with this
  design.

### What may go live, and when

- **Excerpt mode** may go live after the live test. The operator decided this.
- **Model mode** stays off until four gates are passed. Off means that no
  deployment sets the model endpoint variables ([README](../../README.md),
  "Run"). The operator accepted this record with these gates on 2026-10-10:
  1. the notices of the AI disclosure are in the code;
  2. the codes of practice under Article 50(7) have been read for the marking of
     text;
  3. the privacy notice is published;
  4. the gaps of the operator's model endpoint are closed.

Model mode can be switched off again at any time: a deployment without the
model endpoint variables answers with excerpts.

Gate 4 comes from a measurement of the operator's endpoint on 2026-10-10. The
endpoint stores no request body and no prompt. Three gaps remained against the
rule of [ADR-0002](0002-data-and-compliance.md) that the endpoint keeps nothing
it receives. Each is closed in the operator's deployment, not in this
repository:

- access to the endpoint's request log is wider than the people who need it;
- records are kept up to 37 days where 30 are stated;
- the error text the endpoint stores can carry model output.

A switch in code would add nothing to this: it could be flipped as easily as a
variable. The gates are the list above.

The acceptance covers the reading of the role and the duties in this record. It
is the project's reading, not legal advice.

## Consequences

- **Positive**: The project knows its role and has a short list of duties, each
  tied to an article and to a place in the code or the documents. Excerpt mode
  can serve members while model mode waits for its gates.
- **Negative**: The project carries the provider's duties and cannot pass them
  to the model's publisher. The marking of generated text rests on a marker
  string until better means exist on the platform. The assessment must be read
  again when the regulation, its guidance or the bot's functions change.

## Open points

- Codes of practice and Commission guidance on Article 50: not read for this
  record. They are checked before model mode goes live.
- Whether excerpt mode is an AI system: left open, because the notices are given
  either way.
- Which national authority is competent, and whether any registration applies:
  not examined.
- New functions, such as moderation
  ([ADR-0005](0005-moderation-and-escalation.md)), need their own reading before
  they are built.

## References

- Regulation (EU) 2024/1689, Articles 2(10), 2(12), 3(1), 3(3), 3(4), 3(11), 4,
  5, 50, 113 and Annex III:
  <http://publications.europa.eu/resource/celex/32024R1689>
- Regulation (EU) 2026/1744, which replaces Article 50(7) and adds the
  transitional rule for Article 50(2):
  <http://publications.europa.eu/resource/celex/32026R1744>
