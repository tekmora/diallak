# Support forum answer: entries for launch

Draft, and not in force. The feature it describes is not built;
[ADR-0009](../adr/0009-support-forum-first-answer.md) is its accepted record.
The other documents in this directory describe the code on `main` and say
nothing of the forum yet.

This page holds the entries those documents gain when the feature is built.
The pull request that builds it moves each entry to its place and removes this
page. It is not legal advice; the legal review reads these entries with the
rest of the [privacy notice](privacy-notice.md).

## In short

- In the support forum the bot reads the title and the first message of a new
  post, once, and may reply first with excerpts from the documentation.
- It reads no other message, in the forum or anywhere else.
- A post that carries the opt-out tag is neither read nor answered.
- It keeps the text in memory until the reply is sent, stores nothing and logs
  no text and no id.
- The text is sent to nobody. No model writes the reply.

## Privacy notice

### "In short", two more points

- In the support forum the bot reads the title and the first message of a new
  post, once, to find documentation that answers it. It reads no other message.
- Its reply there is public, like the post, and holds documentation text, not
  your text.
- To keep the bot away from a post, add the tag "No automated reply" when you
  create it.

### "What is processed, for what purpose and on what basis", new rows

<!-- markdownlint-disable MD013 -->
| Data | Purpose | Legal basis |
| :--- | :--- | :--- |
| Title and first message of a new post in the support forum | Finding documentation passages for a first reply in that post | Legitimate interests, Article 6(1)(f) |
| Whether a post was written by an application | Leaving such a post without a reply | Legitimate interests, Article 6(1)(f) |
| Whether a post carries the tag "No automated reply" | Leaving such a post unread | Legitimate interests, Article 6(1)(f) |
| Other fields Discord sends with a post or with events about channels, roles and threads, such as the id of the member who created a thread | None: the bot compares the channel and the kind of thread and reads nothing else | Not used |
<!-- markdownlint-enable MD013 -->

The legitimate interest is that a question in the support forum gets a first
answer from the documentation at once. You did not call the bot for this, so
the forum's guidelines say that it reads the first message before you post.

### "Your right to object", one more paragraph

In the support forum you can keep the bot away from a single post: add the tag
"No automated reply" when you create the post. The bot then does not read the
post and does not reply. A tag you add later does not take a reply back,
because the bot reads a post once, when it is created.

### "Who receives your data", changed points

- **Discord** receives the reply, which is public in your post.
- **The model endpoint** receives nothing from the support forum. Posts are
  answered with excerpts only.

### "How long data is kept", new row

<!-- markdownlint-disable MD013 -->
| Data | Kept |
| :--- | :--- |
| Title and first message of your post, in the bot | In memory until the reply is sent or the attempt ends |
<!-- markdownlint-enable MD013 -->

The reply stays in your post under Discord's terms. The bot keeps no copy of it
and no reference to it. A moderator can remove it.

### "What you should not send", one more sentence

The same holds for a post in the support forum: the bot reads its first
message.

## Data inventory

### "What the bot receives", new rows

<!-- markdownlint-disable MD013 -->
| Data | From | Used for | Kept |
| :--- | :--- | :--- | :--- |
| Title of a new post in the support forum | Discord, with the thread event | Finding passages | In memory until the reply is sent or the attempt ends |
| Text of the first message of that post, up to a fixed length | Discord, on the bot's request for that one message | Finding passages | In memory until the reply is sent or the attempt ends |
| Whether the author of that message is an application | Discord, with the message | Leaving such a post without a reply | Not kept |
| Whether a new post carries the opt-out tag | Discord, with the thread event | Leaving a tagged post unread: its first message is not requested | Not kept |
| Events about channels, roles and threads of the server, with the title of each new thread and the id of its creator | Discord, with the standard intent that carries thread events | Nothing, except telling a new post in the configured forum from everything else by the id of the parent channel and the kind of thread | Not kept |
| Other fields of the first message: author id and name, attachments, embeds, polls, components | Discord, with the message | Nothing: the bot reads none of them | Not kept |
<!-- markdownlint-enable MD013 -->

The check line of that section changes with the build: the gateway session
then requests one standard intent and no message intent, so no message event
reaches the bot, and the first message is requested through the HTTP API.

### "What the bot stores"

Unchanged: nothing.

### "What the bot logs"

Unchanged fields. New values of the reason, such as one for an empty first
message, are not personal data. The log holds no title, no text, no post id
and no member id.

### "What the bot passes on", new row

<!-- markdownlint-disable MD013 -->
| To | What | When |
| :--- | :--- | :--- |
| Discord | A public reply in the post, with excerpts from the documentation | A new post in the support forum for which a passage is found |
<!-- markdownlint-enable MD013 -->

The model endpoint receives nothing from the support forum.

### "What Discord holds"

Discord holds the post and the reply under its own terms.

## Record of processing

<!-- markdownlint-disable MD013 -->
| Article 30(1) | Addition |
| :--- | :--- |
| (b) Purposes | Giving a first answer from the documentation to a new post in the support forum |
| (c) Data subjects and data | Members who open a post in the support forum: its title and the text of its first message |
| (d) Recipients | Discord, which shows the public reply. No other recipient |
| (f) Time limits for erasure | Title and text: in memory until the reply is sent or the attempt ends |
| (g) Security measures | The bot receives no message event; it requests one message per new post in one channel. The application holds the platform's right to read message content and uses it for that request only |
<!-- markdownlint-enable MD013 -->

The sentence "The bot requests no gateway intent" under "Security measures"
changes with the build.

## AI disclosure

New rows for "Where the notice appears". The texts are accepted with ADR-0009.

<!-- markdownlint-disable MD013 -->
| Place | Text | State |
| :--- | :--- | :--- |
| Guidelines of the support forum, shown before a member posts | "The automated assistant reads the first message of a new post and may reply first with excerpts from the documentation. It reads nothing else here. To keep it away from a post, add the tag 'No automated reply' when you create it. More is in the privacy notice." | Accepted with ADR-0009, set by the operator |
| First line of a reply in the support forum | "Automated excerpts from the documentation. No AI model wrote this reply." | In the code, used by `/ask` today |
| Last line of a reply in the support forum | "Automated first reply. A person can help here in this post." | Accepted with ADR-0009, not in the code |
<!-- markdownlint-enable MD013 -->

## Server rules

Rule 5 gains one sentence after "check the sources it cites":

> In the support forum it reads the first message of a new post and may reply
> first, unless the post carries the tag "No automated reply".

## Before launch

- The legal research has judged three points, with the other points of the
  privacy notice:
  - whether legitimate interests carries the reading of text that was not
    addressed to the bot;
  - whether the guidelines text and the privacy notice inform a member in time
    (Articles 13 and 21(4) of the GDPR);
  - what follows when a post holds data of the kinds Article 9(1) lists.
- The entries above are in their documents, and the privacy notice is
  published ([compliance](README.md), "Gates for the public launch").
- The implementation passes the cases of ADR-0009.
- The operator has created the opt-out tag, has set the guidelines text of the
  forum and has switched the right to message content on in Discord's
  developer portal.
- The live test has shown the two points ADR-0009 lists as not tried.
