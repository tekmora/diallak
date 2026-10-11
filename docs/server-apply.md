# Applying the server definition

The [server definition](server-definition.md) describes the community server
as data. One path brings a server to that description: plan, dry run, apply,
read-only compare
([ADR-0010](adr/0010-bot-rights-and-apply-path.md), "One apply path").

**All four steps are built. None of them was run against Discord.** Two
commands only read. The third, `-server=apply=<digest>`, changes the server:
it makes the changes of one plan, and only of the plan whose digest a person
read before. Every test talks to a local server that answers as Discord does.

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
- **Dry run** is the plan as text, followed by a digest of that text and of
  the definition it was made from.
- **Apply** reads the server again, plans again, and makes the changes only
  when the digest is the one it was given and the plan holds no refusal. It
  stops at the first failure.
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
[`internal/discord`](../internal/discord/server_read_test.go)). The apply step
runs against a local server that keeps roles and channels in memory, and is
read back afterwards with the reader and compare
(`TestApplyServer_Positive_FullRun` in
[`server_apply_test.go`](../internal/discord/server_apply_test.go)).

## The three commands

```bash
diallak -server=plan
diallak -server=compare
diallak -server=apply=sha256:<64 hex digits>
```

All three need the same environment as a normal start
([README, "Run"](../README.md#run)) and exit when they are done. None opens the
gateway or registers a command. The definition they use is the one built into
the binary ([`server/definition.go`](../server/definition.go)).

- **`-server=plan`** reads the server and prints the dry run, then a line
  `definition: sha256:…` and a last line `plan digest: sha256:…`. The digest
  covers exactly the text above its own line (`serverplan.Digest`), so it
  changes when the plan changes and when the definition changes
  (`server.DefinitionDigest`). It changes nothing; the test
  `TestReadServer_Positive_FourRequestsInOrderAndNothingElse` holds the reader
  to four `GET` requests.
- **`-server=compare`** reads the server and prints `mode: own` or
  `mode: raised`, the number of differences, and one line per difference.
  `raised` means the bot's role holds a permission the definition does not
  give it (`serverplan.Raised`). After a first set-up, `mode: own` is the
  proof that the extra rights are gone.
- **`-server=apply=<digest>`** takes the digest that `-server=plan` printed
  and makes that plan's changes. [How a plan is applied](#how-a-plan-is-applied)
  says when it refuses.

The exit code of `plan` and `compare` is 0 when the read worked, also with
refused changes or with differences: the output is the result. The exit code
of `apply` is 0 only when every change was made. All three exit with 1 and one
line on standard error when the configuration is incomplete, a request failed,
the value of `-server` is not one of the three forms, or `-server` is combined
with `-register-commands`.

<!-- markdownlint-disable MD013 -->
| Limit | Value | Where |
| :--- | :--- | :--- |
| A whole `plan` or `compare` run | 60 seconds | `serverTimeout` in [`cmd/diallak/main.go`](../cmd/diallak/main.go) |
| A whole `apply` run | 15 minutes | `applyTimeout`, same file |
| One read request | 10 seconds | `serverCallTimeout` in [`internal/discord/disgo.go`](../internal/discord/disgo.go) |
| One change request | 60 seconds | `applyCallTimeout`, same file |
<!-- markdownlint-enable MD013 -->

The two apply limits are chosen, not measured. The limit of one change has to
cover the wait the client library takes by itself when Discord answers with a
rate limit.

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
- **A permission bit the table does not name** is kept as its number, per
  role and per side of an entry, and the read goes on
  (`TestReadServer_Boundary_UnknownBitsKeptByNumber`). The table
  ([`serverdef.Permissions`](../internal/serverdef/serverdef.go)) holds the 52
  permissions Discord documents; the test
  `TestPermissions_Positive_NamesAndBitsPinned` pins every name and bit.
  Discord's reference describes a permission value as an integer of open
  length that new permissions are added to (Discord developer documentation,
  "Permissions"), and a server can hold a bit that page does not list: the
  read of release 0.2.0 stopped on bit 47 of a role. Such a bit gives the
  caller no right the plan counts
  (`TestReadServer_Boundary_UnknownBitGivesTheCallerNoRight`).
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

A role may carry `unknown_bits`, and an entry `unknown_allow` and
`unknown_deny`: the numbers of the bits it has set that the table does not
name. `Check` accepts only numbers from 0 to 63 without a name in the table,
each once and in rising order
(`TestCheck_Boundary_UnknownBitsAreTheBitsWithoutAName`). A snapshot without
such a bit holds none of the three keys.

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
| `unknown-permission-bits` | A role or a permission entry of the server has a bit set that the table does not name. Reported with the server's own names and the numbers of the bits, for objects the definition does not hold too |
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
included; the dry run prints it as `rights needed:`. It is the list the
operator sees before raising the bot's role for a first set-up: for that run
the role holds the rights the plan names
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
| `unknown-permission-bits` | The permission entry the change would replace has a bit set that the table does not name. The change sends the whole entry and would clear it |
| `above-caller` | The role stands above the caller's role, or the new order would move or pass a role that does; Discord lets a bot edit and sort only roles below its own |
| `lacks-right` | The caller does not hold a right the change needs |
<!-- markdownlint-enable MD013 -->

A bit without a name is handled in two ways, by where it sits.

- **On a role** it is cleared, and the plan says so. A change of a role's
  permissions sends the definition's whole value, so the bit goes with the
  change. The plan line ends with `clears unlisted bits` and the numbers, and
  the plan digest covers that line: the person who confirms the digest has
  seen which bits go
  (`TestPlan_Positive_UnlistedBitsOnAChangedRoleAreClearedAndNamed`,
  `TestApplyServer_Positive_UnlistedBitOnAChangedRoleIsCleared`). No change is
  made only to clear a bit: a role that already has the permissions of the
  definition is left alone, and compare keeps reporting its bit
  (`TestPlan_Boundary_UnknownBitsLeaveTheRestAlone`). A role an integration
  manages is never written
  (`TestPlan_Negative_ManagedRoleWithUnlistedBitsIsNeverWritten`).
- **On a permission entry of a channel** it still stops the change that would
  replace that entry; every other entry of the same channel is left alone.
  Because a plan with a refusal is not applied, the run waits until a person
  has settled the bit on the server
  (`TestApplyServer_Negative_UnknownBitOnAnEntryStopsTheRun`).

Discord's permissions reference does not say whether a write that clears such
a bit takes effect. The compare after the apply shows it: a bit that is still
set is reported again as `unknown-permission-bits`.

With its three rights the bot may plan one kind of change only: the entry of
an area's role on that area's channel, which is how an area opens and closes.
Everything else waits for a run in which the operator has raised the bot's
role.

## How a plan is applied

`discord.ApplyServer`
([`server_apply.go`](../internal/discord/server_apply.go)) reads the server
with the same four requests as the reader, plans, and then checks three things
before it sends a single change. A run without a token, a server id, a
definition or one of the two digests ends before the first request
(`TestApplyServer_Negative_InputRefusedBeforeAnyRequest`).

1. **The digest is the one it was given.** It computes the digest of the plan
   it has just made, with the digest of the built-in definition. When the
   server or the definition changed since a person read the dry run, the two
   differ, and the run ends with
   `the plan is not the one that was shown` and the new digest
   (`TestApplyServer_Negative_Gates`).
2. **The plan holds no refusal.** A plan with one refused change is not
   applied in part (`TestVerifyApplyGates_Boundary_OneRefusalIsEnough`). The
   refusal has to be settled first: by the operator on the server, or by a
   change of the definition.
3. **Every change is one it knows.** An action outside the seven that change
   something, or an object the definition does not hold, ends the run
   (`TestVerifyApplyGates_Negative_ChangeOutsideTheDefinition`).

Then it makes the changes in the order of the plan, one at a time.

- **Names and rights come from the definition, ids from the server.** The run
  keeps the ids of the roles, categories and channels it read, and of those it
  creates, in memory. No id is printed, logged or stored.
- **Every change carries its reason.** Discord's audit log shows
  `diallak apply <plan digest>` beside each change
  (`TestExecuteSingleChange_Boundary_CreatedObjectAndItsID`,
  `TestApplySetAccess_Positive_RequestCarriesAllowDenyAndTheReason`).
- **The first failure ends the run.** The error names the step, the action,
  the object and the cause, and no id; no later change is sent
  (`TestApplyServer_Negative_Failures`). The changes made before it stay. A
  new `-server=plan` then shows what is left.
- **A cancelled run stops between two changes.** It sends no further change
  and still writes its last line
  (`TestApplyServer_Boundary_CancelledBetweenTwoChanges`).
- **No request is repeated by this code.** The client library waits and
  repeats on a rate limit only.
- **Nothing is deleted.** The file holds no delete request, and a test fails
  when one appears (`TestApplyServer_Boundary_Unit`).
- **Not set at all:** channel topics, forum tags and filter rules. The
  definition names them; compare does not read them and apply does not write
  them.

The output is one line per change and a last line that is always written:

```text
changes: 3
  1. create-role roles[guide-coding]: done
  2. set-role-order role_order: done
  3. create-channel channels[coding-help]: failed
applied: 2 of 3
```

### The order of roles

`set-role-order` is the one change that is computed at the moment it runs. It
reads the roles again, takes the roles that stand below the bot's role, puts
those the definition orders into the definition's order and leaves every other
role where it is, and numbers the result from the bottom without gaps. It
sends one request with all of them, or none when they already stand right
(`TestApplySetRoleOrder_Positive_OnePatchWithEveryRoleBelow`,
`TestApplySetRoleOrder_Boundary_OrderHeldSendsNoWrite`). The read and the
write each have the limit of one change
(`TestApplySetRoleOrder_Boundary_EachRequestHasItsOwnDeadline`).

It refuses when the roles below the bot's role do not fit below its position
(`not enough positions below the bot's role`). That is the case when a new
role lands on the same position as the bot's role, which this repository
cannot know: [Not built, not tried](#not-built-not-tried).

## A first set-up

On a new server the bot's three rights are not enough for anything but area
entries, so the first run needs the operator twice: once to raise the bot's
role and once to lower it again
([ADR-0010](adr/0010-bot-rights-and-apply-path.md), "One apply path").

1. `-server=plan` with the bot's own rights. Every change is refused as
   `outside-own-changes`; the line `rights needed:` is the list for the next
   step.
2. The operator gives the bot's role exactly those rights. The role has to
   stand above every role the plan creates, changes or sorts; a change that
   reaches a role above it is refused as `above-caller`
   (`TestPlan_Boundary_NeighboursOfTheCaller`).
3. `-server=plan` again. Now the plan holds changes and, when all is well, no
   refusal. A person reads it and keeps the digest.
4. `-server=apply=<digest>`.
5. The operator takes the rights away again and puts the bot's role where the
   definition has it.
6. `-server=compare` prints `mode: own`. The differences that remain are
   things the server has and the definition does not.

Between step 4 and step 5 compare still reports two differences, and the plan
holds no change for either: the rights of the bot's role and its place in the
order (`TestApplyServer_Positive_FullRun`).

Three tests walk these steps against the local server
([`server_apply_setup_test.go`](../internal/discord/server_apply_setup_test.go)).

- `TestApplyServer_Positive_FirstSetUpWithTheRightsThePlanNames`: with the
  rights the first plan names, the second plan holds no refusal, the apply run
  makes every change, and after step 5 compare holds no difference. The list
  holds no `ADMINISTRATOR`.
- `TestApplyServer_Boundary_EveryNamedRightIsNeeded`: with any one of the
  named rights left out, the plan holds a refusal as `lacks-right` and the
  apply run sends no change.
- `TestApplyServer_Boundary_FirstSetUpAndTheChannelListDate`: on 2026-11-16
  the same rights are no longer enough. The list of channels is then whole
  only for a reader with `ADMINISTRATOR`
  ([What a snapshot holds](#what-a-snapshot-holds)), so every missing channel
  the definition lets the bot see is refused as `not-visible`. A channel the
  definition hides from the bot, as the ticket channel, is then not planned
  at all.

## Not built, not tried

- Nothing was run against Discord. The reader and the apply step are tested
  against a local server only; the first read and the first change of the real
  server are part of the first set-up.
- Not known: whether Discord moves the other roles up when a role is created.
  The local server can do both. When it does not, the roles stay where they
  were created, `set-role-order` refuses, and the order is set by hand before
  a second plan.
- Not tested: how Discord answers a change of positions that leaves a role
  out, a permission entry that removes a right the bot lacks, and a rate limit
  in the middle of a run.
- Not tested: how Discord answers when the list of channels leaves out hidden
  channels after 2026-11-16. The reader marks the list as incomplete from that
  day on unless the bot holds `ADMINISTRATOR`
  (`TestReadServer_Positive_ChannelsComplete_CutoffAndAdmin`).
- A change is judged on its own when it is planned: an entry for a role whose
  creation was refused is not refused with it. The apply step does not need
  that settled, because it refuses a plan with any refusal.
- Compared today: roles, their order, categories, channels and permission
  entries. Not compared and not applied yet: forum tags, filter rules and
  channel topics.
