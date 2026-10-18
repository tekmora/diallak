# ADR-0026: The link between a member and a forge account

- **Status**: Accepted
- **Date**: 2026-10-18
- **Authors**: Maintainers

## Context

The role "Contributor" (`contributor`) says that a contribution of the member
was merged. It is given by the bot (`assigned_by: bot`), and the function that
gives it, `contributor-role`, is `planned` and needs a record
(`server/definition.json`). The bot can give the role only when it knows which
member a merged contribution belongs to. [ADR-0010](0010-bot-rights-and-apply-path.md)
leaves that link open ("Open points": "The link between a member and a forge
account"): it is member data and gets its own record before the function is
built. Issue [#56](https://github.com/tekmora/diallak/issues/56) notes
Discord's Linked Roles as a candidate that this record has to weigh.

What bounds every option:

- **No inbound path.** The bot opens every connection itself
  ([ADR-0001](0001-architecture-and-boundaries.md)); the deployment accepts no
  inbound traffic ([hosting](../hosting.md), "In short").
- **Only the data a feature needs** is stored, with an inventory entry and a
  retention period, and a member can ask for its deletion
  ([ADR-0002](0002-data-and-compliance.md)).
- **The definition names who gives each role** (`assigned_by`), and its
  validator checks it ([server definition](../server-definition.md)).

## What Discord's Linked Roles offer

Read on 2026-10-18 in Discord's developer documentation, repository
`discord/discord-api-docs` at commit `f7416a2cf165` (2026-10-16):

- **The application names a verification address**,
  `role_connections_verification_url`, set in the developer portal or through
  `PATCH /applications/@me`. A server can then pick the application as a
  verification method for a role (`resources/application.mdx`;
  `resources/application-role-connection-metadata.mdx`).
- **The application declares up to 5 metadata records**, each a key and a
  comparison: integer less, greater, equal or not equal, a date compared in
  days before today, or a boolean (`PUT
  /applications/{application.id}/role-connections/metadata`). The server sets
  the value a member must meet in the role's settings.
- **A member's connection is written with the member's own token.** `PUT
  /users/@me/applications/{application.id}/role-connection` takes a platform
  name, a platform user name and the metadata values, and needs an OAuth2
  access token of that member with the scope `role_connections.write`
  (`resources/user.mdx`). The scope cannot be used with the implicit grant
  (`topics/oauth2.mdx`).
- **Tokens expire.** The example token response holds `expires_in: 604800`
  (seven days) and a `refresh_token`; to keep a connection current after that,
  the application keeps and uses the refresh token (`topics/oauth2.mdx`).
- **Discord, not the bot, gives the role.** A role that is a linked role
  carries the tag `guild_connections` (`topics/permissions.mdx`). The JSON
  parameters of "Modify Guild Role" hold no field for the role's condition
  (`resources/guild.mdx`), so the condition is set by hand in the server's
  settings as far as the documentation shows.

A second Discord path exists: the OAuth2 scope `connections` lets
`GET /users/@me/connections` return a member's linked accounts, each with a
`type` (for example `github`), an `id`, a `name` and `verified`
(`resources/user.mdx`, "Connection Object"). It also needs the member to sign
in through OAuth2.

## The options weighed

<!-- markdownlint-disable MD013 -->
| Option | How it works | For | Against |
| :--- | :--- | :--- | :--- |
| A. Linked Roles | The bot serves a verification page; the member signs in to Discord (`role_connections.write`) and to the forge; the bot writes "has a merged contribution" as a boolean, and Discord gives the role. | The member consents in Discord's own flow; Discord shows the link on the profile. | A public web address and an OAuth2 flow, against ADR-0001; a refresh token per member to keep the value current; the condition set by hand; the definition's `assigned_by` cannot describe it yet. |
| B. The bot keeps the link, proven by a code | A member runs a command; the bot replies, to that member only, with a one-time code; the member writes the code in a comment on the forge under their account; the bot reads the comment through the forge's interface and stores the link. | No inbound path and no token of the member; the forge proves the account. | The bot stores a member id and a forge account per member; a comment written for this is public on the forge. |
| C. Discord's account connections | A sign-in page reads the member's verified GitHub connection (`connections` scope) once and stores the link. | Uses the account the member already linked in Discord. | The same public web address and OAuth2 flow as A; only for members who linked GitHub in Discord. |
| D. By hand | A maintainer gives the role when a member asks and the merge is seen. | No code, no stored data. | The definition's `assigned_by: bot` changes; nobody takes the role away if that is ever wanted. |
<!-- markdownlint-enable MD013 -->

Row A is as first written, with the bot as the verifier; the decision moves
that part to the identity provider. Under A the verifier keeps a refresh token per member to keep the value
current; under B and C the bot keeps one member id and one forge account per
member; under D nothing is kept.

## Decision

Option A, Discord's Linked Roles, with the cluster's existing identity
provider as the verifier. The bot serves no part of it.

- **The identity provider serves the verification address and the OAuth2
  callback.** The application's `role_connections_verification_url` points at
  the provider. The member signs in there to Discord, with the scopes
  `identify` and `role_connections.write`, and to the forge. The provider
  writes the value "has a merged contribution" to the member's role connection
  with the member's own token.
- **The bot keeps no inbound route.** [ADR-0001](0001-architecture-and-boundaries.md)
  is unchanged: the deployment of the bot still accepts no inbound traffic, and
  no record supersedes it. The bot holds no token of a member, no forge account
  and no link.
- **Discord gives the role.** Contributor becomes a linked role whose
  condition names the provider's metadata record. In the server definition
  the role is `assigned_by: integration`, like the roles the funding platforms
  give ([ADR-0024](0024-supporter-roles-from-funding-platforms.md)), and the
  function `contributor-role` is done by the platform, not the bot.

### What the identity provider keeps

The data is the provider's, not the bot's. It is listed here, in the
[data inventory](../compliance/data-inventory.md), the
[record of processing](../compliance/record-of-processing.md) and the
[privacy notice](../compliance/privacy-notice.md), because the operator runs
the provider for the same server.

<!-- markdownlint-disable MD013 -->
| Field | Why | Kept until |
| :--- | :--- | :--- |
| The member's Discord user id | Ties the link to the member | The link ends |
| The Discord refresh token with `role_connections.write`, and the access token it yields | Keeps the value current after a later merge; the access token expires after seven days | The link ends; the access token also at its expiry |
| The forge account's id and name | Proves the account and finds its merged contributions | The link ends |
| The value written to the role connection | What Discord compares with the role's condition | The link ends; Discord also holds it |
<!-- markdownlint-enable MD013 -->

The link ends when the member removes the application's authorisation in
Discord, which makes the provider's next token refresh fail, or when the member
asks for deletion. On either, the provider deletes the four fields; on a
request it first deletes the role connection, so Discord takes the role.

## Consequences

- **Positive**: The open point of ADR-0010 is written down with the platform's
  answer beside the bot's own, before any member data is read.
- **Positive**: The bot's boundary stays as ADR-0001 draws it; the one
  inbound path is the identity provider's, which the cluster runs already.
- **Positive**: Discord's own flow asks the member's consent and shows the
  link on the profile.
- **Negative**: Member data is kept for this, by the provider: a refresh token
  per linked member for as long as the link stands.
- **Negative**: The role's condition is set by hand in the server's settings;
  the definition's apply path cannot set it.

## Alternatives considered

- **The forge's own Discord integration.** Not looked into; no source was read
  for it.
- **Reading the member's name and matching it to a forge account.** Ruled out:
  a name is not a proof, and ADR-0002 identifies a member by the user id of the
  interaction, never by a name they type.

## Open points

- **The provider's setup.** Which provider version serves the flow, at which
  address, how it reads merged contributions from the forge, and how often it
  refreshes the value: named by the cluster's own record, on order
  k8s-idp-linked-roles-20261018.
- **The metadata record.** Answered: one boolean record, key
  `merged_contribution`, in `server/role-connections.json`, with its value
  rule in `server/role-connection-values.json` (merged pull request in a
  public, non-fork repository of the seven listed organisations; private
  repositories are never read; decision diallak-linked-roles-merge-scope,
  2026-10-18). `diallak -role-connections=plan` prints the request without
  sending it; `-role-connections=push=<digest>` sets the record once, after
  the provider's card closes. Evaluating the rule stays the provider's.
- **Whether a linked role's condition can be set through the API.** The pages
  read show no such parameter; not tried.

## Decided in this record

Option A, with the cluster's identity provider as the verifier and Discord
giving the role. The bot gets no inbound route and keeps no data for it.
Accepted on an operator decision through the controller (decision ids
diallak-adr-0026-accept and diallak-adr-0026-accept-refined, 2026-10-18), after
earlier fleet decisions that one identity runs through the existing identity
provider with Discord's linked roles (2026-10-10 and 2026-10-11).

## Sources

- Diallak repository issue #56:
  <https://github.com/tekmora/diallak/issues/56>.
- Discord developer documentation, repository `discord/discord-api-docs` at
  commit `f7416a2cf165`, read 2026-10-18:
  `developers/resources/application-role-connection-metadata.mdx`;
  `developers/resources/user.mdx` ("Application Role Connection Object",
  "Update Current User Application Role Connection", "Connection Object");
  `developers/resources/application.mdx` ("Edit Current Application");
  `developers/topics/oauth2.mdx` (scopes, token response, refresh);
  `developers/topics/permissions.mdx` (role tags);
  `developers/resources/guild.mdx` ("Modify Guild Role").
- `server/definition.json`: role `contributor`, function `contributor-role`.
