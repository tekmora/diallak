# discord-bot

<!-- praetor:readme-governance:start -->
[![HISS Adopted][praetor-hiss-badge]][praetor-hiss-agents]

Praetor manages this repository's declared governance policy. This managed
block records adoption state; it is not a verification certificate.

**Verification**: `make verify-all` runs the repository's configured
verification cascade.

**HISS Audit**: `praetorctl audit` enforces policy, generated-surface
integrity, and the debt ratchet.

**Context Sync**: `praetorctl compile-context --verify` verifies every
generated agent context against `AGENTS.md`.

**Debt Baseline**: `.standards-baseline.json` anchors the debt ratchet at
0 recorded infractions; audit forbids growth.

[praetor-hiss-badge]: https://img.shields.io/badge/Standards-HISS%20Adopted-blue
[praetor-hiss-agents]: https://github.com/tekmora/discord-bot/blob/HEAD/AGENTS.md
<!-- praetor:readme-governance:end -->

Community bot for the Lusoris Discord hub.

> **Status: first slice built, not live.** The bot answers `/ask` from project documentation with
> cited sources. It has been tested against a fake Discord only; the live test and the switch from
> the bot that runs today are still to come.

## What it will do

- **Support and tickets:** a member opens a ticket, the bot keeps the thread and its state.
- **Bug reports:** a report in Discord becomes a GitHub issue draft with a link back to the thread.
- **Votes and wishes:** community polls whose results feed the projects' planning.
- **Supporter perks:** roles for sponsors on the supported funding platforms.
- **Answers from the docs:** a locally run model answers from the projects' documentation. Every
  answer cites its sources and is labelled as AI-generated.

## What it will not do

- It carries no functions of any private infrastructure.
- It does not train on messages. Text from Discord is used to answer the request at hand and is
  never used for model training or evaluation data.

## Decisions so far

| Topic | Decision |
| :--- | :--- |
| Language and library | Go with disgo |
| Server | One hub server; the guild is configuration, not code |
| Auto-answers, first version | Local-model answers with citations and an AI label |
| Discord application | The existing bot application and its token are reused |
| Community questions | GitHub Discussions for searchable answers, the hub for live chat; the bot links them |
| Moderation | The bot filters spam and obvious abuse; people decide unclear cases |
| Compliance | Discord's developer terms and policy, GDPR, and the transparency duty of EU AI Act Art. 50; the provider-or-deployer role assessment is written before launch |

## The name

The product is called **Diallak** (dee-AHL-lak), from Greek *diallaktes*, the impartial mediator.
The repository keeps the name `discord-bot` until the organisation moves. Tekmora (tek-MOH-ra), the
family the name belongs to, comes from Greek *tekmerion*, "decisive proof".

## Run

The bot reads its settings from the environment and fails closed when one is missing or malformed
([`internal/config`](internal/config/config.go)).

| Variable | Required | Meaning |
| :--- | :--- | :--- |
| `DISCORD_BOT_TOKEN_FILE` | yes | Path of the file that holds the bot token |
| `DISCORD_BOT_GUILD_ID` | yes | Id of the one server the bot serves |
| `DISCORD_BOT_CORPUS_DIR` | yes | Absolute path of the documentation directory |
| `DISCORD_BOT_MODEL_BASE_URL` | with the alias | Base URL of an OpenAI-compatible endpoint |
| `DISCORD_BOT_MODEL_ALIAS` | with the base URL | Name the endpoint routes on, never a model name |
| `DISCORD_BOT_MODEL_KEY_FILE` | no | Path of the file that holds the endpoint's credential |

Without the two model variables the bot replies with cited excerpts and says that no model wrote
them.

### The documentation directory

The deployment fills the directory; the bot fetches nothing. It holds one checkout per source and a
`manifest.json` that names each source and the revision it was taken from
([`internal/corpus`](internal/corpus/manifest.go)):

```json
{
  "version": 1,
  "sources": [
    {
      "directory": "snatcharr",
      "repository": "lusoris/SnatchArr",
      "revision": "9e70c2e03d22d535a9f89c628807aab5813829a6",
      "paths": ["README.md", "docs/**/*.md"]
    }
  ]
}
```

The revision appears in every citation. The bot refuses a path or a symbolic link that leaves the
directory.

### Starting the bot

```bash
discord-bot
```

It loads the documentation, connects to Discord and answers `/ask` until it receives a termination
signal. A reply is visible only to the member who asked and can never mention anyone.

### Commands are registered by hand

```bash
discord-bot -register-commands
```

This sets the server's command list for the application to `/ask` and exits. It needs the same
environment as a normal start. Starting the bot never registers or changes a command.

The application is shared with the bot that runs today
([ADR-0001](docs/adr/0001-architecture-and-boundaries.md)), so this step replaces that bot's
commands in the server. It belongs to the cutover and is the operator's act.

## Release

Publishing a GitHub release with a tag `vX.Y.Z` builds the container image
`ghcr.io/tekmora/discord-bot:X.Y.Z` ([`release.yml`](.github/workflows/release.yml)). There is no
`latest` tag. Merging to `main` releases nothing.

The image is built in a reusable workflow of its own
([`release-image.yml`](.github/workflows/release-image.yml)) that also signs it with cosign, attests
its build provenance and writes an SBOM, which is attached to the release. To check a signature:

```bash
cosign verify ghcr.io/tekmora/discord-bot:X.Y.Z \
  --certificate-identity-regexp 'https://github.com/tekmora/discord-bot/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

No release exists yet. On the current plan GitHub offers build attestations for public repositories
only, so the first release waits until this repository is public.

## Develop

```bash
make verify-all   # context check, audit, build and tests
```

The retrieval evaluation runs with the tests. To see its numbers for the SnatchArr fixture
([`internal/eval/testdata/snatcharr`](internal/eval/testdata/snatcharr/README.md)):

```bash
go test -v ./internal/eval -run TestSnatchArrEval
```

Git hooks run the Praetor engine this repository pins (`PRAETOR_REF` in
[`.github/workflows/ci.yml`](.github/workflows/ci.yml)) through
[`.config/lefthook/engine.sh`](.config/lefthook/engine.sh). Agent instructions live in
[`AGENTS.md`](AGENTS.md); the other agent files are compiled from it.

## Licence

[EUPL-1.2](LICENSE).
