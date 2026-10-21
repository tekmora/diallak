# Moderator guide

The commands it names are decided in
[ADR-0021](adr/0021-moderation-commands-and-trail.md). Until the bot serves
them, only the operator moderates.

This guide is for the people who hold the Moderator role on the server. It
says what you may remove, how long a timeout may be, when a case goes to the
operator, and what you never do. Members read the
[server rules](compliance/server-rules.md); this guide binds you when you act
on them.

Moderation starts only when this guide and the organisation's
[code of conduct](https://github.com/tekmora/.github/blob/main/CODE_OF_CONDUCT.md)
exist ([ADR-0005](adr/0005-moderation-and-escalation.md),
[ADR-0008](adr/0008-code-of-conduct-place.md)).

## How you act

The bot's two commands are the only way you act
([ADR-0015](adr/0015-moderator-without-rights-and-server-settings-as-data.md)).
The Moderator role holds no right of its own, so the app offers you no other
way, and none is needed.

<!-- markdownlint-disable MD013 -->
| To | Use | Give |
| :--- | :--- | :--- |
| Remove a message | `Remove message`, in the menu of the message (Apps) | A reason |
| Time a member out | `/timeout` | The member, the length in minutes, a reason |
<!-- markdownlint-enable MD013 -->

- Every act needs a reason. Without one the bot does nothing.
- A timeout lasts from 1 minute to 28 days. The bot refuses a longer one.
- Discord does not let anyone time out the server's owner or a member who
  holds Administrator. Such a case goes to the operator.
- The bot acts on one message or one member per call.

The commands are described in
[ADR-0021](adr/0021-moderation-commands-and-trail.md).

## What the trail records

Every act writes one line into the moderators' log channel: the time, you as
the moderator who acted, the act, the affected member id (and for a message,
its channel and message id) and your reason. The line holds no text of the
removed message. The bot deletes each line after 90 days.

Write the reason for the operator who reads it later: which rule, and what
happened. Do not copy the removed text into it, and do not add personal data
about the member beyond what the case needs. The reason also appears in the
server's audit log.

The trail is the only record. There is no list of warnings and no note on a
member.

The remove command refuses a line in the log channel: the lines stay until the
bot deletes them after 90 days.

## What you may remove

Remove a message when it breaks the [server rules](compliance/server-rules.md)
or the code of conduct they point to, for example:

- harassment, hate speech or threats;
- personal information about someone without their consent;
- sexual, gory or shocking content;
- spam, advertising, unsolicited invitations or repeated posts;
- scams, phishing or malware;
- pirated software or help with getting it;
- a security problem posted in public: remove it and point the member to the
  private path in the security policy.

A message that is only off-topic, wrong or rude in tone is usually answered,
not removed: point to the right channel or to the rules first.

## How long a timeout is

A timeout stops a member from writing for a time. Use it when removing a
message is not enough, for example when a member goes on after a reminder.

| Case | Length |
| :--- | :--- |
| Goes on after a reminder | 10 minutes to 1 hour |
| Spam or flooding | 1 hour to 1 day |
| Harassment, threats, hate speech | 1 day to 7 days, and escalate |
| Anything that needs longer than 7 days | Escalate; the operator decides |

The longest timeout is 28 days. A ban does not exist for moderators.

## When a case goes to the operator

Escalate in writing to the operator, as a request for a decision, when:

- a member should leave the server for good;
- a case needs a timeout longer than 7 days;
- the member is a moderator, a subject guide, the operator or holds
  Administrator;
- the case touches the law, a threat against a person, or a security problem;
- a member objects to an act (yours or another moderator's);
- you are not sure.

Put the link to the message or the member id and what you saw in the request.
The platform's filter puts flagged messages into the queue channel; an unclear
flagged case is yours to decide or to escalate.

## What a moderator never does

- Act without a reason, or act through anything but the two commands.
- Act on a case you are part of: hand it to another moderator or the operator.
- Decide a case on the bot's judgement alone. The bot flags and carries out;
  people decide.
- Grant or take roles, or promise a role to anyone. That is the operator's.
- Name or discuss the member's case in public channels.
- Copy messages, trail lines or member data out of the server.
- Ask a member for personal data.

## What the member is told

The bot sends the member no message. Discord shows a timeout to the member
while it lasts. When it helps, say in the channel that a message was removed
and which rule it broke, without naming the content. The server rules tell
the member how to object.
