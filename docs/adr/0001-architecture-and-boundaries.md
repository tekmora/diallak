# ADR-0001: Architecture and boundaries

- **Status**: Proposed
- **Date**: 2026-10-10
- **Authors**: Maintainers

## Context

The bot serves one community hub server. Its planned functions are listed in the
[README](../../README.md): support tickets, bug reports, votes, supporter roles
and answers from the projects' documentation.

Four things shape the architecture:

- **One server.** The bot is not a product for many servers. It serves a single
  hub guild.
- **A public repository.** The repository becomes public later, so the tree must
  never hold a credential, the real guild id or the name of any private system.
- **A bot is already running.** The bot that serves the hub today stays
  connected until this one replaces it. This repository does not change it.
- **The repository's rules.** Every I/O call needs a deadline and every loop a
  bound ([AGENTS.md](../../AGENTS.md), HISS-02), and CI must pass without a
  Discord token.

## Decision

### Language and library

The bot is written in Go and talks to Discord through
[disgo](https://github.com/disgoorg/disgo) (Apache-2.0). The version is pinned
in `go.mod` and `go.sum` when the first feature adds the dependency. On the date
of this record the latest release was v0.19.6 of 2026-06-07 (`gh api
repos/disgoorg/disgo/releases/latest`).

### One hub guild, given as configuration

The guild is configuration, not code. `DISCORD_BOT_GUILD_ID` names it, and
[`internal/config`](../../internal/config/config.go) refuses a missing, zero or
non-decimal value.

- Commands are registered for that guild only. The bot registers no global
  command.
- The real guild id never enters the tree. Tests use invented ids.

### The token comes from a file

`DISCORD_BOT_TOKEN_FILE` names a file that holds the bot token. The token is
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

The planned layout follows. Package names settle with the first feature.

| Package | Role |
| :--- | :--- |
| `cmd/discord-bot` | Wiring, signal handling, exit codes |
| `internal/config` | Configuration, validated at start |
| `internal/discord` | The disgo adapter; the only importer of disgo |
| `internal/answer` | The answer pipeline ([ADR-0003](0003-answer-pipeline.md)) |

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

## Open decisions

These are not settled by this record and get their own:

- **Where state is stored.** The working assumption is SQLite through a pure-Go
  driver, behind a store interface. It is decided with the first feature that
  stores state, together with the retention periods of ADR-0002.
- **GitHub write access.** The first version needs none: a bug report becomes a
  prefilled new-issue link that the member submits under their own account. A
  later write path gets its own record.
