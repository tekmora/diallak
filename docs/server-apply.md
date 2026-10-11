# Applying the server definition

The [server definition](server-definition.md) describes the community server
as data. One path brings a server to that description: plan, dry run, apply,
read-only compare
([ADR-0010](adr/0010-bot-rights-and-apply-path.md), "One apply path").

**Three of the four steps are built, and they change nothing.** The bot reads
its server, compares it with the definition and lists the changes that would
close the gap. Every request it makes for that is a read. Making the changes is
a later unit; until then nothing applies the definition.

## In short

- **Snapshot:** what a server looks like, as data: roles from the highest to
  the lowest, categories, channels with their permission entries, the server's
  features, and which role and rights the caller holds. It holds names only,
  never an id of the platform.
- **Compare** reports every difference between the definition and a snapshot
  and changes neither.
- **Plan** turns the differences into an ordered list of changes. Each change
  names the rights it needs. A change the caller may not make is refused, with
  a reason, before anything is tried.
- **Dry run** is the plan as text, followed by a digest of that text.
- **Reading** the server takes four read requests under the bot's identity and
  gives a snapshot.
- **The path deletes nothing.** What the server has and the definition does
  not is reported and left alone.

## Try it

```bash
go test ./internal/serverplan/ ./internal/discord/
```

The tests build a snapshot of the server exactly as the definition describes
it, expect no difference and no change, and then plant one difference per case
(`TestCompare_Negative_EachKind`, `TestPlan_Negative_Reasons` in
[`internal/serverplan`](../internal/serverplan)). The reader is tested against
a local server that answers as Discord does
(`TestReadServer_Positive_FixtureMatchesDefinition` in
[`internal/discord`](../internal/discord/server_read_test.go)).

## The two commands

```bash
diallak -server=plan
diallak -server=compare
```

Both need the same environment as a normal start
([README, "Run"](../README.md#run)), read the server once and exit.
Neither opens the gateway, registers a command or changes anything; the test
`TestReadServer_Positive_FourRequestsInOrderAndNothingElse` holds the reader to
four `GET` requests. The definition they use is the one built into the binary
([`server/definition.go`](../server/definition.go)).

- **`-server=plan`** prints the dry run and a last line that starts with
  `plan digest: sha256:`. The digest covers exactly the text above it
  (`serverplan.Digest`). The apply step will take it, so that it runs only the
  plan a person has read.
- **`-server=compare`** prints `mode: own` or `mode: raised`, the number of
  differences, and one line per difference. `raised` means the bot's role
  holds a permission the definition does not give it (`serverplan.Raised`).
  After a first set-up, `mode: own` is the proof that the extra rights are
  gone.

The exit code is 0 when the read worked, also with refused changes or with
differences: the output is the result. It is 1 with one line on standard error
when the configuration is incomplete, the read failed, the mode is not `plan`
or `compare`, or `-server` is combined with `-register-commands`. The whole
run ends after 60 seconds and each request after 10 (`serverTimeout` in
[`cmd/diallak/main.go`](../cmd/diallak/main.go), `serverCallTimeout` in
[`internal/discord/disgo.go`](../internal/discord/disgo.go)).

## How the server is read

`discord.ReadServer`
([`server_read.go`](../internal/discord/server_read.go)) asks for the
application, the server with its roles, the bot's own membership and the list
of channels, in that order, and stops at the first failure.

- **The caller** is the role Discord manages for this bot, and its rights are
  the union of the rights of every role the bot holds.
- **Roles** are sorted from the highest to the lowest; two roles on the same
  position are sorted as Discord ranks them, the lower id first.
- **Channels** of a type the definition does not know are kept with the type
  `other`.
- **Entries for single members** are counted per channel and not read further;
  the snapshot holds no member.
- **A permission bit the table does not hold** stops the read with the name of
  the role and the number of the bit. The table
  ([`serverdef.Permissions`](../internal/serverdef/serverdef.go)) holds the 52
  permissions Discord documents; the test
  `TestPermissions_Positive_NamesAndBitsPinned` pins every name and bit.
- **Errors name no id.** A failed request is reported with its step and its
  cause; the address, which holds the server's id and the bot user's id, is
  dropped, and the token is replaced
  (`TestReadServer_Negative_TransportErrorNamesNoID`,
  `TestReadServer_Negative_AuthorizationHeaderSanitized`).

## What a snapshot holds

`LoadSnapshot` reads a snapshot with the same strict loader as the definition
(`serverdef.DecodeStrict`) and then checks it (`Check`): exactly one role that
every member holds, listed last; a caller role that exists; known permission
names and channel types, no negative count of member entries. An example is
[`testdata/fresh-server.json`](../internal/serverplan/testdata/fresh-server.json).

Objects are matched by name. The bot's role in the definition is matched to
the caller's role, whatever it is called on the server. Two roles, categories
or channels with the same name are reported as ambiguous and never changed,
and neither is anything that rests on them: an entry for such a role, or a
channel in such a category.

`channels_complete` says whether the list of channels is the whole list. From
2026-11-16 Discord leaves out every channel the reading identity cannot see
(Discord developer documentation, "Get Guild Channels"), so a reader without
administrator rights cannot tell a missing channel from a hidden one.

## What compare reports

<!-- markdownlint-disable MD013 -->
| Kind | Reported when |
| :--- | :--- |
| `role-missing`, `role-ambiguous` | A role of the definition has no match on the server, or more than one |
| `role-name` | The caller's role is called something else than the definition's bot role |
| `role-permissions` | A role lacks a permission or holds one beyond the definition |
| `role-order` | The roles stand in another order than `role_order` |
| `category-missing`, `category-ambiguous` | The same for a category |
| `channel-missing`, `channel-ambiguous` | The same for a channel, when the channel list is complete |
| `channel-not-visible` | A channel has no match and the list is not complete: either hidden from the bot by the definition, as the ticket channel is, or missing or hidden |
| `channel-type`, `channel-category` | A channel has another type or sits in another category |
| `access-missing`, `access-differs`, `access-extra` | A permission entry of a channel is absent, differs, or exists on the server only |
| `access-member` | A channel holds entries for single members; the definition gives access to roles only. Reported with the count, never planned as a change |
| `role-extra`, `category-extra`, `channel-extra` | The server has something the definition does not |
| `feature-missing` | The definition has an announcement channel and the server lacks the feature `NEWS` |
<!-- markdownlint-enable MD013 -->

A raised bot role shows up here: while the operator has given it more rights or
a higher place for a first set-up, compare reports both as differences.

## What a plan holds

The list of actions is closed (`serverplan.Actions`), and none of them deletes.

<!-- markdownlint-disable MD013 -->
| Action | For | Rights it needs |
| :--- | :--- | :--- |
| `create-role` | A missing role | `MANAGE_ROLES` and every permission the role gets |
| `set-role-permissions` | A role with other permissions | `MANAGE_ROLES` and every permission added |
| `set-role-order` | Roles in another order, or a role that is created | `MANAGE_ROLES` |
| `create-category` | A missing category | `MANAGE_CHANNELS` |
| `create-channel` | A missing channel, created together with its entries | `MANAGE_CHANNELS` and every permission its entries name |
| `move-channel` | A channel in another category | `MANAGE_CHANNELS` |
| `set-access` | A permission entry that is absent or differs | `MANAGE_ROLES` and every permission the entry names |
| `convert-channel`, `resolve-name` | A channel of another type; an ambiguous name | Always refused: a person has to settle these |
<!-- markdownlint-enable MD013 -->

The plan never holds a change to the caller's own role. Its rights, its name
and its place are the operator's to change by hand.

`Result.Rights` is the list of every right the plan needs, refused changes
included. It is the list the operator sees before raising the bot's role for a
first set-up: for that run the role holds the rights the plan names
([ADR-0010](adr/0010-bot-rights-and-apply-path.md), "One apply path"), and
compare shows afterwards whether they are gone again. The list can name more
than a run uses, because a channel that is refused as not visible may already
exist.

## Why a change is refused

A refused change carries the first reason that holds, in this order.

<!-- markdownlint-disable MD013 -->
| Reason | Meaning |
| :--- | :--- |
| `ambiguous-name` | A name the change rests on exists more than once on the server |
| `type-differs` | The channel has another type; the path does not convert |
| `not-visible` | The channel may exist hidden from the reader |
| `outside-own-changes` | The bot's role is not raised, and the change is not the entry of an area's role on that area's channel |
| `missing-feature` | The server lacks the feature the channel type needs |
| `managed-role` | An integration manages the role |
| `above-caller` | The role stands above the caller's role, or the new order would move or pass a role that does; Discord lets a bot edit and sort only roles below its own |
| `lacks-right` | The caller does not hold a right the change needs |
<!-- markdownlint-enable MD013 -->

With its three rights the bot may plan one kind of change only: the entry of
an area's role on that area's channel, which is how an area opens and closes.
Everything else waits for a run in which the operator has raised the bot's
role.

## Not built, not tried

- The apply step. No code changes a server.
- Nothing was run against Discord. The reader is tested against a local server
  only; the first read of the real server is part of the first set-up.
- Not tested: how Discord answers when the list of channels leaves out hidden
  channels after 2026-11-16. The reader marks the list as incomplete from that
  day on unless the bot holds `ADMINISTRATOR`
  (`TestReadServer_Positive_ChannelsComplete_CutoffAndAdmin`).
- A change is judged on its own. An entry for a role whose creation was
  refused is not refused with it; what the apply step does with a plan that
  holds a refusal is settled when that step is built.
- Compared today: roles, their order, categories, channels and permission
  entries. Not compared yet: forum tags, onboarding questions, filter rules and
  channel topics.
