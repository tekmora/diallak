# ADR-0018: The forum guidelines name the right to object

- **Status**: Proposed
- **Date**: 2026-10-17
- **Authors**: Maintainers
- **Amends**: [ADR-0009](0009-support-forum-first-answer.md), "The label"

## Context

[ADR-0009](0009-support-forum-first-answer.md) has the bot read the first
message of a new post in the support forum and reply first. Its guidelines
text, shown before a member posts, names the tag "No automated reply" and
links the privacy notice. It does not name the right to object.

[Legal research](../compliance/legal-research.md), point 8, found that this
falls short of GDPR Article 21(4) for the forum:

- Article 21(4) asks that the right to object is "explicitly brought to the
  attention of the data subject" and "presented clearly and separately from
  any other information", at the latest at the first communication.
- The bot's public reply in the post is a communication with the member, and
  the member did not call the bot. The guidelines text is the first thing the
  member reads about the bot there.
- The tag keeps the bot away from one post. It is not the right of
  Article 21(1), which covers the reading itself, for every post.

For `/ask` the research reached a different reading (point 6): the member
calls the bot, and the linked notice gives the right its own section. That
reading is not changed here.

ADR-0009 is accepted, so its text stays as it is and this record amends it
([records](README.md)).

## Decision

The guidelines text of the support forum gains one sentence on the right to
object, as a paragraph of its own after the existing text:

> You can object at any time to the assistant reading your posts here: write
> to the e-mail address in the privacy notice, or open a private thread in
> the ticket channel. The privacy notice explains this under 'Your right to
> object'.

- The sentence stands apart from the text about the tag, so that the right is
  presented separately from other information and is not read as the tag.
- It names the two paths the privacy notice names, in the same order: e-mail
  as the binding path, the ticket channel next to it.
- The rest of ADR-0009's guidelines text, and its other two texts, stay as
  they are.
- The operator sets the guidelines text in the server, as ADR-0009 says.

## Consequences

- **Positive**: The member reads the right before the bot reads the member's
  first post, in the place where the bot acts.
- **Positive**: The tag and the right are no longer easy to confuse.
- **Negative**: The guidelines text grows by one paragraph.
- **Negative**: An objection has to be honoured for the member's later posts.
  The bot stores nothing about members today, so how it skips the posts of a
  member who objected is decided with the forum feature
  ([issue 39](https://github.com/tekmora/diallak/issues/39)); this record
  does not decide it.

## Alternatives considered

- **The right in the bot's reply.** The reply comes after the reading, so it
  would inform the member too late for that post, and it would repeat the
  sentence in every reply.
- **The tag as the way to object.** The tag works for one post and only when
  it is set at creation. It does not meet Article 21(1) for later posts.

## References

- [Legal research](../compliance/legal-research.md), points 6 and 8, with
  the sources quoted there (GDPR Articles 13(1) and 21(4), WP260 rev.01).
- [Issue 145](https://github.com/tekmora/diallak/issues/145).
