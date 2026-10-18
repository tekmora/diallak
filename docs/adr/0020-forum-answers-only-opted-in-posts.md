# ADR-0020: The forum answer reads only posts a member opted in

- **Status**: Accepted
- **Date**: 2026-10-17
- **Authors**: Maintainers
- **Amends**: [ADR-0009](0009-support-forum-first-answer.md), "A member can
  keep the bot away from a post" and "The label";
  [ADR-0018](0018-forum-guidelines-name-the-right-to-object.md), the
  guidelines text it keeps

## Context

[ADR-0009](0009-support-forum-first-answer.md) has the bot read the first
message of every new post in the support forum, unless the member sets the
opt-out tag "No automated reply" when creating the post.

[Legal research](../compliance/legal-research.md), point 9, found that a post
can hold data of the kinds GDPR Article 9(1) lists, such as health data, and
that the prohibition applies whatever the bot looks for:

- The Court holds the prohibition independent of the controller's aim
  (C-252/21, paragraph 69) and applies it to data that reveal a category only
  indirectly (C-184/20, paragraph 127).
- Article 6(1)(f) alone does not lift it.
- The exception for data manifestly made public (Article 9(2)(e)) covers only
  the poster's own data and only a clear choice to make it public to everyone;
  no other exception fit the facts.
- One read, nothing kept and the opt-out tag lower the impact. They are not an
  exception.

[Issue 146](https://github.com/tekmora/diallak/issues/146) asked for a
decision before the forum feature
([issue 39](https://github.com/tekmora/diallak/issues/39)) is built. The
operator decided that the bot reads only posts that carry a tag the member
sets to ask for the automated answer.

ADR-0009 and ADR-0018 are accepted, so their text stays as it is and this
record amends them ([records](README.md)).

## Decision

### The bot reads a post only when the member asks for it

A new post is read and answered only when it carries the opt-in tag at its
creation. A post without the tag is neither read nor answered; the bot does
not request its first message and does not use its title.

- One setting names the tag. The operator creates the tag in the forum and
  leaves it free for every member to set; its name is "Automated answer".
  The tag id never enters the tree.
- With the forum configured and no tag configured, the start stops, as it did
  for the opt-out tag.
- The tag counts when the post is created, because the bot reads once, at
  that moment. A tag added later does not make the bot read the post, and a
  tag removed later does not take a sent reply back.
- The opt-out tag of ADR-0009 goes away. The default is now that the bot does
  not read.

### The guidelines text

The guidelines text of the forum becomes:

> The automated assistant reads a new post only when you add the tag
> 'Automated answer' when you create it. It then reads the first
> message and may reply first with excerpts from the documentation. It reads
> nothing else here. Do not ask for it on a post that holds health data or
> other sensitive data, or data about other people. More is in the privacy
> notice.

The paragraph on the right to object from ADR-0018 follows unchanged, as a
paragraph of its own. The two reply lines of ADR-0009 stay as they are.

### The ground under Article 9 is not settled

The tag is a clear act of the member, set for one post, before anything is
read. It is meant to come close to explicit consent under Article 9(2)(a).
Whether it fits is **unverified**:

- Explicit consent needs a statement that is specific, informed and freely
  given (Articles 4(11), 7 and 9(2)(a)); whether a tag next to the guidelines
  text meets this has not been researched.
- A post can hold data about other people, whose consent the tag does not
  give.
- Withdrawing consent (Article 7(3)) has nothing to stop: the bot reads once
  and keeps nothing. Whether that is enough has not been checked.

The research point stays open in the legal research until it is checked with
a source.

### Follow-up with the forum feature

These change when the forum feature is built, not with this record. All
three are done:

- [x] The server definition: the forum's `opt_out` tag becomes an opt-in tag,
  and the `first-answer` check of the definition asks for exactly one (#169;
  `server/definition.json`, tag "Automated answer";
  `internal/serverdef/review_test.go`, "exactly one opt_in").
- [x] The drafts in [support forum answer](../compliance/support-forum-answer.md)
  and the privacy notice: the opt-in tag and the new guidelines text (#169;
  privacy notice, rows on the tag "Automated answer").
- [x] The acceptance cases 17 to 19 of ADR-0009, for the opt-in tag: the tests
  that name them are in `internal/discord/forum_test.go` and
  `internal/config/forum_test.go` ([support forum
  answer](../compliance/support-forum-answer.md), gates).

## Consequences

- **Positive**: The bot reads no post the member did not ask it to read, so
  special-category data reach it only after the member's own act.
- **Positive**: A member who does not set the tag needs no action to keep the
  bot away.
- **Negative**: Fewer posts get a first answer, because members have to know
  and set the tag.
- **Negative**: The fit under Article 9(2)(a) stays unverified, so the
  feature still carries the open point of the legal research.

## Alternatives considered

- **Keep the opt-out tag.** Research point 9 found no exception of Article
  9(2) that fits reading every post by default.
- **Drop the forum feature.** Removes the risk and the feature with it; `/ask`
  stays as the way to ask the bot.
- **Rely on Article 9(2)(e).** It covers only the poster's own data and only
  where the server is open to the general public, which is unverified.

## References

- [Legal research](../compliance/legal-research.md), point 9, with the sources
  quoted there (GDPR Article 9(1) and (2), C-252/21, C-184/20).
- [Issue 146](https://github.com/tekmora/diallak/issues/146) and
  [issue 39](https://github.com/tekmora/diallak/issues/39).
- Operator decision of 2026-10-17 on issue 146 (paraphrased): the forum answer
  reads only posts that carry a tag the member sets to opt in; the record notes
  that the fit under Article 9(2)(a) is unverified.
