# ADR-0008: Where the code of conduct lives

- **Status**: Accepted
- **Date**: 2026-10-10
- **Authors**: Maintainers

## Context

[ADR-0005](0005-moderation-and-escalation.md) requires two texts before
moderation starts: a short code of conduct for members and a moderator guide.
It says that both live under `docs/` of this repository.

The operator has since decided that one code of conduct holds for all spaces of
the organisation and is kept once per organisation. A copy in this repository
would be a second source that can drift from the first.

## Decision

- The code of conduct lives once per organisation, not in this repository. This
  repository links it and holds no copy.
- The [server rules](../compliance/server-rules.md) are a short add-on for what
  is specific to the server and point to the code of conduct.
- The moderator guide stays under `docs/` of this repository, as ADR-0005 says.
- The gate of ADR-0005 stands with this change: moderation starts only when the
  code of conduct exists at the organisation and the moderator guide exists
  here.

## Consequences

- **Positive**: One text to maintain, and the same rules of conduct in every
  space of the organisation.
- **Negative**: This repository depends on a text outside it. The link is
  checked when moderation is built.

## Supersedes

The sentence "Both live under `docs/`." in ADR-0005, section "Written rules
come first", for the code of conduct only. Everything else in ADR-0005 stands.

## Decided in this record

The operator decided that there is one code of conduct per organisation and
that the server rules are an add-on to it. That this repository holds no copy
follows from that decision. The choices made here: the moderator guide stays in
this repository, and the gate of ADR-0005 counts the code of conduct at the
organisation.
