# ADR-0024: Supporter roles from the funding platforms

- **Status**: Proposed
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

This record is a proposal. It names what the function needs to decide and the
options for each point. Every point that touches member data, the link between
a member and a funding platform, or the taking away of a role is left open for
the operator; nothing below is built until this record is accepted.

## Decision

### Platforms: Ko-fi and Patreon

The function covers the two platforms whose integrations the server already
has and the definition already names: Ko-fi and Patreon. Another platform needs
a change to this record. This is the only point the proposal states as its
choice, since the definition fixes it already.

### Open: how a member is linked to a payment

<!-- markdownlint-disable MD013 -->
| Option | How it works | For | Against |
| :--- | :--- | :--- | :--- |
| A. The integrations give "Supporter" themselves | Each platform's own Discord integration is set to give the Supporter role for a payment or a membership tier; the bot does nothing. | No code, no stored data, no platform credentials; the member links their account on the platform, under the platform's terms. | The definition's `assigned_by: bot` and the function's `by: bot` change to the integration; "Supporter for a year" has no source unless a platform offers a matching tier; the bot cannot check that the integrations are set up as intended. |
| B. The bot follows the integrations' roles | The integrations give their own roles as today; the bot gives "Supporter" to a member who holds one of them and takes it from a member who holds none. | No platform credentials and no inbound traffic; the bot reads only role membership it already sees. | Depends on what each integration gives and when it takes it away, which is not read yet (Open points); "Supporter for a year" needs a start date the bot would have to keep. |
| C. The bot reads the platforms' interfaces | The bot asks each platform which members support and how long, and links them to server members by the Discord account they connected there. | Exact durations; both roles from one source. | Platform credentials in the bot; a webhook-only platform needs inbound traffic, which the bot does not have; the bot reads and may keep payment-adjacent data. |
| D. Discord's Linked Roles | The member signs in once through the bot's own connection page, and Discord gives the role from metadata the bot reports (issue #56). | The member consents in Discord's own flow. | A public web address, an OAuth2 token per member and a second sign-in; the definition's `assigned_by` cannot describe it yet. |
<!-- markdownlint-enable MD013 -->

### Open: what the bot reads and keeps

The answer follows from the link above.

- **Under A** the bot reads and keeps nothing.
- **Under B** the bot reads role membership, which it sees today. Giving
  "Supporter for a year" needs a first-seen date per member. Options: keep that
  date (one identifier and one timestamp per member, with an inventory entry
  and a retention period), or leave "Supporter for a year" to be given by hand
  until a platform reports the duration itself.
- **Under C and D** the bot reads data from outside Discord. Which fields, how
  long they are kept and how a member asks for their deletion would have to be
  listed field by field before anything is built.

The privacy texts and the data inventory change with the chosen option. They
are not part of this record.

### Open: when a role is taken away

<!-- markdownlint-disable MD013 -->
| Option | For | Against |
| :--- | :--- | :--- |
| At once, when support ends | The role says what is true today. | A failed payment or a short gap takes the role and gives it back. |
| After a grace period | Short gaps do not show. | A period is configuration and a timer; the bot needs to remember when support ended. |
| "Supporter for a year" stays once given | It thanks for a year that happened. | It is then a lasting role the bot gives, close to "Honorary supporter"; ADR-0016 keeps those apart. |
<!-- markdownlint-enable MD013 -->

Whatever is chosen, the function never touches "Honorary supporter"
(ADR-0016). Whether a member who asks for deletion of their data under
ADR-0002 keeps a role that rests on that data is part of this open point.

## Consequences

- **Positive**: The open points are written down before any member data is
  read, so the privacy texts and the code follow a decision instead of making
  it.
- **Positive**: Option A or B keeps the bot without platform credentials and
  without inbound traffic.
- **Negative**: Nothing ships until the operator decides the three open points.
  Version 1.0 waits on this record.
- **Negative**: Every option except A puts a new reason to change roles into
  the bot. A wrong role given to a member is visible to the server.

## Alternatives considered

- **Give both supporter roles by hand.** It needs no code. ADR-0016 already
  added "Honorary supporter" for standing given by hand; a supporter role given
  by hand would then be taken by nobody when support ends.
- **A private channel or a perk for supporters.** Declined by the operator
  earlier (ADR-0014, product). Not reopened.

## Open points

- **What the integrations give.** Which role each integration gives for a
  payment or a tier, whether it gives one of this server's roles or only its
  own, and when it takes it away. Read from the platforms' own help pages and
  tried on the server before acceptance. The search results read on
  2026-10-18 (Sources) say both integrations can give a chosen role; the full
  pages could not be fetched that day.
- **What the platforms' interfaces offer** (option C). Patreon's API reference
  was read on 2026-10-18 (Sources). What it says:
  - A user's `social_connections` field is a "mapping from user's connected app
    names to external user id on the respective app". The reference does not
    say whether a Discord account is always among them; that is tried before
    acceptance.
  - A campaign's members are listed by `GET
    /api/oauth2/v2/campaigns/{campaign_id}/members`, one member by `GET
    /api/oauth2/v2/members/{member_id}`. The bot could poll these without
    inbound traffic.
  - A member carries `patron_status`, `currently_entitled_amount_cents`,
    `pledge_relationship_start` (the start of the most recent pledge chain,
    not changed by pledge updates), `last_charge_date`, `last_charge_status`,
    `next_charge_date`, `pledge_cadence`, `campaign_lifetime_support_cents`,
    `is_free_trial` and `is_gifted`. The e-mail address needs a scope of its
    own, and the name may be empty.
  - Webhooks report members and pledges being created, updated and deleted.
    They need an inbound address, which the bot does not have.
  - The changelog lists member identity masking (2026-05-26) and the end of
    API v1 on 2026-10-07.

  Whether Ko-fi offers anything but a webhook is still not read: its help
  pages refused the fetch on 2026-10-18.
- **The decision itself.** The three open points above.

## Decided in this record

Nothing about member data, the link to a funding platform or the taking away
of a role. The record proposes only that the function covers Ko-fi and
Patreon, the two platforms the definition names.

Acceptance waits for the operator.

## Sources

- Diallak repository issue #196:
  <https://github.com/tekmora/diallak/issues/196>.
- Diallak repository issue #56 (Linked Roles):
  <https://github.com/tekmora/diallak/issues/56>.
- `server/definition.json`: roles `supporter`, `supporter-12-months`,
  `honorary-supporter`, `funding-ko-fi`, `funding-patreon`; function
  `supporter-roles`.
- Ko-fi, "NEW: Discord Integration - Give Roles and Invites to Supporters",
  <https://ko-fi.com/discord>, and Ko-fi Help, "How do supporters join my
  Discord server?",
  <https://help.ko-fi.com/hc/en-us/articles/8664701197073>: search result
  summaries read 2026-10-18 (roles for one-time support and for membership
  tiers); the pages themselves not read. A second fetch of the help page on
  2026-10-18 was refused with HTTP 403.
- Patreon Developers forum, "Campaign members list with their
  social_connections" (2024-07-07),
  <https://www.patreondevelopers.com/t/campaign-members-list-with-their-social-connections/8999>:
  search result summary read 2026-10-18.
- Patreon, API reference, <https://docs.patreon.com/>: sections "User v2"
  (`social_connections`), "Member" attributes, the members endpoints of API
  v2, webhook triggers v2 and the changelog (newest entry 2026-10-07), read
  2026-10-18. A first fetch earlier that day failed.
