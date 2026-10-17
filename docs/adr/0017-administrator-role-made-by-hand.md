# ADR-0017: A role that grants Administrator is made by hand

- **Status**: Accepted
- **Date**: 2026-10-17
- **Authors**: Maintainers

## Context

The [server definition](../server-definition.md) gives the Operator role
`ADMINISTRATOR`. It is the only role that grants the right.

[ADR-0010](0010-bot-rights-and-apply-path.md) left one point open, "The
Operator role in the first set-up": either the apply path sets up the role,
or the operator makes it by hand. Until now the planner took neither path.
On a server without the role, the plan held
`create-role roles[operator] needs ADMINISTRATOR, MANAGE_ROLES`, because a
role is created with every right it gets and a bot gives a role only rights
it holds itself. That had two outcomes.

- With the rights of the first set-up the step was refused as `lacks-right`,
  and `rights needed:` named `ADMINISTRATOR`. A reader who follows that list
  gives the bot's role the one right ADR-0010 keeps from it.
- With `ADMINISTRATOR` on the bot's role, the plan had no refusal, and the
  bot would create an administrator role itself.

## Decision

A role that grants `ADMINISTRATOR` is made and given its rights by a person,
never by the apply path.

- The planner refuses `create-role` and `set-role-permissions` for such a
  role with the reason `made-by-hand`. The detail says what the person does:
  make the role by hand, with `ADMINISTRATOR`, above the bot's role, or give
  it its rights by hand.
- This reason is tested before every other one. Whatever else holds, the
  line names the hand act.
- The refusal needs no right of the bot's role. `rights needed:` therefore
  never names `ADMINISTRATOR` because of such a role.
- The refusal applies whatever rights the bot holds, `ADMINISTRATOR`
  included.
- A plan with the refusal is not applied, as with any refusal.

The first set-up in [Applying the server definition](../server-apply.md)
already has the operator make the role before the first plan ("A new
server", step 3).

## Consequences

- **Positive**: The bot's role holds `ADMINISTRATOR` at no step, and no list
  the tool prints asks for it.
- **Positive**: A server without the role stops at the plan with a line that
  says what to do, not halfway through the apply.
- **Negative**: One more act by hand before the first plan. The tool does not
  check who holds the role.
- **Negative**: The rule is keyed to the right, not to the role. A future
  role that grants `ADMINISTRATOR` falls under it without a further record.

## Alternatives considered

- **The path creates the role.** For that run the bot's role would hold
  `ADMINISTRATOR`, which is the path ADR-0010 did not choose for the bot's
  rights.
- **Leave the planner as it was and only document the step.** Then
  `rights needed:` keeps naming `ADMINISTRATOR`, and a bot that holds the
  right still creates the role.

## Supersedes

This record changes no accepted statement. It settles the open point "The
Operator role in the first set-up" of
[ADR-0010](0010-bot-rights-and-apply-path.md). That record stands as it is.

## Open points

- **Not tried against Discord.** The refusal is a planner rule and needs no
  call to the platform. The first set-up has not run.

## Decided in this record

The issue that asked for this record
([#83](https://github.com/tekmora/diallak/issues/83)) names making the role
by hand as the intended answer. The work was ordered through the controller.
The choices below were made here and are accepted with this record, open to
the operator's veto:

- The reason name `made-by-hand` and the wording of its two details.
- The rule is keyed to `ADMINISTRATOR` in the role's rights, not to the role
  id `operator`.
- The reason comes before every other refusal.

## References

- [ADR-0010](0010-bot-rights-and-apply-path.md), "Open points", for the open
  question, and "One apply path" for the bot's rights.
- `checkMadeByHand` in [`plan.go`](../../internal/serverplan/plan.go) and
  [`made_by_hand_test.go`](../../internal/serverplan/made_by_hand_test.go).
