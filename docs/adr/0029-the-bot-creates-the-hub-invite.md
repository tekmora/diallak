# ADR-0029: The bot creates the hub invite

- **Status**: Accepted
- **Date**: 2026-11-20
- **Authors**: Maintainers
- **Superseded in part by**:
  [ADR-0030](0030-the-hub-invite-does-not-wait-for-a-name.md),
  [ADR-0031](0031-a-person-sets-the-bot-entry-on-rules.md)

## Context

[Step 5 of the product plan](../product.md) and
[#254](https://github.com/tekmora/diallak/issues/254) call for one permanent
hub invite in organisation profiles and project READMEs. A permanent link
keeps those references useful without replacing an expired invite. The
[definition](../../server/definition.json) gives `@everyone` no
`CREATE_INSTANT_INVITE`, so members do not make invites through that role.

## Decision

The [definition](../../server/definition.json) holds an `invites` entry `hub`
for `rules`, with a purpose, `max_age: 0`, `max_uses: 0`, and
`temporary: false`. The [channel API](https://github.com/discord/discord-api-docs/blob/main/developers/resources/channel.mdx)
defines zero age as never expiring and zero uses as unlimited. The optional
`code` is empty until creation; a later pull request records the returned code
in the definition ([apply](../../internal/discord/server_apply_invites.go)).

The bot's entry on `rules` allows `CREATE_INSTANT_INVITE` alone, number 1
(bit 0). `@everyone` supplies `VIEW_CHANNEL`. The bot role keeps its four
server-wide rights: `MANAGE_ROLES`, `MANAGE_MESSAGES`, `MODERATE_MEMBERS` and
`MANAGE_GUILD`, or 2^28 + 2^13 + 2^40 + 2^5 = 1099780071456
([definition](../../server/definition.json),
[permission table](../../internal/serverdef/serverdef.go)). The two numbers
sum to 1099780071457 on `rules`; the 1 applies only there. Discord requires
`CREATE_INSTANT_INVITE` for `POST /channels/{channel.id}/invites` ([channel
API](https://github.com/discord/discord-api-docs/blob/main/developers/resources/channel.mdx)).
The setting is called Create Invite in English and Einladung erstellen in
German ([English](https://support.discord.com/hc/en-us/articles/10543994968087),
[German](https://support.discord.com/hc/de/articles/10543994968087)).

The first set-up address carries 1168500596785, formerly 1168500596784:
`CONNECT`, `CREATE_INSTANT_INVITE`, `CREATE_PRIVATE_THREADS`,
`MANAGE_CHANNELS`, `MANAGE_GUILD`, `MANAGE_MESSAGES`, `MANAGE_ROLES` and
`MODERATE_MEMBERS` (`TestServerApplyDoc_Negative_InvitationNumberIsThePlansRights`
in [the documentation test](../../cmd/diallak/server_apply_doc_test.go)). The
`create-channel` and `set-access` candidates need `MANAGE_CHANNELS` or
`MANAGE_ROLES` plus every allowed or denied bit in the entry
([planner](../../internal/serverplan/plan.go)). Discord permits a caller to
write only rights it holds ([permissions](https://discord.com/developers/docs/topics/permissions)).
The person tidying up removes Create Invite from the role; the channel entry
keeps it on `rules`. On the hub, the run writing that entry likewise needs
Create Invite on the bot role in `rights needed:`, then a person switches it
off ([first set-up](../server-apply.md#a-first-set-up)).

The reader lists invites when the caller has `MANAGE_GUILD` or
`ADMINISTRATOR`; otherwise it marks them unread
([reader](../../internal/discord/server_read_invites.go)). Discord's
[guild API](https://github.com/discord/discord-api-docs/blob/main/developers/resources/guild.mdx)
returns invite metadata including `uses`, `max_uses`, `max_age`, `temporary`
and `created_at` to a caller with `MANAGE_GUILD`. The snapshot keeps the
fields used for comparison ([snapshot](../../internal/serverplan/snapshot_invites.go)).

Compare reports `invites-not-read`, `invite-create`, `invite-missing`,
`invite-differs` or `invite-extra`. An unrecorded entry matches a bot invite
by channel and limits; a recorded entry matches by code and is then checked
for differences. Recorded entries match first; each entry takes at most one
invite and each invite is taken at most once. Unmatched bot invites are extra
([compare](../../internal/serverplan/compare_invites.go)). The plan offers
`create-invite` only for an unrecorded entry; unread invites produce a refused
candidate. A missing or changed recorded invite produces no step
([plan](../../internal/serverplan/plan_invites.go)). Apply sends the limits and
prints the new code ([apply](../../internal/discord/server_apply_invites.go),
[output](../../internal/discord/server_apply.go)).

After the rename, the operator's cluster session runs the live apply
([product plan](../product.md)). After recording the code, compare must show
no invite difference. Check `GET /invites/{code}` separately for `expires_at`
being null: that route returns `expires_at`, not `max_age` ([invite
API](https://github.com/discord/discord-api-docs/blob/main/developers/resources/invite.mdx)).

## Supersedes

This record replaces single statements. The records stand otherwise.

- [ADR-0010](0010-bot-rights-and-apply-path.md), "Channel entries of the bot's
  role": add a row for `rules`, allowing `CREATE_INSTANT_INVITE` for the hub
  invite.
- ADR-0010, the paragraph bounding entries by channel kind: a channel named by
  an invites entry adds `CREATE_INSTANT_INVITE`
  ([validator](../../internal/serverdef/validate_bot.go)).

## Consequences

The bot can create the published invite on `rules` alone; the role does not
keep the right server-wide ([definition](../../server/definition.json)). A
recorded invite that disappears or changes needs a person to decide what to do:
compare reports it and the plan does not replace it
([compare](../../internal/serverplan/compare_invites.go),
[plan](../../internal/serverplan/plan_invites.go)).

## Alternatives considered

- **A person makes the invite in the app.** Compare matches only bot invites,
  so it cannot track a hand-made invite's changes
  ([compare](../../internal/serverplan/compare_invites.go)).
- **Give `@everyone` the right.** Every member could make links, and the
  published link would be one of many ([channel permissions](https://support.discord.com/hc/en-us/articles/10543994968087)).

## Open points

The README and profile links wait for the name in step 4, and are made in one
batch per organisation ([product plan](../product.md)).

## Decided in this record

The bot creates the permanent hub invite, with the right limited to `rules`
after setup. Its code is recorded after the first live apply
([definition](../../server/definition.json), [product plan](../product.md)).

## References

- [Discord channel API](https://github.com/discord/discord-api-docs/blob/main/developers/resources/channel.mdx),
  [guild API](https://github.com/discord/discord-api-docs/blob/main/developers/resources/guild.mdx),
  and [invite API](https://github.com/discord/discord-api-docs/blob/main/developers/resources/invite.mdx),
  read 2026-11-20.
- Discord channel settings: [English](https://support.discord.com/hc/en-us/articles/10543994968087),
  [German](https://support.discord.com/hc/de/articles/10543994968087).
- [ADR-0010](0010-bot-rights-and-apply-path.md),
  [ADR-0015](0015-moderator-without-rights-and-server-settings-as-data.md),
  [ADR-0028](0028-version-1-answers-without-a-model.md),
  and [#254](https://github.com/tekmora/diallak/issues/254).
