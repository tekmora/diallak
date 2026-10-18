# ADR-0024: Supporter roles from the funding platforms

- **Status**: Accepted
- **Date**: 2026-10-18
- **Authors**: Maintainers

## Context

The [server definition](../server-definition.md) holds two supporter roles,
"Supporter" (`supporter`) and "Supporter for a year" (`supporter-12-months`).
Both are of kind `recognition`, are given by the bot (`assigned_by: bot`) and
carry no right. The function that gives and takes them, `supporter-roles`, is
`planned` and needs a record (`server/definition.json`). Version 1.0 contains
it ([product](../product.md), issue #196).

What is already decided and is not reopened here:

- **A supporter gets a role and a plain-text credit, and nothing else.** There
  is no channel for supporters only, and nothing is sold
  ([ADR-0014](0014-roles-managed-notification-recognition.md), "What boosting
  and supporting give"; [product](../product.md), "Paid perks").
- **The credit page lives outside the repository** and lists a name only with
  the person's consent. Release notes link to it and refuse supporter names
  (issues #125 and #194; [hosting](../hosting.md), `SUPPORTER_NAMES`).
- **"Honorary supporter" is given by hand.** No function that reads a funding
  platform gives, keeps or takes it
  ([ADR-0016](0016-honorary-supporter-and-third-subject-guide.md)).
- **The two funding integrations' own roles are managed by them.** "Ko-fi Bot"
  (`funding-ko-fi`) and "Patreon" (`funding-patreon`) are checked and never
  created, changed or deleted by this tool (ADR-0014).

What bounds the choices:

- **Data.** [ADR-0002](0002-data-and-compliance.md) keeps a stored record to
  the identifiers, timestamps and state a feature needs, with an entry in the
  [data inventory](../compliance/data-inventory.md) and a retention period for
  every stored field. Who pays whom is personal data of the member.
- **Network.** The bot has no inbound traffic ([hosting](../hosting.md), "In
  short"). Today it holds no database; the news function's state file is the
  only volume.
- **Rights.** The bot gives a role only when that role stands below its own
  ([ADR-0010](0010-bot-rights-and-apply-path.md), "The role order bounds the
  right to manage roles"). Both supporter roles do.

### What the integrations do

The platforms' help articles were read in full on 2026-10-18 (Sources). They
show:

- **Both integrations give a role the creator chooses** for a membership tier,
  and take it away when the membership ends. Neither gives its own bot role
  ("Ko-fi Bot", "Patreon") to a member: that role is the integration's
  identity on the server.
- **Ko-fi** takes a monthly or tier role away when the subscription ends. A
  role for one-time support never expires; the creator takes it away by hand.
  On an overdue payment the articles disagree: the rewards article says the
  role goes at once, the memberships article says it stays while the payment
  is retried. Ko-fi asks that a role it gives is not managed by another
  party, and its integration needs a creator account with Contributor or
  Standard status.
- **Patreon** gives roles for paid tiers only, takes a role away when a pledge
  is deleted or a payment is declined, and syncs once a day. It removes
  anyone added by hand to a role it manages, and its own bot role must never
  be set on a tier.
- **Both integrations' bots need** the rights to manage roles and to create
  invites, and must stand above the roles they give.

One role shared by both integrations would therefore break: Patreon would take
away what Ko-fi gave, and Ko-fi asks for a role nobody else manages.

## Decision

### Platforms: Ko-fi and Patreon

The function covers the two platforms whose integrations the server already
has and the definition already names: Ko-fi and Patreon. Another platform needs
a change to this record.

### Link: the bot follows one role per platform

Each integration gives a role of its own to its supporters. The definition
holds them as two hidden recognition roles without rights:

- "Ko-fi supporter" (`supporter-ko-fi`), given by the Ko-fi integration;
- "Patreon supporter" (`supporter-patreon`), given by the Patreon integration.

Both have `assigned_by: integration`, so the bot never gives or takes them.
Nothing attaches to them: no channel, no right, no perk. They are no thanks on
their own; they only tell the bot who supports.

The bot gives "Supporter" to a member who holds either role and takes it from
a member who has held neither for longer than the grace period below.

The bot needs no platform credentials and no inbound traffic, and asks no
platform anything.

### Data: none stored

The bot reads role membership on the server and stores nothing about a
member: no link to a platform account, no amount, no date. The data inventory
gains no field.

The state of a grace period, the time a member was last seen without either
role, is held in memory only. A restart loses it and starts the grace period
again; it never takes a role earlier than the period allows.

### Removal: after a grace period

"Supporter" is taken away once a member has held neither platform role for a
grace period. The period is configuration. It is at least one day, because
Patreon syncs once a day; its default is set when the function is built
(#196).

A member who asks for their data to be deleted under ADR-0002 has no stored
data here. The roles stay as the integrations and the grace period leave them.

### Ko-fi: monthly support and tiers only

The Ko-fi integration is set up for monthly support and membership tiers only.
One-time support gets no role from it, since Ko-fi never takes such a role
away. One-time support is credited by hand on the credit page.

### What the function does not touch

- "Honorary supporter" (ADR-0016).
- The integrations' own bot roles and the two platform roles.
- "Supporter for a year". It has no source under this record: no platform role
  carries a duration, and no member data is kept from which one could be
  counted. The function does not give it. It stays open until a platform
  reports the duration through its role or a new record decides.

### Set up on the server by the operator

The platform roles exist in the definition and are created like any other
recognition role. Setting up the integrations needs the operator's accounts on
Ko-fi and Patreon:

- the Ko-fi integration for monthly support and tiers, each mapped to "Ko-fi
  supporter";
- each paid Patreon tier mapped to "Patreon supporter";
- both integration bot roles placed above the two platform roles.

## Consequences

- **Positive**: The bot holds no platform credentials, no inbound address and
  no member data. The privacy texts change only to say that the bot reads who
  holds a role.
- **Positive**: Each integration stays the only party that manages its own
  role, as both platforms require.
- **Negative**: The bot cannot check how the integrations are set up on the
  platforms. A tier mapped to the wrong role shows on the server only.
- **Negative**: "Supporter for a year" stays without a giver.
- **Negative**: A restart starts a running grace period again, so a role can
  stay longer than the period.

## Alternatives considered

- **The integrations give "Supporter" themselves.** No code at all, but both
  integrations would manage the same role, which Patreon breaks by removing
  what Ko-fi gave.
- **The bot follows the integrations' bot roles.** No member holds those roles;
  they are the integrations' own identity.
- **The bot reads the platforms' interfaces.** Exact durations, but platform
  credentials in the bot, inbound traffic for Ko-fi's webhook and stored
  payment-adjacent data.
- **Discord's Linked Roles** (issue #56). A public web address, a token per
  member and a second sign-in.
- **Keep a first-seen date per member** for "Supporter for a year". Declined:
  no member data is stored.
- **Give both supporter roles by hand.** A role given by hand is then taken by
  nobody when support ends.

## Open points

- **How the bot learns who holds a platform role.** Listing a role's members
  needs Discord's server members intent, which the bot's connection does not
  ask for today ([ADR-0001](0001-architecture-and-boundaries.md)). Decided
  when the function is built (#196).
- **The default grace period.** At least one day; set in #196.
- **Ko-fi on an overdue payment.** Its articles disagree; tried on the server
  once the integration is set up. The grace period covers either reading.

## Sources

- Diallak repository issue #196:
  <https://github.com/tekmora/diallak/issues/196>.
- Diallak repository issue #56 (Linked Roles):
  <https://github.com/tekmora/diallak/issues/56>.
- `server/definition.json`: roles `supporter`, `supporter-12-months`,
  `honorary-supporter`, `supporter-ko-fi`, `supporter-patreon`,
  `funding-ko-fi`, `funding-patreon`; function `supporter-roles`.
- Ko-fi Help, read in full 2026-10-18 through the help centre's article
  interface:
  - "Setting up Discord rewards with Ko-fi", article 360020363857, updated
    2026-09-30;
  - "How do supporters join my Discord server?", article 8664701197073,
    updated 2026-10-17;
  - "Ko-fi Memberships and Membership Tiers", article 4402945994001, updated
    2026-10-15.
- Patreon Help Center, read in full 2026-10-18 through the help centre's
  article interface:
  - "Setting up Discord for your members", article 213552323, updated
    2026-10-08;
  - "Getting Discord access", article 212052266, updated 2026-10-18;
  - "Discord isn't working for my members", article 360037465012, updated
    2026-09-10;
  - "Troubleshooting Discord issues", article 4408372541581, updated
    2026-10-16.
- Patreon, API reference, <https://docs.patreon.com/>, read 2026-10-18 for the
  interface option.
