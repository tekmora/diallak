# Applying the server definition

The [server definition](server-definition.md) describes the community server
as data. One path brings a server to that description: plan, dry run, apply,
read-only compare
([ADR-0010](adr/0010-bot-rights-and-apply-path.md), "One apply path").

**Half of the path is built, and it changes nothing.** The package
[`internal/serverplan`](../internal/serverplan) compares the definition with a
description of a server and lists the changes that would close the gap. It
reads two files and talks to nobody. Reading a real server and making the
changes are a later unit; until then nothing applies the definition.

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
- **Dry run** is the plan as text.
- **The path deletes nothing.** What the server has and the definition does
  not is reported and left alone.

## Try it

```bash
go test ./internal/serverplan/
```

The tests build a snapshot of the server exactly as the definition describes
it, expect no difference and no change, and then plant one difference per case
(`TestCompare_Negative_EachKind`, `TestPlan_Negative_Reasons` in
[`internal/serverplan`](../internal/serverplan)). No command runs the path yet.

## What a snapshot holds

`LoadSnapshot` reads a snapshot with the same strict loader as the definition
(`serverdef.DecodeStrict`) and then checks it (`Check`): exactly one role that
every member holds, listed last; a caller role that exists; known permission
names and channel types. An example is
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

## Not built

- Reading a server into a snapshot, and the apply step with a deadline on each
  call.
- A command that runs the path.
- A change is judged on its own. An entry for a role whose creation was
  refused is not refused with it; what the apply step does with a plan that
  holds a refusal is settled when that step is built.
- Compared today: roles, their order, categories, channels and permission
  entries. Not compared yet: forum tags, onboarding questions, filter rules and
  channel topics.
