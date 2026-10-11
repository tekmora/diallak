# ADR-0009: First answer in the support forum

- **Status**: Accepted
- **Date**: 2026-10-11
- **Authors**: Maintainers

## Context

Members ask for help in the support forum, one post per question
([server rules](../compliance/server-rules.md), rule 3). The operator decided
that the bot answers a new post there first, from the documentation, and that
it reads the full text of the post to do so.

Until now the bot sees only what a member passes to a command. Three accepted
records say so:

- [ADR-0001](0001-architecture-and-boundaries.md): the bot requests no
  privileged gateway intent, and a feature that needs one adds its own record.
- [ADR-0002](0002-data-and-compliance.md): the bot subscribes to no message
  events and sees only what a member passes to a command.
- [ADR-0003](0003-answer-pipeline.md): a reply is visible only to the member
  who asked.

A post in the forum is text that a member did not address to the bot. On
Discord an application reads such text only with the privileged right to
message content: without it the text of a message arrives empty. The operator
chose the full text over an answer from the title of a post, which needs no
such right, and accepted the cost: a privileged right, more member data and new
entries in the privacy documents.

This record is that feature's own record. It fixes how little the bot reads,
what it keeps, what the reply looks like, how a member keeps the bot away from
a post and which cases the implementation has to pass. Nothing of it is built
yet.

## Decision

### One place, one moment

- The bot reads the first message of a new post in one forum channel. A setting
  names that channel; without it the feature is off and the bot behaves as
  before. The channel id never enters the tree, like the server id.
- It reads the message once, when the post is created.
- It never reads a later message in a post, an edit of the first message, a
  post that existed before the bot started or was created while it was down, a
  thread in any other channel, or any other message on the server.

These limits follow the rule that only the data a purpose needs is processed
(Articles 5(1)(c) and 25(2) of the GDPR). That is the project's reading, not
legal advice.

### A member can keep the bot away from a post

A post that carries the opt-out tag is neither read nor answered. The bot does
not request its first message and does not use the title that came with the
event.

- A second setting names the tag. The operator creates the tag in the forum
  and leaves it free for every member to set; the proposed name is "No
  automated reply". The tag id never enters the tree.
- With the forum configured and no tag configured, the start stops. The
  feature does not run without the way to opt out.
- The tag counts when the post is created, because the bot reads once, at that
  moment. A tag that is added later does not take a reply back, and a tag that
  is removed later does not make the bot read the post.

### How the text reaches the bot

The bot does not subscribe to message events for this.

1. The gateway session requests one standard intent, the one that carries
   thread events. It requests no message intent and no privileged intent, so no
   message event of any channel reaches the process.
2. A thread event tells the bot that a post was created. The bot acts only when
   the event carries the mark of a newly created thread, the thread is a public
   thread, its parent is the configured forum and it does not carry the
   opt-out tag.
3. For such a post the bot asks the platform for the first message through the
   HTTP API: one request for one message.

The right to message content is switched on for the application in Discord's
developer portal. That is the operator's act. The right applies to the HTTP API
whatever intents a gateway session passes (Discord's documentation, "HTTP
Restrictions").

The standard intent also brings events about channels, roles and threads of
the whole server. A thread event holds the title of the thread and the id of
its creator. The bot drops every event that is not a new post in the
configured forum, after it has compared the parent id, the type and the tags
and nothing else.

### What is read

<!-- markdownlint-disable MD013 -->
| Data | Read | Used for |
| :--- | :--- | :--- |
| Title of the post | Yes | Finding documentation passages |
| Text of the first message | Yes, up to a fixed length; the rest is cut | Finding documentation passages |
| Whether the author is an application | Yes | A post that an application wrote gets no reply |
| Tags of the post | Only whether the opt-out tag is among them | Leaving a tagged post unread |
| Author id, name, roles | No | |
| Attachments, embeds, polls, components | No | |
<!-- markdownlint-enable MD013 -->

### What is kept, and for how long

- Title and text are held in memory until the reply is sent or the attempt
  ends. The deadline of a question bounds that time
  ([README](../../README.md), "Optional settings").
- The bot stores nothing. It keeps no list of posts, authors or replies.
- The logs hold what they hold today: outcome, reason, error class and
  duration ([data inventory](../compliance/data-inventory.md), "What the bot
  logs"). They hold no title, no text, no post id and no member id.
- The reply stays in the post under Discord's terms. The bot keeps no copy and
  no reference to it.

### Where the text goes

Nowhere. In the forum the bot answers with excerpts from the documentation,
which no model writes. The text of a post is not sent to the model endpoint,
whatever the model settings are.

Model-written replies in the forum would be a separate record: they would send
text that was not addressed to the bot to the endpoint, and they would publish
text that a crafted post could steer. ADR-0003 kept replies private for the
second reason.

### The reply

- The reply is a public message in the post. A post gets at most one.
- It mentions nobody.
- When no passage passes the threshold of ADR-0003, the bot posts nothing. The
  same holds when the attempt fails or runs out of time. People answer in the
  post either way, and a public "nothing found" helps nobody.
- The first message can be empty for the bot: a post with an attachment only,
  or the right switched off in the portal. The bot then posts nothing and logs
  the reason. It does not fall back to the title alone.
- At start, with the feature configured, the bot asks the platform whether the
  application has the right. If not, the start stops with a message that says
  so.

### The label

The wording follows the rules of the
[AI disclosure](../compliance/ai-disclosure.md): plain text, on every reply.

<!-- markdownlint-disable MD013 -->
| Place | Text |
| :--- | :--- |
| Guidelines of the forum, shown before a member posts | "The automated assistant reads the first message of a new post and may reply first with excerpts from the documentation. It reads nothing else here. To keep it away from a post, add the tag 'No automated reply' when you create it. More is in the privacy notice." |
| First line of the reply | "Automated excerpts from the documentation. No AI model wrote this reply." (unchanged) |
| Last line of the reply | "Automated first reply. A person can help here in this post." |
<!-- markdownlint-enable MD013 -->

The guidelines text is set in the server by the operator.

### What stays as it is

- `/ask` is unchanged: its reply is visible only to the member who asked, and
  model mode follows the gates of [ADR-0006](0006-ai-act-role.md).
- Text from Discord is never training data and never part of an evaluation set
  (ADR-0002).
- The bot serves one server (ADR-0001) and does not read messages for
  moderation ([ADR-0005](0005-moderation-and-escalation.md)).

### Cases the implementation must pass

Each case is a test against the fake Discord. Each is seen failing once before
it is trusted.

<!-- markdownlint-disable MD013 -->
| # | Case | Expected |
| :--- | :--- | :--- |
| 1 | The feature is not configured | The session requests no intent, as today; no thread event is handled |
| 2 | The feature is configured | The session requests exactly the one standard intent: no message intent, no privileged intent |
| 3 | A thread is created in another channel or another forum | No request for a message, no reply |
| 4 | A message event is handed to the bot anyway | Dropped; its content is never read |
| 5 | A thread event without the mark of a new thread, such as the bot being added to an old post | No request, no reply |
| 6 | A second message in a post, or an edit of the first | No request, no second reply |
| 7 | A post written by an application | No reply |
| 8 | A post whose title and text hold an e-mail address, a telephone number and a name | A reply is sent; the captured log holds none of the three; no file is written; the model endpoint receives no request although one is configured |
| 9 | A post with no passage above the threshold | No reply |
| 10 | The request for the first message fails, runs out of time or returns an empty text | No reply; the outcome is logged with its reason; the bot keeps running |
| 11 | The application lacks the right at start | The start stops with a message that says so |
| 12 | The documentation text of a reply holds a mention | The reply mentions nobody |
| 13 | Any reply in the forum | First and last line are exactly the two texts above |
| 14 | A text longer than the fixed length | It is cut, and the reply is built from the part that is left |
| 15 | More new posts at once than the bound on concurrent work | The posts over the bound get no reply; nothing is queued without a bound |
| 16 | The bot starts while the forum already holds posts | No request for any of them |
| 17 | A new post in the forum that carries the opt-out tag | No request for its message, no reply; its title is not used |
| 18 | The forum is configured and the opt-out tag is not | The start stops with a message that says so |
| 19 | The opt-out tag is added to a post, or removed from it, after its creation | No request, no second reply; a reply that was sent stays |
<!-- markdownlint-enable MD013 -->

## Consequences

- **Positive**: A member gets a first answer without calling a command. The
  bot reads one message per new post in one channel and nothing else, by
  construction: it never receives a message event. Nothing more is stored or
  logged than before.
- **Negative**: The application holds a privileged right, which Discord
  reviews once an application reaches 10,000 users. The bot now receives
  events about channels, roles and threads of the server, although it uses one
  kind. A reply is public, so a wrong excerpt is seen by everyone. Posts made
  while the bot is down get no first answer from it.

## Alternatives considered

- **An answer from the title alone.** It needs no privileged right, because a
  title arrives with the thread event. The operator decided for the full text.
- **Message events with the content right in the gateway session.** Every
  message of every channel the bot can see would reach the process, and a
  filter in the code would be all that keeps the rest unread.
- **An answer only when the bot is mentioned.** Discord delivers the text of a
  message that mentions the application without the privileged right. A member
  would have to know that and do it; the operator wants the first answer
  without a step by the member.

## Supersedes

For the support forum only; everything else in the three records stands.

- ADR-0001, "A gateway connection, least privilege" and "The existing Discord
  application": the sentences that the bot requests no privileged intent. They
  stand for the gateway session. They no longer mean that the bot uses no
  privileged right: it uses the right to message content through the HTTP API
  for the first message of a new post.
- ADR-0002, "Store as little as possible": "It sees only what a member passes
  to a command." The bot also sees the title and the first message of a new
  post in the forum. It still subscribes to no message events.
- ADR-0003: "In the first version the reply is visible only to the member who
  asked". This stands for `/ask`. The reply in the forum is public.

## Open points

- **Model-written replies in the forum.** Not part of this record. They would
  be a separate, later record.
- **Legal review.** Three points go into the legal research before launch and
  are launch gates of the feature
  ([Support forum answer](../compliance/support-forum-answer.md), "Before
  launch"): whether legitimate interests carries the reading of text that was
  not addressed to the bot; whether the guidelines text and the privacy notice
  inform a member in time (Articles 13 and 21(4) of the GDPR); what follows
  when a post holds data of the kinds Article 9(1) lists.
- **Private threads.** The server rules and the privacy notice named "a
  private thread in the support forum" as a way to reach the operator.
  Discord's documentation says that private threads can only be created in
  text channels. The operator decided that this path moves to a ticket
  channel; the two texts change with that decision.
- **Not tried.** That the HTTP API returns the text while the gateway session
  passes no content intent, and that the first message can be requested at once
  after the thread event, are read from the documentation. The live test
  proves both.

## Decided in this record

The operator decided that the bot answers first in the support forum from the
documentation, that it reads the full text of the post, that the right is used
only for new posts in that forum, that the reply is public and labelled as
automated, that `/ask` keeps its private reply, that the accepted records are
superseded by a new record, that the privacy documents get their entries before
launch, and that switching the right on in the developer portal is the
operator's own act. The operator accepted this record on 2026-10-11 and added
that a member can put a tag on a post so that the bot neither reads nor answers
it.

The choices below were made here and are accepted with this record:

- The text is requested through the HTTP API; the gateway session requests one
  standard intent and no message intent.
- The bot reads once, at creation: no later message, no edit, no post from
  before its start.
- Nothing is kept beyond the attempt, and the logs gain no field.
- The forum is answered with excerpts only; the text of a post never reaches
  the model endpoint.
- The bot stays silent when it finds nothing, when the attempt fails and when
  the text is empty, and it refuses to start without the right.
- The opt-out tag is named by a setting, counts at creation only, and the
  feature does not start without it.
- The wording of the label and the list of cases.

## References

- Operator decision of 2026-10-11 on how the bot answers first in the support
  forum (paraphrased): read the full text of the post, chosen over an answer
  from the title, with the costs named above.
- Operator decision of 2026-10-11 on this record (paraphrased): accepted as
  written, with a tag that a member can put on a post so that the bot neither
  reads nor answers it.
- Discord developer documentation, repository `discord/discord-api-docs` at
  commit `37148620d232` of 2026-10-09: `developers/events/gateway.mdx`
  ("Privileged Intents", "HTTP Restrictions", "Message Content Intent", the
  list of intents), `developers/events/gateway-events.mdx` ("Thread Create"),
  `developers/topics/threads.mdx` ("Forums", private threads),
  `developers/resources/channel.mdx` (channel types, forum tags, "Start Thread
  in Forum or Media Channel"), `developers/resources/message.mdx` ("Get Channel
  Message"),
  `developers/resources/application.mdx` (application flags):
  <https://github.com/discord/discord-api-docs/tree/37148620d2324292b76047541d5f8331f81ea77a>
- Regulation (EU) 2016/679, Articles 5(1)(c), 6(1)(f), 9(1), 13, 21(4) and
  25(2), as published:
  <http://publications.europa.eu/resource/celex/32016R0679>
