# ADR-0014: Roles other parties manage, notification roles and more recognition roles

- **Status**: Accepted
- **Date**: 2026-10-12
- **Authors**: Maintainers

## Context

The [server definition](../server-definition.md) listed seventeen roles. Three
things were missing from it.

- **Roles the server has and the tool does not own.** Discord gives a server a
  role for members who boost it, and an integration brings a role of its own.
  The compare run listed three such roles as extra (`role-extra`, detail
  `managed`): the booster role and the roles of two funding integrations. The
  definition could not say that they belong there.
- **Roles a community of this kind usually has.** The operator asked for a
  maintainer role, notification roles for releases and events, a helper role
  and four recognition roles.
- **How a role is shown.** Whether a role's members form a group of their own
  in the member list, and whether members can mention the role, was not data.
  Both are fields of Discord's role object (`hoist`, `mentionable`).

[ADR-0010](0010-bot-rights-and-apply-path.md) bounds what the bot may hold,
[ADR-0011](0011-area-roles-from-member-choice.md) lets a member choose roles
through the bot, and [ADR-0005](0005-moderation-and-escalation.md) keeps
moderator actions on one path with a trail. This record adds the roles inside
those bounds. It builds no function of the bot.

## Decision

### A managed role is checked and never changed

The definition gains the role kind `managed` for a role another party creates
and keeps. Its `assigned_by` names that party: `platform` for the booster
role, `integration` for a role an integration brought.

<!-- markdownlint-disable MD013 -->
| Role | Managed by | What the compare run checks |
| :--- | :--- | :--- |
| Server Booster | the platform | exists; is managed; is shown apart |
| Ko-fi Bot | its integration | exists; is managed; stands below Moderator |
| Patreon | its integration | exists; is managed; stands below Moderator |
<!-- markdownlint-enable MD013 -->

- **The tool never creates, changes or deletes a managed role.** The plan
  holds no step for one, whatever the compare run finds, and the apply step
  returns an error before any call if a plan names one.
- **A managed role stands outside `role_order`.** The tool does not place it.
  A funding role above Moderator is reported (`role-place`), and the operator
  drags it down. The bound itself is the operator's decision. What it keeps
  apart: an integration gives its role for a payment, and the roles above
  Moderator are given by the operator alone.
- **A managed role holds no right in the definition and no channel entry.**
  The compare run does not judge the rights the other party gave it.
- **It is found by name and by the platform's managed flag.** A role of that
  name that a person made by hand is reported (`role-not-managed`).
- **A managed role that is missing** is reported as `role-missing` with the
  note that the plan does not create it. It appears when the integration is
  connected or, for the booster role, when Discord creates it.

Discord marks its booster role with a tag on the role. The library this
repository uses decodes that tag as false whether it is present or not, so
the tag is not read; see References.

### Two display fields, compared only where the definition states them

A role may state `shown_apart` (its members form a group of their own in the
member list) and `mentionable` (members can mention the role).

- **A field the definition does not state is not compared and not changed.**
  The Operator role, which the operator makes by hand, can therefore never
  cause a refusal over how it is shown.
- **A stated field that differs** is one difference (`role-display`) and one
  plan step (`set-role-display`). The step needs the right to manage roles.
  Like every role change it belongs to a raised run: with the bot's own rights
  the plan refuses it. It is also refused for a role above the bot's own and
  for a server role another party manages.
- **The booster role's display is set by hand.** The definition states that it
  is shown apart, the compare run reports while it is not, and the plan holds
  no step for it. Discord's pages do not say whether a bot may change the
  display of a role the platform manages.
- **Where a field must not be stated.** The validator refuses `mentionable`
  on a managed role, and either field on the bot's own role and on the role
  every member holds.
- **A colour is not data.** Nothing compares or sets it.

### Notification roles are chosen by the member and mentionable by everyone

Two roles of the new kind `notification`: "Notify: releases" and "Notify:
events". A member takes and drops them through the same `/areas` command as
the interest and language roles; the definition's `choices` gain a third
question for them.

Both roles state `mentionable: true`. Discord lets a role be mentioned by
whoever holds the right "Mention @everyone, @here, and All Roles", or by
everyone when the role's own setting allows it. ADR-0010 keeps that right
from the bot, and it mentions far more than two roles. The role's own setting
is the narrower way, so the notification roles use it.

The validator holds a notification role to the rules of an area role: no
right, no channel allow, assigned by choice, the option of exactly one
question, below the bot's role in `role_order`. It also refuses a notification
role that does not state `mentionable: true`.

### Six recognition roles the operator gives

<!-- markdownlint-disable MD013 -->
| Role | For whom |
| :--- | :--- |
| Maintainer | maintains one of the projects and speaks for it on the server |
| Helper | helps members; shown apart in the member list |
| Translator | translated one of the projects or its documentation |
| Tester | tests builds before a release and reports what fails |
| Creator | makes guides, videos or other material about the projects |
| Partner | speaks for a project or community this one works with |
<!-- markdownlint-enable MD013 -->

- **None of them carries a right.** They are of kind `recognition`, and the
  validator refuses a right or a channel allow on that kind.
- **The Helper role is not a small moderator role.** It sees no staff channel
  and deletes no message. A helper reports through the path every member has.
  Deleting a message stays with moderators acting through the bot, so the
  trail of ADR-0005 has no second path beside it.
- **The operator gives them by hand**, so they stand above the bot's role in
  `role_order` and the bot can neither give nor change them (ADR-0010, "The
  role order bounds the right to manage roles").

The order of roles, from the highest: Operator, Moderator, Maintainer, Helper,
the two subject guide roles, Translator, Tester, Creator, Partner, the bot's
role, Contributor, the two supporter roles, the two notification roles, the
interest roles, the language roles.

### What boosting and supporting give

- **Boosting** shows the member as a group of their own in the member list.
  It gates nothing, and no right of the bot follows from it.
- **Supporting** gives a role and a credit. There is no channel for supporters
  only: early builds are announced in a channel every member reads. Nothing is
  sold.

## Consequences

- **Positive**: The compare run stops listing three expected roles as extra,
  and it now notices when one of them is missing, replaced by a hand-made role
  or placed above the moderators.
- **Positive**: No new right. The numbers of the invitation address and of the
  bot's every-day rights stay as they are
  ([Applying the server definition](../server-apply.md)).
- **Positive**: Display is data where it matters and stays untouched where the
  definition is silent.
- **Negative**: Every member can mention a notification role. A member who
  took the role agreed to mentions for releases and events, not to mentions by
  anyone for anything. Misuse is a case for the moderators.
- **Negative**: Two hand acts stay with the operator and are not checked by a
  run that changes anything: the booster role's display, and the place of a
  funding role.
- **Negative**: A managed role is found by its name. If the other party
  renames its role, the compare run reports the entry as missing and the role
  as extra until the definition follows.
- **Negative**: The first set-up creates eight more roles, and the place of
  the bot's own role moves: it now stands below Partner.

## Alternatives considered

- **Leave managed roles out and accept them as extra.** The compare run would
  keep printing three lines nobody acts on, and it could not tell a missing
  integration from a present one.
- **Recognise the booster role by its tag.** It is what Discord documents. The
  library decodes the tag as false in both cases (References), so the reader
  would have had to parse the role a second time, beside the library.
- **Give the bot the right to mention all roles** and keep the notification
  roles unmentionable for members. The bot would then be the only one who can
  announce. ADR-0010 excludes the right, and it reaches every role of the
  server.
- **A helper role that deletes messages.** It was put to the operator and
  declined: a second way to delete would stand beside the moderators' path
  and outside its trail.
- **Place managed roles through the plan.** Moving a role needs it to stand
  below the bot's own role, which is a place the tool does not decide for a
  role it does not own.
- **More for boosters:** the use of other servers' emoji and stickers, the
  soundboard, an emoji set of their own. Declined by the operator; display
  only.

## Supersedes

This record changes single statements. The records stand as they are.

- [ADR-0010](0010-bot-rights-and-apply-path.md), "The role order bounds the
  right to manage roles": the list of six steps becomes the order above, and
  "every role except the one every member holds is listed exactly once" now
  also excepts the managed roles.
- [ADR-0011](0011-area-roles-from-member-choice.md), "What changes in the
  definition": a choice may also give a notification role, and the command
  shows three menus instead of two.

## Open points

- **Not tried against Discord.** That the display controls are offered on the
  platform's own booster role; that a member can mention a role whose setting
  allows it without holding the mention right, exactly as the help page says;
  that the names of the two funding roles are what a newly connected
  integration creates today. The names were read from the server on
  2026-10-12.
- **Where a new role lands.** Discord's page for creating a role does not say
  at which place the new role stands. If it stands at the bottom, a funding
  role that was on the server before the first set-up stands above Moderator
  afterwards: the plan orders the roles it knows and leaves every other role
  where it is. The compare run then reports `role-place` for it until the
  operator drags it down. The tests of the apply step model this case.
- **Who may be mentioned how often.** Whether the platform's spam filter
  limits mentions of a role is not read, and nothing here rests on it.
- **How the bot learns that a release is out or an event is announced,** and
  whether it or a person writes the mention, belongs to the record for the
  news posts.

## Decided in this record

The operator decided: the two funding roles get entries marked as managed by
their integration, which the plan never creates, changes or deletes and which
are checked for existence and for a place below Moderator; the definition
gains a maintainer role, notification roles for releases and events, a helper
role and the four recognition roles; the notification roles are mentionable by
members; the helper role carries no right and sees no staff channel; boosting
is shown in the member list and gates nothing, with the display set by hand;
supporters get a role and a credit and no channel of their own.

The choices below were made here and are accepted with this record, open to
the operator's veto:

- The role kinds `managed` and `notification`, and the value
  `assigned_by: integration`.
- Managed roles stand outside `role_order` and are found by name and managed
  flag.
- The display fields are optional, and silence means "not compared".
- The compare kinds `role-not-managed`, `role-place` and `role-display`, and
  the plan step `set-role-display`.
- The notification roles are a third question of the `/areas` command, not a
  command of their own.
- The place of the six new recognition roles in the order.
- The booster role has no place check.

## References

- Operator decisions of 2026-10-12 (paraphrased): entries for the two funding
  roles, marked as managed and only checked; the further roles and their
  names, with the notification roles mentionable by members because the other
  way needs a right ADR-0010 excludes; a helper role without rights; a display
  perk for boosters with the display set by hand; no channel for supporters
  only.
- Discord developer documentation, repository `discord/discord-api-docs` at
  commit `37148620d232` of 2026-10-09: `developers/topics/permissions.mdx`
  (role object with `hoist`, `managed`, `mentionable` and `tags`; "Role Tags
  Structure": a tag of type null is present and null when true and absent when
  false; `premium_subscriber` marks the booster role),
  `developers/resources/guild.mdx` ("Create Guild Role" and "Modify Guild
  Role" take `hoist` and `mentionable` and need `MANAGE_ROLES`):
  <https://github.com/discord/discord-api-docs/tree/37148620d2324292b76047541d5f8331f81ea77a>
- Discord Help Center, "Role Mention Permissions & Suppression", last edited
  2020-04-06, read 2026-10-12: the mention right lets its holder mention a
  role whose setting "Allow anyone to @mention this role" is off:
  <https://support.discord.com/hc/en-us/articles/360039176412>
- Library `github.com/disgoorg/disgo` v0.19.6, `discord/role.go`: the role
  tag's field for the booster role is a plain boolean. A role decoded from
  `"tags": {"premium_subscriber": null}` and one without the tag both give
  false; measured on 2026-10-12 with a test kept outside this repository.
- The reading of the server's roles on 2026-10-12 is kept outside this
  repository.
