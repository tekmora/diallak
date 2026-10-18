# ADR-0001: Architecture and boundaries

- **Status**: Accepted
- **Date**: 2026-10-10
- **Authors**: Maintainers
- **Amended by**: [ADR-0027](0027-members-intent-for-supporter-roles.md),
  "The bot requests no privileged intent"

## Context

The bot serves one community hub server. Its planned functions are listed in the
[README](../../README.md): support tickets, bug reports, votes, supporter roles
and answers from the projects' documentation.

Four things shape the architecture:

- **One server.** The bot is not a product for many servers. It serves a single
  hub guild.
- **A public repository.** The repository is public, so the tree must never
  hold a credential, the real guild id or the name of any private system.
- **A bot is already running.** The bot that serves the hub today stays
  connected until this one replaces it. This repository does not change it.
- **The repository's rules.** Every I/O call needs a deadline and every loop a
  bound ([AGENTS.md](../../AGENTS.md), HISS-02), and CI must pass without a
  Discord token.

## Decision

### Language and library

The bot is written in Go and talks to Discord through
[disgo](https://github.com/disgoorg/disgo) (Apache-2.0). `go.mod` and `go.sum`
pin v0.19.6 of 2026-06-07, the latest release on the date of this record (`gh
api repos/disgoorg/disgo/releases/latest`).

### One hub guild, given as configuration

The guild is configuration, not code. `DIALLAK_GUILD_ID` names it, and
[`internal/config`](../../internal/config/config.go) refuses a missing, zero or
non-decimal value.

- Commands are registered for that guild only. The bot registers no global
  command.
- The real guild id never enters the tree. Tests use invented ids.

### The token comes from a file

`DIALLAK_TOKEN_FILE` names a file that holds the bot token. The token is
never passed as an environment value and never logged. `internal/config` bounds
the read and fails closed on an empty or oversized file;
`internal/config/config_test.go` covers both.

The operator creates the Discord application and its token. The bot only reads
the file.

### A gateway connection, least privilege

The bot keeps a gateway connection. It does not expose an HTTP interactions
endpoint, so it needs no inbound network path. Later features such as poll votes
and thread events arrive as gateway events.

The first version requests no privileged gateway intent. Slash commands arrive
as interactions and need none. A feature that needs a privileged intent adds its
own ADR, because the intent changes what member data the bot receives
([ADR-0002](0002-data-and-compliance.md)).

### Discord is an adapter behind ports

Only one package imports disgo. It translates Discord events into calls on small
interfaces that the feature packages define, and translates their results back
into Discord responses.

- Feature code depends on those interfaces, not on disgo types.
- Tests run the features against a fake Discord. CI needs no token and opens no
  connection.
- A library change touches the adapter package only.

The layout (`go list ./...`):

| Package | Role |
| :--- | :--- |
| `cmd/diallak` | Wiring, signal handling, exit codes |
| `internal/config` | Configuration, validated at start |
| `internal/discord` | The disgo adapter; the only importer of disgo |
| `internal/app` | Builds the pipeline from the configuration |
| `internal/corpus` | Reads the documentation directory and its manifest |
| `internal/retrieval` | The index over the documentation |
| `internal/model` | Client of the model endpoint |
| `internal/answer` | The answer pipeline ([ADR-0003](0003-answer-pipeline.md)) |
| `internal/eval` | The evaluation set and its bar |

### One process, bounded work

The bot runs as a single process with a single gateway session. One guild needs
no sharding.

- Every outbound call carries a `context.Context` deadline.
- Concurrent work has a fixed upper bound.
- On a termination signal the bot stops taking new work and closes the gateway
  within a deadline.

### The existing Discord application

The bot runs under the Discord application the current bot uses, with the same
token. The application is renamed through the API. This is the operator's
decision for now; a move to an application of its own would be a new record.

- One token serves one bot. The two bots do not run side by side: the cutover
  stops the current bot and starts this one, and rolling back starts the current
  bot again.
- The token stays in the deployment's secret store. This bot reads it from a
  file, as above.
- The bot requests no privileged intent, whatever the application has enabled.
- The bot asks the platform for its application id at start-up and never takes
  it from the token: for an older application the two differ. A failed lookup
  stops the start (`lookupApplicationID` in
  [`internal/discord`](../../internal/discord/disgo.go)).
- Commands the current bot registered belong to the same application. The
  cutover lists them and removes the ones this bot does not serve.
- Until the cutover every feature is tested against a fake Discord. A live test
  replaces the current bot for its duration and is the operator's act.

### An image and an environment contract

This repository builds a container image ([Dockerfile](../../Dockerfile)) and
documents the environment variables the bot reads. Deployment manifests live
outside this repository.

### No function of any private infrastructure

The bot carries no function of any private infrastructure. It talks to the
Discord API and to endpoints given in configuration: the model endpoint of
ADR-0003 and, with later features, public APIs such as the funding platforms. No
private host name, cluster name or internal service name appears in code or
documentation.

## Consequences

- **Positive**: The bot needs no public endpoint and no privileged intent to
  start. Features are testable without Discord. The guild, the token and every
  endpoint can change without a code change. The tree is safe to publish.
- **Negative**: A gateway bot must stay connected, so it runs as a long-lived
  process and cannot run as a function invoked per request. The adapter boundary
  costs some translation code. A second guild would need a design change, by
  intent.

## Alternatives considered

- **discordgo and arikawa.** Both are Go libraries. Their latest releases were
  v0.29.0 of 2025-05-24 and v3.6.0 of 2025-09-04; disgo was the most recently
  released of the three.
- **An HTTP interactions endpoint.** It needs a public inbound endpoint and
  receives interactions only. Gateway events such as poll votes would not reach
  the bot.
- **A new Discord application.** It would let both bots run side by side until
  the cutover and would start with no privileged intent enabled. The operator
  chose to keep the one application the hub already has installed.
- **Global commands.** They would appear in every guild that installs the
  application. The bot serves one guild.

## Decided in this record

The operator decided the language and the library, the single hub guild, the
reuse of the existing application and that the bot carries nothing of any
private infrastructure. Reading the application id from the platform, the image
with its environment contract and the two working assumptions under "Open
decisions" follow from recorded decisions.

The choices below have no decision of their own outside this file. They were
made here and are accepted with this record:

- A gateway connection instead of an HTTP interactions endpoint.
- No privileged gateway intent in the first version; a feature that needs one
  adds its own record.
- Credentials are read from files, never from environment values, and a read
  that fails stops the start.
- Commands are registered for the hub guild only, never globally.
- One package imports disgo. Features depend on their own interfaces and are
  tested against a fake Discord.
- One process and one gateway session, a deadline on every outbound call, a
  fixed bound on concurrent work and a shutdown within a deadline.

## Open decisions

These are not settled by this record and get their own:

- **Where state is stored.** The working assumption is SQLite through a pure-Go
  driver, behind a store interface. It is decided with the first feature that
  stores state, together with the retention periods of ADR-0002.
- **GitHub write access.** The first version needs none: a bug report becomes a
  prefilled new-issue link that the member submits under their own account. A
  later write path gets its own record.
