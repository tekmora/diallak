# Diallak

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
[praetor-hiss-agents]: https://github.com/tekmora/diallak/blob/HEAD/AGENTS.md
<!-- praetor:readme-governance:end -->

Community bot for the Lusoris Discord hub.

> **Status: first slice built, not live.** The bot answers `/ask` from project
> documentation with cited sources. It has been tested against a fake Discord
> only; the live test and the switch from the bot that runs today are still to
> come.

## What it will do

- **Support and tickets:** a member opens a ticket, the bot keeps the thread and
  its state.
- **Bug reports:** a report in Discord becomes a GitHub issue draft with a link
  back to the thread.
- **Votes and wishes:** community polls whose results feed the projects'
  planning.
- **Supporter perks:** roles for sponsors on the supported funding platforms.
- **Answers from the docs:** a locally run model answers from the projects'
  documentation. Every answer cites its sources and is labelled as AI-generated.

## What it will not do

- It carries no functions of any private infrastructure.
- It does not train on messages. Text from Discord is used to answer the request
  at hand and is never used for model training or evaluation data.

## Decisions so far

<!-- markdownlint-disable MD013 -->
| Topic | Decision |
| :--- | :--- |
| Language and library | Go with disgo |
| Server | One hub server; the guild is configuration, not code |
| Auto-answers, first version | Local-model answers with citations and an AI label |
| Discord application | The existing bot application and its token are reused |
| Community questions | GitHub Discussions for searchable answers, the hub for live chat; the bot links them |
| Moderation | The bot filters spam and obvious abuse; people decide unclear cases |
| Compliance | Discord's developer terms and policy, GDPR, and the transparency duty of EU AI Act Art. 50; the provider-or-deployer role assessment is written before launch |
<!-- markdownlint-enable MD013 -->

## The name

The bot is called **Diallak** (dee-AHL-lak), from Greek *diallaktes*, the
impartial mediator. Tekmora (tek-MOH-ra), the family the name belongs to, comes
from Greek *tekmerion*, "decisive proof". The repository was first named
`discord-bot`; its old address redirects.

## Run

The bot reads its settings from the environment and fails closed when one is
missing or malformed ([`internal/config`](internal/config/config.go)).

<!-- markdownlint-disable MD013 -->
| Variable | Required | Meaning |
| :--- | :--- | :--- |
| `DIALLAK_TOKEN_FILE` | yes | Path of the file that holds the bot token |
| `DIALLAK_GUILD_ID` | yes | Id of the one server the bot serves |
| `DIALLAK_CORPUS_DIR` | yes | Absolute path of the documentation directory |
| `DIALLAK_MODEL_BASE_URL` | with the alias | Base URL of an OpenAI-compatible endpoint |
| `DIALLAK_MODEL_ALIAS` | with the base URL | Name the endpoint routes on, never a model name |
| `DIALLAK_MODEL_KEY_FILE` | no | Path of the file that holds the endpoint's credential |
| `DIALLAK_SUPPORT_CHANNEL` | no | Wording for where a person helps, put into every reply; default "the support channel" |
<!-- markdownlint-enable MD013 -->

Without the two model variables the bot replies with cited excerpts and says
that no model wrote them. In the live server that is the only mode for now: no
deployment sets the two variables until the gates of
[ADR-0006](docs/adr/0006-ai-act-role.md) are passed.

### Optional settings

<!-- markdownlint-disable MD013 -->
| Name | Default | Range |
| :--- | :--- | :--- |
| `DIALLAK_TOP_PASSAGES` | `3` | `1..10` |
| `DIALLAK_MIN_SCORE` | `0.20` | greater than 0, at most 1 |
| `DIALLAK_QUESTION_MAX_CHARS` | `500` | `1..500` |
| `DIALLAK_EXCERPT_CHARS` | `400` | `50..1000` |
| `DIALLAK_REPLY_MAX_CHARS` | `1900` | `500..2000` |
| `DIALLAK_MAX_CONCURRENT_ASKS` | `4` | `1..64` |
| `DIALLAK_ASK_TIMEOUT` | `60s` | `1s..10m` |
| `DIALLAK_MODEL_TIMEOUT` | `45s` | `1s..2m` and strictly below ask timeout |
| `DIALLAK_SOURCE_LINK_TEMPLATE` | `https://github.com/{repository}/blob/{revision}/{path}` | `https://` URI with `{repository}`, `{revision}`, `{path}`, at most 200 chars |
<!-- markdownlint-enable MD013 -->

### The documentation directory

The deployment fills the directory; the bot fetches nothing. It holds one
checkout per source and a `manifest.json` that names each source and the
revision it was taken from ([`internal/corpus`](internal/corpus/manifest.go)):

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

The revision appears in every citation. The bot refuses a path or a symbolic
link that leaves the directory.

### Starting the bot

```bash
diallak
```

It loads the documentation, connects to Discord and answers `/ask` until it
receives a termination signal. A reply is visible only to the member who asked
and can never mention anyone.

### Commands are registered by hand

```bash
diallak -register-commands
```

This sets the server's command list for the application to `/ask` and exits. It
needs the same environment as a normal start. Starting the bot never registers
or changes a command.

The application is shared with the bot that runs today
([ADR-0001](docs/adr/0001-architecture-and-boundaries.md)), so this step
replaces that bot's commands in the server. It belongs to the cutover and is the
operator's act.

## Release

Every pull request adds one changelog fragment under
[`changelog.d`](changelog.d):

```bash
praetorctl changelog new --type=fixed --title="Refuse a question that is too long"
```

A release is cut in three steps:

1. On a branch, `praetorctl release --version=vX.Y.Z --date=YYYY-MM-DD` renders
   the fragments into `CHANGELOG.md`. The branch is merged like any other
   change.
2. The merged commit on `main` is tagged `vX.Y.Z`, and the tag is pushed.
3. The tag starts [`release.yml`](.github/workflows/release.yml). It refuses a
   version that has no section in `CHANGELOG.md`, builds the container image
   `ghcr.io/tekmora/diallak:X.Y.Z`, and publishes the release with the changelog
   section as its notes. There is no `latest` tag.

The image is built in a reusable workflow of its own
([`release-image.yml`](.github/workflows/release-image.yml)) that also signs it
with cosign, attests its build provenance and writes an SBOM, which is attached
to the release.

No release exists yet, and merging to `main` releases nothing. How to check a
signature is described here once it has been tried against the first release.

## Develop

```bash
make verify-all   # context check, audit, build and tests
```

The retrieval evaluation runs with the tests. To see its numbers for the
SnatchArr fixture
([`internal/eval/testdata/snatcharr`](internal/eval/testdata/snatcharr/README.md)):

```bash
go test -v ./internal/eval -run TestSnatchArrEval
```

Git hooks run the Praetor engine this repository pins (`PRAETOR_REF` in
[`.github/workflows/ci.yml`](.github/workflows/ci.yml)) through
[`.config/lefthook/engine.sh`](.config/lefthook/engine.sh). Agent instructions
live in [`AGENTS.md`](AGENTS.md); the other agent files are compiled from it.

## Licence

Code: [EUPL-1.2](LICENSE). Documentation text, which is this file and everything
under [`docs/`](docs/): [CC BY-SA 4.0](LICENSES/CC-BY-SA-4.0.txt).
[`REUSE.toml`](REUSE.toml) states the licence of every path, and `reuse lint`
checks it.
