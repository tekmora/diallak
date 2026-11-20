# ADR-0031: A person sets the bot entry on rules

- **Status**: Accepted
- **Date**: 2026-11-21
- **Authors**: Maintainers
- **Supersedes in part**: [ADR-0029](0029-the-bot-creates-the-hub-invite.md)
- **Supersedes in part**: [ADR-0030](0030-the-hub-invite-does-not-wait-for-a-name.md)

## Context

In own mode, the [planner](../../internal/serverplan/plan.go) writes access
entries only on area channels, for the role of that area. The hub plan refuses
the bot's entry on `rules` and refuses the invite because the bot lacks Create
Invite there. Giving Create Invite to the bot role makes the run raised; it
also offers to create both supporter roles that their integrations manage.

## Decision

A person sets the bot role's entry on `rules` by hand to match the
[definition](../../server/definition.json): allow Create Invite alone and
deny nothing. The own-mode plan then holds `create-invite invites[hub]`.
No role right is switched on or off for the invite. Follow
[A server the bot is already on](../server-apply.md#a-server-the-bot-is-already-on).

## Supersedes

The records stand otherwise. This record replaces these statements:

- ADR-0029, "Decision": "On the hub, the run writing that entry likewise needs
  Create Invite on the bot role in `rights needed:`, then a person switches it
  off".
- ADR-0030, "Decision": "A person switches on by hand the rights the plan
  lists under `rights needed:` and switches Create Invite off after the run,
  as described in that section."

## Consequences

Compare reports no drift for the entry when it matches the definition. The
bot's entry on `announcements` remains refused in own mode until a person
sets it as the definition gives it.

## Alternatives considered

- **Give Create Invite to the role and switch it off after the run.** Rejected:
  the raised run also creates the supporter roles alongside their integrations,
  and a missed switch-off leaves the role with an extra right.

## Open points

None.

## Decided in this record

A person sets the bot's entry on `rules` to match the definition before the
own-mode plan creates the hub invite.

## References

- [ADR-0029](0029-the-bot-creates-the-hub-invite.md) and
  [ADR-0030](0030-the-hub-invite-does-not-wait-for-a-name.md).
- [Planner](../../internal/serverplan/plan.go),
  [definition](../../server/definition.json), and
  [A server the bot is already on](../server-apply.md#a-server-the-bot-is-already-on).
