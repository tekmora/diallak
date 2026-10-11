# Privacy notice

Draft. It is not published to members yet, and it is not legal advice. The
operator has decided its content; a legal review comes before the public launch
and is listed at the end.

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

## Your right to object

*Article 21(1) and (4).*

You can object at any time to the processing of your personal data by the bot,
on grounds relating to your particular situation. After an objection your data
is no longer processed, unless compelling legitimate grounds override your
interests.

To object, write to the e-mail address under "Who is responsible" or open a
private thread in the support forum, addressed to the operator.

## Who is responsible

*Article 13(1)(a) and (b).*

The bot is run by the provider named here and in the [imprint](imprint.md):

<!-- identity block: start -->
- **Name:** *filled before launch*
- **Address:** *filled before launch*
- **E-mail:** *filled before launch*
<!-- identity block: end -->

The block is filled before the public launch with the provider's full name, the
address of a service that is authorised to receive legal mail for the provider,
and an e-mail address.

No data protection officer is appointed.

## What is processed, for what purpose and on what basis

*Article 13(1)(c) and (d), Article 14(2)(f).*

<!-- markdownlint-disable MD013 -->
| Data | Purpose | Legal basis |
| :--- | :--- | :--- |
| The question you pass to `/ask` | Finding documentation passages and writing the reply | Legitimate interests, Article 6(1)(f) |
| The id of the server the command came from | Refusing every server but the configured one | Legitimate interests, Article 6(1)(f) |
| Other fields Discord sends with a command, such as your member id, name, roles and locale | None: the bot reads none of them and keeps none | Not used |
<!-- markdownlint-enable MD013 -->

The question comes from you. The other fields come from Discord, which attaches
them to every command.

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

## Where the bot runs

*Article 13(1)(e) and (f).*

<!-- hosting statement: this block is replaced as a whole when the bot moves -->
The bot and the model endpoint run on machines the operator owns, in Germany.
No hosting provider is involved, and the bot sends your data to no place
outside the European Economic Area. The reply itself reaches you through
Discord, like every message on the server; Discord's own policy covers that.
<!-- hosting statement: end -->

A move to a rented server is planned. Before it happens this block names the
provider, the country of its data centre and the agreement that binds it.

## How long data is kept

*Article 13(2)(a).*

<!-- markdownlint-disable MD013 -->
| Data | Kept |
| :--- | :--- |
| Your question, in the bot | In memory until the reply is sent |
| Your question, at the model endpoint (model mode only) | Not kept: the endpoint must not keep or train on what it receives (ADR-0002). The endpoint keeps its records about a request for 30 days; what such a record holds is stated here before model mode goes live |
| Log lines of the bot, which hold no personal data | 30 days |
<!-- markdownlint-enable MD013 -->

## Your rights

*Article 13(2)(b), (c) and (d).*

You can ask the controller for access to your personal data, for its
rectification or erasure, for restriction of its processing, and for data
portability. Your right to object has its own section above.

The bot stores nothing about you, so there is normally nothing to hand over,
correct or delete. You can still ask, and you get an answer without undue
delay and at the latest within one month (Article 12(3)).

The processing does not rest on consent, so there is no consent to withdraw.

You have the right to lodge a complaint with a supervisory authority, in
particular in the Member State where you live or work (Article 77).

To send a request, write to the e-mail address under "Who is responsible". A
private thread in the support forum, addressed to the operator, is accepted
too.

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
moderation trail, GitHub Discussions, supporter roles and a first answer in the
support forum ([data inventory](data-inventory.md), "Not yet built"). This
notice gains the matching entries before such a feature goes live. The entries
for the support forum are drafted in
[Support forum answer](support-forum-answer.md). Two periods are decided
already: a support ticket is kept 30 days after it is closed, and the
moderation trail 90 days.

## Where this notice lives

The source is this file in the project's repository. Once published, it is
linked from the bot's profile, the rules channel and the application's entry in
Discord's developer portal. It moves when the organisation has one place for
its legal texts, and a link stays behind.

Discord's Developer Terms of Service ask for both links: a privacy policy that
is linked in the developer portal and easy to reach from the application
(Section 5(a)), and an easy way for a user to ask that their data is changed or
deleted (Section 5(b)). The request path under "Your rights" is that way.

## Open point for the operator

Which parts move to the rented server, the bot alone or the model endpoint too.
The block under "Where the bot runs" changes with the answer.

## Before the public launch

A legal review examines what the operator has decided for this draft:

- what the identity block and the imprint have to hold;
- whether legitimate interests is the right legal basis;
- that no data protection officer has to be appointed;
- that the notice may state the right to complain without naming an authority;
- what the notice has to say about the place the bot runs in, now and after the
  move;
- whether the bot itself has to point a member to the right to object at first
  contact (Article 21(4) says "at the latest at the time of the first
  communication").

## Sources

- Regulation (EU) 2016/679, Articles 6(1)(f), 12(3), 13, 14(2)(f), 21 and 77, as
  published, read on 2026-10-10 from the EU Publications Office:
  <http://publications.europa.eu/resource/celex/32016R0679>
- Discord Developer Terms of Service, Section 5, effective 2024-07-08, read on
  2026-10-10:
  <https://support-dev.discord.com/hc/en-us/articles/8562894815383-Discord-Developer-Terms-of-Service>
