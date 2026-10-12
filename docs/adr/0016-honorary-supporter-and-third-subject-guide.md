# ADR-0016: An honorary supporter role and a third subject guide role

- **Status**: Accepted
- **Date**: 2026-10-12
- **Authors**: Maintainers

## Context

The [server definition](../server-definition.md) holds two supporter roles,
"Supporter" and "Supporter for a year". The bot gives both: a planned function
gives and takes them from what the funding platforms report
([ADR-0014](0014-roles-managed-notification-recognition.md)). It also holds
two subject guide roles, for coding and for video, which the operator gives by
hand.

Two things could not be said in it.

- **Supporter standing that does not come from funding.** The operator wants
  to give a person the standing of a supporter for good, with everything that
  is attached to supporters, without a funding platform behind it and without
  anything on the server pointing at one.
- **A subject guide for software engineering.** The two guide roles name
  coding and video. Software engineering as a subject had no role.

The Supporter role cannot carry the first. The planned function takes that
role from a member the funding platforms no longer report. To keep it on one
member, the function would need a list of members it never touches: a member
id in the bot's stored data, and a promise that rests on code not yet written.

## Decision

### An Honorary supporter role, given by the operator

The definition gains the role "Honorary supporter" (`honorary-supporter`), of
kind `recognition`, given by the operator by hand.

- **It stands directly above the bot's role** in `role_order`. Discord lets a
  bot give, take and change only roles below its own
  ([ADR-0010](0010-bot-rights-and-apply-path.md), "The role order bounds the
  right to manage roles"). The bot can therefore neither give nor take this
  role, whatever function is built later. The validator refuses a definition
  that moves a role the operator gives below the bot's role
  (`TestRoleOrder_Boundary_NeighboursOfTheBot`).
- **It carries no right and states no display field**, like the two supporter
  roles.
- **It is tied to no funding platform.** No function reads a funding platform
  for it, its name does not name one, and its holder needs no role of a
  funding integration.
- **Who holds it is stored nowhere.** The role is given in Discord's own
  settings. No file of this repository, no configuration value and no data of
  the bot names a holder. The
  [data inventory](../compliance/data-inventory.md) gains no entry.

### Supporter standing

A member has supporter standing who holds "Supporter", "Supporter for a year"
or "Honorary supporter".

- **Whatever is attached to a supporter role is attached to the honorary
  role.** A right on one of the two supporter roles, or something an entry on
  a channel allows one of them, must also be given to the honorary role. Two
  tests hold that on the committed file and on planted gaps:
  `TestDefinition_Boundary_SupporterStandingHeldByHonorary` for entries on
  channels and `TestDefinition_Negative_SupporterStandingRolesAndRights` for
  the roles and their rights.
- **Today nothing is attached.** ADR-0014 gives supporters a role and a
  credit, no right and no channel. The rule binds the change that attaches
  something later.
- **The credit** names a person in public. Giving it is the operator's act,
  with that person's consent; nothing in this repository writes a name.

### A third subject guide role

The definition gains "Subject guide: software engineering"
(`guide-software-engineering`), beside the roles for coding and for video. It
is given by the operator, carries no right and stands with the other two in
the order.

The role for coding stays as it is. A person can hold more than one guide
role.

### The order

From the highest: Operator, Moderator, Maintainer, Helper, the three subject
guide roles (coding, video, software engineering), Translator, Tester,
Creator, Partner, Honorary supporter, the bot's role, Contributor, the two
supporter roles, the two notification roles, the interest roles, the language
roles. `TestDefinition_Positive_HonoraryAndThirdGuideRole` holds the place of
the two new roles.

## Consequences

- **Positive**: That the honorary role is never taken away by a function of
  the bot follows from the platform's role order, not from an exemption in
  code.
- **Positive**: No personal data. Nothing is added to what the bot stores, and
  the privacy notice stays as it is.
- **Positive**: No new right. The numbers of the invitation address and of the
  bot's every-day rights stay as they are
  ([Applying the server definition](../server-apply.md)).
- **Negative**: The first set-up creates two more roles, and the place of the
  bot's own role moves again: it now stands directly below Honorary supporter.
- **Negative**: Giving the role is one more hand act for the operator, and no
  run checks who holds it.
- **Negative**: Two guide roles overlap. Where coding ends and software
  engineering begins is for the operator to say when giving them.
- **Negative**: Supporter standing is now a rule over three roles. A change
  that attaches something to supporters has to name all of them; the test
  fails when it names the two bot-given ones only.

## Alternatives considered

- **Give the Supporter role by hand.** It needs no change today. Once the
  function for the supporter roles is built, it would take the role away
  unless it kept a list of members to leave alone. That list is a member id in
  stored data, a new entry in the data inventory and the privacy notice, and a
  promise that holds only as long as that code is right. The role's purpose
  also says financial support where there is none.
- **A list of honorary members in the bot's configuration.** The same stored
  id, in another place.
- **Use the coding guide role for software engineering.** It was put to the
  operator; the operator chose a role that names the subject.
- **Rename the coding guide role.** The tool finds a role by its name. What a
  renamed role does in the compare run and in a plan was not tried.

## Supersedes

This record changes single statements. The records stand as they are.

- [ADR-0014](0014-roles-managed-notification-recognition.md), "Six
  recognition roles the operator gives": the order of roles becomes the order
  above, with three subject guide roles and Honorary supporter between Partner
  and the bot's role.
- ADR-0014, "Consequences": the bot's own role stands below Honorary
  supporter, not below Partner.
- ADR-0014, "What boosting and supporting give": the role and the credit of a
  supporter are also held through the honorary role.

## Open points

- **Not tried against Discord.** Nothing of this record was run against the
  platform. The first set-up has not run; the two roles are created with it.
- **A role above the bot's role on a server that is already set up.** This
  record does not cover how a further role the operator gives is added once
  the bot's role stands at its place. It was not examined here.
- **What supporters get later.** A channel, a right or a function for
  supporters needs its own record; the rule above then says who it reaches.

## Decided in this record

The operator decided: a role for supporter standing that the operator gives by
hand, for good, tied to no funding platform and showing nothing of one; it
stands above the bot's role; a subject guide role for software engineering
beside the two that exist; no member id is stored anywhere.

The choices below were made here and are accepted with this record, open to
the operator's veto:

- The ids `honorary-supporter` and `guide-software-engineering`, and the
  wording of the two purposes.
- The honorary role stands directly above the bot's role, after Partner.
- The new guide role stands after the role for video.
- Neither role states a display field.
- Supporter standing is held by a test over the definition, not by a rule of
  the validator.

## References

- Operator decisions of 2026-10-12 (paraphrased): supporter standing for good
  through a role of its own that the operator gives, independent of any
  funding platform and with no reference to one; a subject guide role for
  software engineering; no stored member id.
- [ADR-0010](0010-bot-rights-and-apply-path.md), "The role order bounds the
  right to manage roles", for the platform's rule that a bot changes only
  roles below its own. This record adds no reading of Discord's pages.
