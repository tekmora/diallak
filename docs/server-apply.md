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
  features, and which role, rights and position the caller holds. It holds
  names only, never an id of the platform.
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

A role also carries how it is shown: `shown_apart` when its members form a
group of their own in the member list, `mentionable` when members can mention
it. A snapshot leaves out a key that is false.

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
| `role-missing`, `role-ambiguous` | A role of the definition has no match on the server, or more than one. A missing role that another party manages carries the note `managed; the plan does not create it` |
| `role-name` | The caller's role is called something else than the definition's bot role |
| `role-permissions` | A role lacks a permission or holds one beyond the definition |
| `role-order` | The roles stand in another order than `role_order` |
| `role-not-managed` | The definition marks the role as managed by another party, and the server role of that name is not: a person made it by hand |
| `role-place` | A role an integration manages stands above the Moderator role. The plan holds no step for it; a person drags the role down |
| `role-display` | `shown_apart` or `mentionable` of a role differs from what the definition states. A field the definition does not state is not compared |
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
| `set-role-display` | A role whose stated `shown_apart` or `mentionable` differs. The change sends the stated fields only | `MANAGE_ROLES` |
| `set-role-order` | Roles in another order, counted after the roles the plan creates | `MANAGE_ROLES` |
| `create-category` | A missing category | `MANAGE_CHANNELS` |
| `create-channel` | A missing channel, created together with its entries | `MANAGE_CHANNELS` and every permission its entries name |
| `move-channel` | A channel in another category | `MANAGE_CHANNELS` |
| `set-access` | A permission entry that is absent or differs | `MANAGE_ROLES` and every permission the entry names |
| `convert-channel`, `resolve-name` | A channel of another type; an ambiguous name | Always refused: a person has to settle these |
<!-- markdownlint-enable MD013 -->

The plan never holds a change to the caller's own role. Its rights, its name
and its place are the operator's to change by hand.

It never holds a change to a role another party manages either: the booster
role and the roles of the two funding integrations
([ADR-0014](adr/0014-roles-managed-notification-recognition.md)). Compare
reports one that is missing, made by hand, shown otherwise than the definition
states or, for a funding role, placed above the Moderator role; each of these
is a person's to settle. The apply step returns an error before any request
if a plan names such a role
(`TestApply_Defense_ManagedRoleRefusedWithoutRestCall`).

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
| `managed-role` | The platform or an integration manages the server role |
| `unknown-permission-bits` | The permission entry the change would replace has a bit set that the table does not name. The change sends the whole entry and would clear it |
| `above-caller` | The role stands above the caller's role, or the new order would move or pass a role that does; Discord lets a bot edit and sort only roles below its own |
| `too-few-positions` | The order step would need more positions below the caller's role than the server has; the detail names the roles to place by hand |
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

With its four rights the bot may plan one kind of change only: the entry of
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
(`not enough positions below the bot's role`). The plan knows that rule and
the position of the bot's role, so it refuses such a step first, with
`too-few-positions`, and an apply never meets it with a plan it accepted
(`TestPlan_Boundary_ReorderWithoutCreation`,
`TestPlan_Boundary_CreatedRoleWithTheCallerOnTop`).

Discord moves no role when it creates one (measured on a live server on
2026-10-12: after 24 creations the bot's role still stood on position 4,
with 27 roles below it). The roles of one run share the places below the
roles already there and are read in the order of their creation, which is
the definition's. The plan counts on that and holds an order step only when
the order after its own creations differs from the definition:

- **A first set-up** holds none, and runs through
  (`TestApplyServer_Negative_FirstSetUpOnAPlatformThatMovesNoRoleUp`).
- **A role added at the end of `role_order`** later holds none either
  (`TestApplyServer_Negative_RoleAddedLaterOnAPlatformThatMovesNoRoleUp`).
- **A role added between two roles** that stand on the server needs a place
  that a server whose roles stand without gaps does not have. The plan
  creates the role and refuses the order step with `too-few-positions`,
  naming the role; a person drags it to its place, and the next plan holds no
  order step
  (`TestApplyServer_Negative_RoleInsertedMidOrderIsRefusedInThePlan`).

## A first set-up

On a new server the bot's four rights are not enough for anything but area
entries. For the first run the bot's role holds more and stands above the
roles it sets up, and afterwards a person lowers it again
([ADR-0010](adr/0010-bot-rights-and-apply-path.md), "One apply path").

### A new server

A person acts four times: adds the bot, switches the server to a community
server, makes the Operator role, and tidies up after the run. The rights of
the run come with the address that adds the bot, so nobody switches them on
by hand.

1. **Add the bot with the rights of the set-up.** Open this address with your
   application's id in it, choose the server and accept:

   ```text
   https://discord.com/oauth2/authorize?client_id=<application id>&scope=bot&permissions=1168500596784
   ```

   The number stands for seven rights. It holds no `ADMINISTRATOR`.

   - Four are for the run: `MANAGE_ROLES`, `MANAGE_CHANNELS`, `CONNECT` and
     `CREATE_PRIVATE_THREADS`. The run needs six more, and the bot has those
     as a member through `@everyone`.
   - Three more are rights the definition gives the bot's role for good:
     `MANAGE_MESSAGES`, `MODERATE_MEMBERS` and `MANAGE_GUILD`. No change of
     the run needs them. The address carries them because the plan never
     holds a change to the bot's own role
     ([What a plan holds](#what-a-plan-holds)).

   `MANAGE_GUILD` is held for a function that is decided and not built:
   server settings as data
   ([ADR-0015](adr/0015-moderator-without-rights-and-server-settings-as-data.md)).
   Until it is built the tool makes no call that needs the right
   (`TestServerApplyCalls_Positive_FullRunDisplayAndEntryChange`). Discord
   describes the address in its
   [page on authorisation](https://discord.com/developers/docs/topics/oauth2),
   "Bot Authorization Flow"; adding `&guild_id=<server id>&disable_guild_select=true`
   fixes the server in the dialog.
2. **Switch the server to a community server**: Server Settings, Enable
   Community
   ([Enabling Your Community Server](https://support.discord.com/hc/en-us/articles/360047132851)).
   The definition has an announcement channel, and without the server feature
   `NEWS` the plan refuses it as `missing-feature`
   ([Why a change is refused](#why-a-change-is-refused)). The bot cannot
   switch this on: Discord asks for Administrator to switch the server
   feature `COMMUNITY` on
   ([page on servers](https://discord.com/developers/docs/resources/guild),
   "Mutable Guild Features"), and the bot's role holds that right at no step.
3. **Make the Operator role** by hand: the name the definition gives it, with
   **Administrator**, above the bot's role, given to yourself
   ([The Operator role](#the-operator-role)).
4. `-server=plan`. The plan holds changes and, when all is well, no refusal.
   A person reads it and keeps the digest. A right the bot lacks shows by its
   programming name on a change refused as `lacks-right`.
5. `-server=apply=<digest>`.
6. **Tidy up.** On the bot's role switch off **Manage Channels**, **Connect**
   and **Create Private Threads**, so that Manage Roles, Manage Messages,
   Time Out Members and Manage Server stay, and drag the role to the place the
   definition gives it. The bot can do neither: a bot edits and sorts only roles below its own
   highest role, and its own role is that role (Discord's
   [permissions page](https://discord.com/developers/docs/topics/permissions),
   "Permission Hierarchy").
7. `-server=compare` prints `mode: own`. Three kinds of difference can
   remain, and no plan holds a change for any of them:

   - things the server has and the definition does not;
   - `role-missing` with the note `managed; the plan does not create it`, for
     the booster role until a member boosts the server and for a funding role
     until its integration is connected;
   - `role-place` for a funding role that was on the server before the run
     and now stands above the Moderator role. Drag it below that role.

Between step 5 and step 6 compare still reports two differences, and the plan
holds no change for either: the rights of the bot's role and its place in the
order (`TestApplyServer_Positive_FullRun`).

The number is computed, not written by hand: the test
`TestServerApplyDoc_Negative_InvitationNumberIsThePlansRights` plans a new
server, takes the rights the plan needs together with the rights the
definition gives the bot's role, leaves out those `@everyone` keeps, and
fails when the address on this page carries another number or when those
rights leave a refusal in the plan. A definition with other entries gives
another number; the test prints it.
`TestServerApplyDoc_Negative_TidyUpRightsMatchExtraSetUpRights` fails when the
rights step 6 switches off are not exactly those of the address that the
definition does not give the bot's role.

### A server the bot is already on

Do not open the address a second time to change the rights. Discord's pages
do not say what a second authorisation does to the role of a bot that is
already a member, and they do not say what becomes of the role and of its
entries on channels when the bot is removed and added again (both read
2026-10-12; [Not built, not tried](#not-built-not-tried)). Switch the missing
rights on by hand:

1. `-server=plan` with the rights the bot's role has. A change the role's
   rights do not cover is refused as `outside-own-changes` or `lacks-right`;
   the line `rights needed:` lists every right the run needs.
2. Give the bot's role the rights of that list which neither it nor
   `@everyone` holds, and every right the definition gives it for good: the
   plan never holds a change to the bot's own role, so `-server=compare`
   reports such a right as `role-permissions roles[bot]` after the word
   `lacks` until a person switches it on. The table below has the label each
   one carries in the role's settings. The role has to stand above every role the plan creates,
   changes or sorts; a change that reaches a role above it is refused as
   `above-caller` (`TestPlan_Boundary_NeighboursOfTheCaller`).
3. Go on with step 2 of [A new server](#a-new-server).

### The Operator role

The Operator role is the one role a person makes: under the name the
definition gives it, with `ADMINISTRATOR`, above the bot's role. The role's
settings call that right **Administrator**; the tool prints the names of
Discord's programming interface, and the settings show other words for some
of them
([The rights by their name in the settings](#the-rights-by-their-name-in-the-settings)).
The plan cannot make this role. Creating a role needs every right the
role gets ([What a plan holds](#what-a-plan-holds)), a bot gives a role only
rights it holds itself, and the bot's role holds no `ADMINISTRATOR` at any
step. ADR-0010 left open who makes the role ("Open points"). The tests of a
first set-up start from a server that has it (`setupFreshServerFake` in
[`server_apply_test.go`](../internal/discord/server_apply_test.go)).

Two lines of `-server=compare` show that this is not done yet:

- `role-missing roles[operator]`: the role is not there, or has another name.
- `role-permissions roles[bot]` with `ADMINISTRATOR` after the word `beyond`:
  the bot's role holds the right, for example from an invitation with another
  number. The operator takes it away first.

Without the Operator role the tool does not stop by itself yet
([#83](https://github.com/tekmora/diallak/issues/83)). `rights needed:` then
names `ADMINISTRATOR`, because the plan holds `create-role roles[operator]`
(`addRoleMissingCandidates` in [`plan.go`](../internal/serverplan/plan.go)).
That list is not one to give the bot's role: make the role, then plan again.
No test covers a server without the role.

### What the tests of a first set-up cover

Five tests walk a first set-up against the local server
([`server_apply_setup_test.go`](../internal/discord/server_apply_setup_test.go),
[`server_apply_managed_test.go`](../internal/discord/server_apply_managed_test.go)).
They give the bot's role every right the first plan names: the four of the
address that the run needs and those a member has.

- `TestApplyServer_Positive_FirstSetUpWithTheRightsThePlanNames`: with the
  rights the first plan names, the second plan holds no refusal, the apply run
  makes every change, and after the tidy-up compare holds nothing but managed
  roles that are missing. The list holds no `ADMINISTRATOR`.
- `TestApplyServer_Boundary_FirstSetUpWithoutTheManagedRoles`: on a server
  without the three managed roles, compare names exactly those three after
  the tidy-up, each with its note, and the next plan holds neither a change
  nor a refusal.
- `TestApplyServer_Boundary_FirstSetUpLeavesFundingRolesWhereTheyStand`: on a
  server that held the three managed roles before, the two funding roles
  stand above the Moderator role after the run and compare reports
  `role-place` for both; once they are dragged down, compare holds no
  difference. The local server puts a new role at the bottom, which Discord's
  page does not state ([Not built, not tried](#not-built-not-tried)).
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

`TestPlan_Positive_RaisedFirstSetUp`
([`plan_test.go`](../internal/serverplan/plan_test.go)) plans the set-up with
the seven rights of the address on the bot's role and the member rights on
`@everyone` alone: no refusal.

### The rights by their name in the settings

The tool prints a right under the name of Discord's programming interface
(`permissionsTable` in [`serverdef.go`](../internal/serverdef/serverdef.go)). The
operator switches it on under another name: in the server's settings, under
Roles, on the role's permissions. For most rights the label is the same words
in another spelling. One is not: `MODERATE_MEMBERS` is the switch
**Time Out Members**, in the German app **Mitglieder in Timeout versetzen**,
and no switch is called "Moderate Members".

The table holds every right the definition of this repository names, on a
role or on a channel's entry, and `MANAGE_CHANNELS`, which every plan that
makes a channel needs ([What a plan holds](#what-a-plan-holds)). The line
`rights needed:` of a plan names a part of them; which part depends on what
the server already has. The test `TestServerApplyDoc_Negative_LabelsEveryRight`
fails when the definition names a right the table lacks, or when a row lacks
one of the two labels.

<!-- markdownlint-disable MD013 -->
| Name the tool prints | Label, English app | Label, German app | Read from |
| :--- | :--- | :--- | :--- |
| `ADMINISTRATOR` | Administrator, under "Advanced Permissions" | Administrator, under "Erweiterte Berechtigungen" | [Discord Roles and Permissions](https://support.discord.com/hc/en-us/articles/214836687) |
| `MANAGE_ROLES` | Manage Roles, under "General Server Permissions" | Rollen verwalten, under "Allgemeine Serverberechtigungen" | the same page |
| `MANAGE_CHANNELS` | Manage Channels | Kanäle verwalten | [Permissions on Discord](https://discord.com/community/permissions-on-discord-discord); German: [Sprachkanäle FAQ](https://support.discord.com/hc/de/articles/19583625604887) |
| `MANAGE_GUILD` | Manage Server | Server verwalten | [Pause Invites FAQ](https://support.discord.com/hc/en-us/articles/8458903738647) |
| `MANAGE_MESSAGES` | Manage Messages | Nachrichten verwalten | [Channel Permissions Settings 101](https://support.discord.com/hc/en-us/articles/10543994968087) |
| `MODERATE_MEMBERS` | **Time Out Members** | **Mitglieder in Timeout versetzen** | [Time Out FAQ](https://support.discord.com/hc/en-us/articles/4413305239191) |
| `VIEW_CHANNEL` | View Channel | Kanal anzeigen | Channel Permissions Settings 101 |
| `SEND_MESSAGES` | Send Messages | Nachrichten senden | Channel Permissions Settings 101 |
| `SEND_MESSAGES_IN_THREADS` | Send Messages in Threads | Nachrichten in Threads senden | Channel Permissions Settings 101; [Threads FAQ](https://support.discord.com/hc/en-us/articles/4403205878423) |
| `CREATE_PUBLIC_THREADS` | Create Public Threads | Öffentliche Threads erstellen | the same two pages |
| `CREATE_PRIVATE_THREADS` | Create Private Threads | Private Threads erstellen | the same two pages |
| `READ_MESSAGE_HISTORY` | Read Message History | Nachrichtenverlauf lesen | Channel Permissions Settings 101 |
| `ADD_REACTIONS` | Add Reactions | Reaktionen hinzufügen | Channel Permissions Settings 101 |
| `EMBED_LINKS` | Embed Links | Links einbetten | Channel Permissions Settings 101 |
| `ATTACH_FILES` | Attach Files | Dateien anhängen | Channel Permissions Settings 101 |
| `USE_APPLICATION_COMMANDS` | Use Application Commands | Anwendungsbefehle verwenden | Channel Permissions Settings 101 |
| `CONNECT` | Connect | Verbinden | Permissions on Discord; German: [Geplante Events](https://support.discord.com/hc/de/articles/4409494125719) |
| `SPEAK` | Speak | Sprechen | the same two pages |
<!-- markdownlint-enable MD013 -->

What the table does not settle:

- The labels were read from Discord's own pages on 2026-10-12, not in the app.
  Which name belongs to which label follows from the description of each
  right in Discord's
  [permissions page](https://discord.com/developers/docs/topics/permissions).
- A German label was read from the German version of the page its row names
  (the same address with `de` in place of `en-us`), unless the row names a
  German page of its own. The app's other languages were not read.
- The timeout right has two English spellings on Discord's own pages: the
  help page writes "Time Out Members", the permissions page for developers
  "Timeout Members". Both mean the same switch.
- The German labels of `CONNECT` and `SPEAK` were read in a sentence of the
  page about events, not in a list of switches.
- Channel Permissions Settings 101 shows a channel's settings. "Permissions on
  Discord" names `VIEW_CHANNEL` on a role "Read Text Channels & See Voice
  Channels"; the page about private servers calls it "View Channels", in
  German "Kanäle anzeigen". The wording on a role's own screen was not read
  in the app.
- On a channel's entries the right `MANAGE_ROLES` appears as "Manage
  Permissions", in German "Berechtigungen verwalten".
- A definition with other entries names other rights. For a name that is not
  in the table, the description in Discord's permissions page says what the
  switch does.

## Not built, not tried

- Nothing was run against Discord. The reader and the apply step are tested
  against a local server only; the first read and the first change of the real
  server are part of the first set-up.
- The address that adds the bot was not opened. That it carries the rights as
  a number and that a person accepts them is read from Discord's page on
  authorisation (2026-10-12). Not stated on a page of Discord's that was read:
  whether the person can switch single rights off in the dialog, what a second
  authorisation does to the role of a bot that is already a member, and
  whether removing the bot deletes its role and the role's entries on
  channels. A right that did not arrive shows in the plan as `lacks-right`.
- Not stated on a page of Discord's that was read: in which direction roles
  on one position are sorted by id. The reader takes the lower id as the
  higher role, as discord.py does, and the read-back of the live server of
  2026-10-12 matched the definition with it. Not tried: what Discord does
  with a request for positions in which two roles share one.
- Not known: where a new role stands among the roles the server already has.
  Discord's page for creating a role names no place. The local server puts it
  at the bottom, so a managed role that was there before ends above the roles
  of the run, and `set-role-order` leaves it there
  (`TestApplyServer_Boundary_FirstSetUpLeavesFundingRolesWhereTheyStand`).
- Not tried: whether Discord lets a bot change how the platform's own booster
  role is shown. The plan holds no step for it.
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
- Compared today: roles, their order, how a role is shown where the
  definition states it, categories, channels and permission entries. Not compared and not applied yet: forum tags, filter rules and
  channel topics.
