# Privacy notice

Draft for operator review. It is not published to members yet, and it is not
legal advice. A point the operator still has to decide or fill in is marked
**OPEN**; the list at the end collects them.

This notice covers the bot Diallak in the community server. What Discord does
with your data is covered by Discord's own privacy policy, not by this notice.

English is the source and the binding version of this notice. A translation is
linked when someone provides it and is marked as a translation.

The notice follows the list of Article 13 of Regulation (EU) 2016/679 (GDPR).
Each section names the point of that article it answers. What the bot does with
data is taken from the [data inventory](data-inventory.md), which is checked
against the code.

## In short

- The bot stores nothing about you. It has no database and writes no file.
- It reads two things from a command: the question you pass to `/ask` and the id
  of the server the command came from.
- Your question is held in memory until the reply is sent, and then it is gone.
- The reply is visible only to you.
- Your question is never used to train or evaluate a model
  ([ADR-0002](../adr/0002-data-and-compliance.md)).

## Who is responsible

*Article 13(1)(a) and (b).*

The bot is run by Lusoris, the maintainer of the project. Contact:
<lusoris@pm.me>.

Whether a handle and an e-mail address are enough to name the responsible party
is a legal question. It is examined in the legal review that comes before the
public launch; until then this section is not final.

**OPEN**: contact of a data protection officer, or the statement that none is
appointed.

## What is processed, for what purpose and on what basis

*Article 13(1)(c) and (d).*

<!-- markdownlint-disable MD013 -->
| Data | Purpose | Legal basis |
| :--- | :--- | :--- |
| The question you pass to `/ask` | Finding documentation passages and writing the reply | Legitimate interests, Article 6(1)(f) |
| The id of the server the command came from | Refusing every server but the configured one | Legitimate interests, Article 6(1)(f) |
| Other fields Discord sends with a command, such as your member id, name, roles and locale | None: the bot reads none of them and keeps none | Not used |
<!-- markdownlint-enable MD013 -->

The legitimate interest is to answer the question a member puts to the bot, in
the server the bot is meant for. The bot asks for no consent. This basis is the
operator's choice and is examined in the legal review before the public launch.

The bot's logs hold no question, no answer and no member id. They record the
outcome of a request, such as `answer` or `refusal`, and how long it took
([data inventory](data-inventory.md), "What the bot logs").

## Who receives your data

*Article 13(1)(e).*

- **Discord** receives the reply, because the bot answers through Discord.
- **The model endpoint** receives your question and the documentation passages
  that were found, in model mode only. The operator runs that endpoint. It
  receives no member id and no name
  ([`internal/model`](../../internal/model/model.go)).
- Nobody else. The bot sells nothing and shares nothing for advertising.

Model mode is switched off in the live server until the gates of
[ADR-0006](../adr/0006-ai-act-role.md) are passed. Until then your question
leaves the bot only as far as the reply needs it: the reply holds documentation
text, not your question.

**OPEN**: the hosting provider of the bot and of the model endpoint, if a third
party hosts either of them.

## Transfers outside the European Union

*Article 13(1)(f).*

**OPEN**: where the bot and the model endpoint run. If either runs outside the
European Economic Area, this section names the country and the safeguard.

## How long data is kept

*Article 13(2)(a).*

<!-- markdownlint-disable MD013 -->
| Data | Kept |
| :--- | :--- |
| Your question, in the bot | In memory until the reply is sent |
| Your question, at the model endpoint (model mode only) | Not kept: the endpoint must not keep or train on what it receives (ADR-0002). What the endpoint records about a request, and for how long, is stated here before model mode goes live |
| Log lines of the bot, which hold no personal data | **OPEN**: the period the hosting keeps logs |
<!-- markdownlint-enable MD013 -->

## Your rights

*Article 13(2)(b), (c) and (d).*

You can ask the controller for access to your personal data, for its
rectification or erasure, for restriction of its processing, and for data
portability. You can object to the processing (Article 21), because it rests on
legitimate interests.

The bot stores nothing about you, so there is normally nothing to hand over,
correct or delete. You can still ask, and you get an answer without undue
delay and at the latest within one month (Article 12(3)).

The processing does not rest on consent, so there is no consent to withdraw.

You have the right to lodge a complaint with a supervisory authority, in
particular in the Member State where you live or work (Article 77). **OPEN**:
the authority competent for the controller.

**OPEN**: how a member sends a request, for example an address or a channel.

## Whether you have to provide data

*Article 13(2)(e).*

No. Using `/ask` is voluntary. Without a question the bot has nothing to answer.

## Automated decisions

*Article 13(2)(f).*

The bot makes no decision about you and builds no profile. Its replies are
automated text about project documentation, and each one says so
([AI disclosure](ai-disclosure.md)).

## What you should not send

Do not put secrets, or personal data of yourself or of other people, into a
question. The bot does not need them to answer.

## Changes to this notice

Features that are planned will process more data: support tickets, the
moderation trail, GitHub Discussions and supporter roles
([data inventory](data-inventory.md), "Not yet built"). This notice gains the
matching entries before such a feature goes live.

## Open points for the operator

1. Data protection officer: contact, or none appointed.
2. Hosting provider of the bot and of the model endpoint, if any.
3. Where the bot and the model endpoint run, and the safeguard for a transfer
   outside the European Economic Area, if there is one.
4. The period the hosting keeps the bot's logs.
5. The competent supervisory authority.
6. How a member sends a request.
7. Where the published notice lives and from where it is linked.

## Before the public launch

A legal review examines two points the operator has decided for the draft:
whether naming a handle and an e-mail address satisfies the duty to name the
responsible party, and whether legitimate interests is the right legal basis.

## Sources

- Regulation (EU) 2016/679, Articles 6(1)(f), 12(3), 13, 21 and 77, as
  published, read on 2026-10-10 from the EU Publications Office:
  <http://publications.europa.eu/resource/celex/32016R0679>
