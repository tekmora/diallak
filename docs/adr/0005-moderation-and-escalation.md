# ADR-0005: Moderation and escalation

- **Status**: Proposed
- **Date**: 2026-10-10
- **Authors**: Maintainers

## Context

A public community needs moderation, and a bot must not be its judge. The
operator decided the division of work: the bot filters spam and obvious abuse,
people decide everything unclear.

Moderation also touches member data more than any other feature, so this record
fixes who may do what before anything is built. No person is named here or in
any file, issue or log of this repository; roles are named instead.

## Decision

### Three instances, in this order

1. **Automatic filtering** acts only on spam and obvious abuse.
2. **Moderators** decide every unclear case.
3. **The operator** is the last instance and decides what moderators escalate.

The bot escalates; it does not punish on its own judgement. An unclear case goes
to the moderators with a link to the message and the reason it was flagged.

### The platform filters; the bot handles what is flagged

Spam and obvious abuse are filtered by the platform's own moderation rules. The
bot does not read every message. It handles only what those rules flag: it puts
the case in front of the moderators and records what is done.

- The bot requests no privileged intent for moderation
  ([ADR-0001](0001-architecture-and-boundaries.md)), and message content stays
  out of its stores.
- What the platform's rules can do is verified against Discord's current
  documentation before the build. The result is recorded here.

### Two role templates

| Role | May | May not |
| :--- | :--- | :--- |
| Moderator | Delete messages; time members out | Ban; manage channels, roles |
| Subject guide | Be the visible helper for one topic | Moderate |

A person can hold both roles. The moderator role has the least permissions that
do the job. The subject guide role carries no moderation permission by itself.

### The bot assigns no role

Inviting a person and granting a role are the operator's own acts. The bot
grants neither the moderator role nor a subject guide role, and it announces
nothing about a person before the operator says they accepted.

### Moderator actions leave a trail the operator can read

Every moderator action through the bot is recorded with its time, the role that
acted, the action and the affected message or member id. The trail holds no
message text beyond what the action needs, and its retention follows
[ADR-0002](0002-data-and-compliance.md).

### Written rules come first

Moderation starts only when two texts exist: a short code of conduct for
members, and a moderator guide that says what may be removed and when to
escalate. Both live under `docs/`.

## Consequences

- **Positive**: A member is never sanctioned by a model's judgement alone.
  Moderators can do their job without rights that could damage the server. The
  operator can review every action.
- **Negative**: Unclear cases wait for a person. The trail is one more store of
  member data with its own retention period. Roles have to be kept in step with
  this record by hand, since the bot does not manage them.

## Open decisions

- Where escalations appear for moderators, and how the operator receives what
  moderators escalate.
- The retention period of the trail.
