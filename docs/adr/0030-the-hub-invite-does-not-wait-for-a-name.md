# ADR-0030: The hub invite does not wait for a name

- **Status**: Accepted
- **Date**: 2026-11-20
- **Authors**: Maintainers
- **Supersedes in part**:
  [ADR-0029](0029-the-bot-creates-the-hub-invite.md), when the live apply runs
  and when the links are made

## Context

[ADR-0029](0029-the-bot-creates-the-hub-invite.md) and the
[product plan](../product.md) tie the invite in step 5 to the naming step 4.
The naming step is set aside for now. An invite code holds no bot name; the
invite object identifies its inviter as a user object
([Discord invite API](https://github.com/discord/discord-api-docs/blob/main/developers/resources/invite.mdx)).

## Decision

The live apply, recording the code, and adding the profile and README links go
ahead without step 4. The cluster session builds the bot from source at a main
commit and follows the
[existing-server path](../server-apply.md#a-server-the-bot-is-already-on) for
a server the bot is already on.
A person switches on by hand the rights the plan lists under `rights needed:`
and switches Create Invite off after the run, as described in that section.
The links are made in one batch per organisation
([#254](https://github.com/tekmora/diallak/issues/254)).

## Supersedes

This record replaces two statements in ADR-0029. The record stands otherwise.

- "Decision": "After the rename, the operator's cluster session runs the live
  apply".
- "Open points": "The README and profile links wait for the name in step 4,
  and are made in one batch per organisation". The batch rule remains.

## Consequences

The invite and links can be published while the naming step remains set aside.
The recorded code and subsequent comparison follow the existing
[existing-server path](../server-apply.md#a-server-the-bot-is-already-on)
and the
[invite definition](../server-definition.md#invites).

## Alternatives considered

- **Wait for step 4.** Rejected: the profile and README links would remain
  absent for an unknown time.

## Open points

None.

## Decided in this record

The live apply, recording the invite code and making the links proceed without
the naming step.

## References

- [ADR-0029](0029-the-bot-creates-the-hub-invite.md), the hub invite.
- [Discord invite API](https://github.com/discord/discord-api-docs/blob/main/developers/resources/invite.mdx),
  as referenced by ADR-0029.
- [Product plan](../product.md),
  [existing-server path](../server-apply.md#a-server-the-bot-is-already-on),
  [invite definition](../server-definition.md#invites), and
  [#254](https://github.com/tekmora/diallak/issues/254).
