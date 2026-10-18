# Privacy notice

Draft. It is not published to members yet, and it is not legal advice. The
operator has decided its content. The legal points it raises are researched in
[Legal research](legal-research.md); that research is not a gate for the launch.

This notice covers the bot Diallak in the community server. What Discord does
with your data is covered by Discord's own privacy policy, not by this notice.

English is the source and the binding version of this notice. A translation is
linked when someone provides it and is marked as a translation.

The notice follows the list of Article 13 of Regulation (EU) 2016/679 (GDPR).
Each section names the point of that article it answers. What the bot does with
data is taken from the [data inventory](data-inventory.md), which is checked
against the code.

## In short

- The bot stores nothing about you. It has no database; the one file it may
  write holds public news items, nothing about members.
- It reads two things from `/ask`: the question you pass and the id of the
  server the command came from.
- With `/areas` you choose interest, language and notification roles. For that
  the bot reads your member id, the roles you hold and whether you have passed
  the server's screening, gives or takes the roles you chose, and keeps none of
  it. Other members can see the roles you hold.
- With `/bug` you give a title and the steps of a bug. The bot reads them only
  to build a link that opens a filled-in new issue on GitHub, shows the link to
  you alone and keeps nothing. Nothing reaches GitHub unless you open the link
  and submit the issue there yourself, under your own account and GitHub's terms.
- When a moderator removes your message or times you out, the bot writes one
  line into the server's log channel with your member id, the moderator and
  the reason, and deletes it after 90 days. It holds no text of your message,
  and the bot sends you no message about it.
- Your question is held in memory until the reply is sent, and then it is gone.
- The reply is visible only to you.
- When your question left the bot, the reply has a button that shows you a
  receipt: which guardrails and policies ran, where your text went and how
  long it is kept. The bot holds that receipt in memory for 15 minutes. It
  sends it as a direct message only when you press for that.
- When the operator has switched on the bot's own masking, the receipt also
  shows the text sent on for the model after masking, and what kinds of data
  the masking replaced. The bot holds that masked text with the receipt in
  memory for 15 minutes, not your question as you wrote it.
- Your question is never used to train or evaluate a model
  ([ADR-0002](../adr/0002-data-and-compliance.md)).
- When the operator has switched the forum answer on, the bot reads the title
  and the first message of a new post in the support forum, once, to find
  documentation that answers it. It reads no other message.
- It reads a post only when you add the tag "Automated answer" when you create
  it ([ADR-0020](../adr/0020-forum-answers-only-opted-in-posts.md)).
- Its reply there is public, like the post, and holds documentation text, not
  your text. No model writes it.

## Your right to object

*Article 21(1) and (4).*

You can object at any time to the processing of your personal data by the bot,
on grounds relating to your particular situation. After an objection your data
is no longer processed, unless compelling legitimate grounds override your
interests.

To object, write to the e-mail address under "Who is responsible". That is the
binding path. A private thread in the ticket channel of the server, addressed
to the operator, is accepted too.

In the support forum the bot reads a post only when you add the tag
"Automated answer" when you create the post. Without the tag it does not read
the post and does not reply. Adding the tag later does not make the bot read
the post, and removing it later does not take a reply back, because the bot
reads a post once, when it is created.

## Who is responsible

*Article 13(1)(a) and (b).*

The bot is run by the provider named here and in the [imprint](imprint.md):

<!-- identity block: start -->
- **Name:** Kilian Cromm
- **Address:** c/o IBC, Bahnhofstraße 9, 65549 Limburg a. d. Lahn
- **E-mail:** lusoris@pm.me
<!-- identity block: end -->

The block holds the provider's full name, a c/o address at which the provider
receives legal mail, and an e-mail address.

No data protection officer is appointed.

## What is processed, for what purpose and on what basis

*Article 13(1)(c) and (d), Article 14(2)(f).*

<!-- markdownlint-disable MD013 -->
| Data | Purpose | Legal basis |
| :--- | :--- | :--- |
| The question you pass to `/ask` | Finding documentation passages and writing the reply | Legitimate interests, Article 6(1)(f) |
| The id of the server the command came from | Refusing every server but the configured one | Legitimate interests, Article 6(1)(f) |
| With `/areas`: your member id, the roles you hold, whether you have passed the server's screening, and the options you choose | Showing what you hold, giving and taking the roles you choose, refusing a member who has not passed screening | Legitimate interests, Article 6(1)(f) |
| With `/bug`: the title and the steps you pass | Building the new-issue link in the reply | Legitimate interests, Article 6(1)(f) |
| When you press the receipt's direct message button: your member id | Sending the receipt to you as a direct message, for that one request | Legitimate interests, Article 6(1)(f) |
| When a moderator acts on you: your member id, and for a removed message its channel and message id | Carrying out the act and writing the trail line, so the operator can check what moderators did | Legitimate interests, Article 6(1)(f) |
| When you are a moderator and use a moderation command: your member id, the roles you hold, and the reason you give | Refusing callers without the Moderator role and naming you and your reason in the trail line | Legitimate interests, Article 6(1)(f) |
| Title and first message of a new post in the support forum | Finding documentation passages for a first reply in that post | Legitimate interests, Article 6(1)(f) |
| Whether a post was written by an application | Leaving such a post without a reply | Legitimate interests, Article 6(1)(f) |
| Whether a post carries the tag "Automated answer" | Reading only a post that carries it | Legitimate interests, Article 6(1)(f) |
| When the supporter function is switched on, for every member of the server: your member id and the roles you hold, when you join, change or leave the server, and once after the bot starts | Giving you "Supporter" while you hold "Ko-fi supporter" or "Patreon supporter", and taking it after a grace period when you hold neither ([ADR-0024](../adr/0024-supporter-roles-from-funding-platforms.md), [ADR-0027](../adr/0027-members-intent-for-supporter-roles.md)) | Legitimate interests, Article 6(1)(f); see "Legal points still open" |
| When you link a forge account for the Contributor role, at the cluster's identity provider: your Discord user id, a Discord token that lets the provider write your role connection, your forge account's id and name, and whether a contribution of that account was merged | Writing that value so that Discord gives you the Contributor role, and keeping it current after a later merge ([ADR-0026](../adr/0026-member-to-forge-account-link.md)). The bot receives none of it | Open; see "Legal points still open" |
| Other fields Discord sends with a post or with events about channels, roles and threads, such as the id of the member who created a thread | None: the bot compares the channel and the kind of thread and reads nothing else | Not used |
| Other fields Discord sends with a command or a button press, such as your name and locale, your member id and roles with `/ask`, and the text of a message a moderator removes | None: the bot reads none of them and keeps none | Not used |
<!-- markdownlint-enable MD013 -->

The question, the options you choose and your post come from you. The other
fields come from Discord, which attaches them to every command and event.

The legitimate interest is to answer the question a member puts to the bot, in
the server the bot is meant for, to give a member the roles that member
chose, to keep the server moderated with a record of every act, to give a
question in the support forum a first answer from the documentation at once,
and to give the Supporter role to a member who supports the project on a
funding platform.
You did not call the bot in the forum, so the forum's guidelines say that it
reads the first message before you post. The bot
asks for no consent. This basis is the
operator's choice; [Legal research](legal-research.md), point 2, has the
sources.

The bot's logs hold no question, no answer and no member id. They record the
outcome of a request, such as `answer` or `refusal`, and how long it took; for
`/areas`, which role was given or taken and whether that worked; for a
receipt, whether it was shown or sent; for a moderation command, why it was
refused or whether it worked; for a forum post, whether it was answered or
why not, with no title, text, post id or member id
([data inventory](data-inventory.md), "What the bot logs").

## Who receives your data

*Article 13(1)(e).*

- **Discord** receives the reply, because the bot answers through Discord. With
  `/areas` Discord also receives the role change, with a fixed reason that says
  it was your own choice. With the supporter function, Discord receives the
  change of "Supporter", with a fixed reason that names the funding platform
  roles. Discord shows the role to the other members and keeps
  an entry in the server's audit log for 45 days, visible to roles that may
  view that log. When you press for it, Discord also receives your receipt as
  a direct message to you. When a moderator acts, Discord receives the act
  with the moderator's reason, which its audit log keeps for 45 days, and the
  trail line in the log channel, which the moderators and the operator can
  read. In the support forum Discord receives the reply, which is public in
  your post.
- **The model endpoint** receives your question and the documentation passages
  that were found, in model mode only. The operator runs that endpoint. It
  receives no member id and no name
  ([`internal/model`](../../internal/model/model.go)). It receives nothing
  from the support forum: posts are answered with excerpts only.
- **The embedding endpoint** receives your question, in hybrid retrieval mode
  only. The operator runs that endpoint on-premises. It receives no member id
  and no name ([`internal/embed`](../../internal/embed/embed.go)), at most once
  per question, and must not keep or train on what it receives
  ([ADR-0022](../adr/0022-embeddings-for-paraphrased-questions.md)). It
  receives nothing from the support forum: posts are searched by keyword only.
- Nobody else. The bot sells nothing and shares nothing for advertising.

Model mode is switched off in the live server until the gates of
[ADR-0013](../adr/0013-guardrail-policy-and-receipt.md) are passed. Until
then your question leaves the bot only as far as the reply needs it: the reply
holds documentation text, not your question.

The endpoint runs guardrails on your question before the model sees it. They
can mask or change parts of the text, personal data among them, but they do
not find everything. Without the bot's own masking, the bot does not see what
they changed: the receipt names the guardrails that ran and never says that
your data was masked.

When the operator sets `DIALLAK_MODEL_GUARDRAIL`, the bot first sends your
question to the endpoint's masking route and sends on only the text that comes
back ([ADR-0013](../adr/0013-guardrail-policy-and-receipt.md), step two). The
receipt then shows you that masked text and says that the masking does not
find everything: a passage it missed stands there as you wrote it. When no
masked text comes back, the bot sends nothing to the model and answers with
excerpts.

## Where the bot runs

*Article 13(1)(e) and (f).*

<!-- hosting statement: this block is replaced as a whole when the bot moves -->
The bot and the model endpoint run on machines the operator owns, in Germany.
No hosting provider is involved. The bot and the endpoint themselves send
your data to no place outside the European Economic Area.
<!-- hosting statement: end -->

**The reply and Discord.** The bot hands its reply to Discord, which delivers
it to you like every message on the server. For members in the European
Economic Area, Discord Netherlands BV is the controller of what Discord holds.
Discord also processes data in the United States. Discord Inc. states that it
takes part in the EU-U.S. Data Privacy Framework, for which the European
Commission decided that it protects data adequately (Article 45 GDPR,
Implementing Decision (EU) 2023/1795). No source found settles whether handing
the reply to Discord is a transfer to a third country by the operator; this
notice names it either way. The [legal research](legal-research.md), point 5,
has the details.

A move to a rented server is planned. Before it happens this block names the
provider, the country of its data centre and the agreement that binds it.

## How long data is kept

*Article 13(2)(a).*

<!-- markdownlint-disable MD013 -->
| Data | Kept |
| :--- | :--- |
| Your question, in the bot | In memory until the reply is sent |
| The title and the steps you pass to `/bug`, in the bot | In memory until the reply is sent |
| Your member id, your roles and your choice with `/areas`, in the bot | In memory until the reply is sent. The role itself stays on you until you take it off with `/areas` |
| Your question, at the model endpoint (model mode only) | Not written to storage and not used for training (ADR-0002). The three rows below name what the endpoint holds for a time anyway |
| Your question, at the embedding endpoint (hybrid retrieval mode only) | Not written to storage and not used for training (ADR-0002, ADR-0022) |
| The answer, in the endpoint's cache (model mode only) | At most 60 minutes, in memory only. The cache finds an entry by a hash of the request, so it holds the answer but not your question. The bot sends no member id: another member who sends the identical question within that time gets the same answer |
| A blocked question, in the endpoint's log (model mode only) | When a filter of the endpoint blocks a question, one log line holds the fragment that matched, never the whole question. The line is deleted after about 74 hours at most |
| The endpoint's record of a request (model mode only) | 30 days. The record holds figures about the request only: which key and which model, how many tokens, and when. It holds no question and no answer |
| The receipt of a reply, in the bot | In memory for 15 minutes, for at most 1000 replies at once, and lost on a restart. It holds the names of the guardrails and policies, the time, and whether the model's answer was shown, and no member id. Without the bot's own masking it holds no text of yours |
| The masked text of your question, in the bot (only with the bot's own masking) | In memory with the receipt for 15 minutes, for at most 1000 replies at once, and lost on a restart. It is the text after masking, as the receipt shows it, with the kinds of data replaced and their counts; a passage the masking missed stands in it as you wrote it |
| Your member id, when you press for the receipt as a direct message | In memory until the direct message is sent |
| The trail line of a moderator's act, in the server's log channel | 90 days, unless the operator configures 1 to 365 days; the bot deletes it once a day after that |
| Title and first message of your post in the support forum, in the bot | In memory until the reply is sent or the attempt ends |
| Your member id and roles from a member event or the member list, in the bot (supporter function only) | In memory while the event or list is handled. When you hold neither platform role, your member id and a timer stay in memory until the grace period ends, 72 hours unless the operator configures 1 to 90 days, or until you hold a platform role again; lost on a restart. "Supporter" itself stays on you until the grace period ends |
| The link for the Contributor role, at the cluster's identity provider: your Discord user id, the Discord tokens, your forge account's id and name, and the value written | Until you remove the application's authorisation in Discord, which ends the next token refresh, or ask for deletion. On a deletion request the provider first deletes your role connection, so Discord takes the role. The access token also ends after seven days |
| Log lines of the bot, which hold no personal data | 30 days |
<!-- markdownlint-enable MD013 -->

A reply in the support forum stays in your post under Discord's terms. The bot
keeps no copy of it and no reference to it. A moderator can remove it.

The three periods of the endpoint, and what each place holds, were measured
on the operator's endpoint on 2026-10-11.

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

To send a request, write to the e-mail address under "Who is responsible".
That is the binding path. A private thread in the ticket channel of the
server, addressed to the operator, is accepted too. Such a thread is visible to
you, to the people you add and to moderators who manage threads; use e-mail
when you want none of them to read along.

## Whether you have to provide data

*Article 13(2)(e).*

No. Using `/ask` is voluntary. Without a question the bot has nothing to answer.
Using `/areas` is voluntary too, and every role it gives you can be taken off
with it again. Using `/bug` is voluntary, and an issue is filed only when you
submit it on GitHub yourself.

## Automated decisions

*Article 13(2)(f).*

The bot makes no decision about you and builds no profile. Its replies are
automated text about project documentation, and each one says so
([AI disclosure](ai-disclosure.md)).

## What you should not send

Do not put secrets, or personal data of yourself or of other people, into a
question. The bot does not need them to answer. The same holds for a post in
the support forum that carries the tag "Automated answer": the bot reads its
first message.

## Changes to this notice

The news function, when the operator switches it on, posts own releases,
security advisories and digests of public feeds. It reads only public sources
and processes no data about members: its posts mention a role, never a
member, and the file it keeps holds feed items and read times only
([data inventory](data-inventory.md), "What the bot stores").

The Discussions bridge, when the operator switches it on, posts the title,
category and link of each new public Q&A or Ideas discussion of the project's
repositories into one channel. It names no author and mentions no one, and
nothing written on the server goes to GitHub.

Features that are planned will process more data: support tickets and GitHub
Discussions as a source of answers
([data inventory](data-inventory.md), "Not yet built"). This notice gains the
matching entries before such a feature goes live. One
period is decided already: a support ticket is kept 30 days after it is
closed.

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

## Legal points researched

[Legal research](legal-research.md), points 1 to 6, has the findings and
sources on what the operator has decided for this draft. The research is not a
gate for the launch:

- what the identity block and the imprint have to hold;
- whether legitimate interests is the right legal basis;
- that no data protection officer has to be appointed;
- that the notice may state the right to complain without naming an authority;
- what the notice has to say about the place the bot runs in, now and after the
  move;
- whether the bot itself has to point a member to the right to object at first
  contact (Article 21(4) says "at the latest at the time of the first
  communication").

## Legal points still open

These points are named for legal review and not yet researched. They are not a
gate for the launch:

- whether legitimate interests covers reading a member's roles and giving and
  taking the roles the member chose with `/areas`, given that other members can
  see the roles, and that language roles can hint at where someone is from
  ([ADR-0011](../adr/0011-area-roles-from-member-choice.md));
- whether legitimate interests covers reading the roles of every member for
  the supporter function, when only supporters gain from it
  ([ADR-0024](../adr/0024-supporter-roles-from-funding-platforms.md), [ADR-0027](../adr/0027-members-intent-for-supporter-roles.md));
- the legal basis for the link the identity provider keeps for the Contributor
  role; consent given in Discord's authorisation screen, Article 6(1)(a), and
  withdrawn by removing the authorisation, is the candidate
  ([ADR-0026](../adr/0026-member-to-forge-account-link.md)).

## Sources

- Regulation (EU) 2016/679, Articles 6(1)(f), 12(3), 13, 14(2)(f), 21 and 77, as
  published, read on 2026-10-10 from the EU Publications Office:
  <http://publications.europa.eu/resource/celex/32016R0679>
- Discord Developer Terms of Service, Section 5, effective 2024-07-08, read on
  2026-10-10:
  <https://support-dev.discord.com/hc/en-us/articles/8562894815383-Discord-Developer-Terms-of-Service>
