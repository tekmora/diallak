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
- **Thanks to supporters:** a cosmetic role and a public credit in plain text.
  Nothing is sold: there is no supporter-only channel and no priority support.
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

The target state of the server itself, its roles, channels and permissions, is
data with a validator: [docs/server-definition.md](docs/server-definition.md).
Nothing applies it to a server yet; the steps that compare and plan are
described in [docs/server-apply.md](docs/server-apply.md).

## The name

The bot is called **Diallak** (dee-AHL-lak), from Greek *diallaktes*, the
impartial mediator. Tekmora (tek-MOH-ra), the family the name belongs to, comes
from Greek *tekmerion*, "decisive proof". The repository was first named
`discord-bot`; its old address redirects.

## Run

The bot reads its settings from the environment and fails closed when one is
missing or malformed ([`internal/config`](internal/config/config.go)).
What a deployment needs beyond these settings (image, network, resources, the
documentation directory) is in [docs/hosting.md](docs/hosting.md).

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
| `DIALLAK_HEALTH_ADDR` | no | Listen address "host:port" for health probes; off by default |
<!-- markdownlint-enable MD013 -->

Without the two model variables the bot replies with cited excerpts and says
that no model wrote them. In the live server that is the only mode for now: no
deployment sets the two variables until the gates of
[ADR-0006](docs/adr/0006-ai-act-role.md) are passed.

### Optional settings

Each of these has a built-in default. A value outside its range stops the start
with a message that names the variable
([`internal/config`](internal/config/config.go)).

<!-- markdownlint-disable MD013 -->
| Variable | What it sets | Default | Range |
| :--- | :--- | :--- | :--- |
| `DIALLAK_TOP_PASSAGES` | How many documentation passages a reply rests on at most | `3` | `1` to `10` |
| `DIALLAK_MIN_SCORE` | How well a passage must match the question to count as a source | `0.20` | above `0`, at most `1` |
| `DIALLAK_QUESTION_MAX_CHARS` | Longest question the bot accepts, in characters | `500` | `1` to `500` |
| `DIALLAK_EXCERPT_CHARS` | Longest excerpt shown per source, in characters | `400` | `50` to `1000` |
| `DIALLAK_REPLY_MAX_CHARS` | Longest reply, in characters; Discord allows 2000 | `1900` | `500` to `2000` |
| `DIALLAK_MAX_CONCURRENT_ASKS` | How many questions are answered at the same time; one more gets the busy reply | `4` | `1` to `64` |
| `DIALLAK_ASK_TIMEOUT` | Time one question may take in total | `60s` | `1s` to `10m` |
| `DIALLAK_MODEL_TIMEOUT` | Time one request to the model endpoint may take | `45s` | `1s` to `5m`, and below the ask timeout |
| `DIALLAK_SOURCE_LINK_TEMPLATE` | Shape of the link to a source | `https://github.com/{repository}/blob/{revision}/{path}` | an `https://` address that holds each of the three placeholders once, after the host, at most 200 characters |
<!-- markdownlint-enable MD013 -->

The question limit also sets the length the `/ask` command accepts, so a change
needs the registration step below to reach the server. Where each default comes
from is stated at its constant: most are chosen, not measured; the score comes
from the evaluation in
[`internal/eval`](internal/eval/testdata/snatcharr/README.md).

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

### Health checks

When `DIALLAK_HEALTH_ADDR` is configured, the bot starts an HTTP listener for
deployment health and readiness probes ([`internal/health`](internal/health),
[ADR-0007](docs/adr/0007-health-listener.md)). The listener is off by default.

- `GET /healthz` reports process liveness and answers `200 ok` while the server
  runs.
- `GET /readyz` reports readiness. It answers `503 not ready` until the Discord
  gateway connection opens, then `200 ready`, and `503 not ready` again once
  shutdown begins.

Readiness means the gateway opened and the bot is not shutting down; it does
not follow later reconnects.

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
sh .config/lefthook/engine.sh changelog new --type=fixed \
  --title="Refuse a long question"
```

The wrapper runs the engine this repository pins. A `praetorctl` on the path
can be another version, so commands that write into the repository go through
the wrapper.

A release is cut in three steps:

1. On a branch, `sh .config/lefthook/engine.sh release --version=vX.Y.Z
   --date=YYYY-MM-DD` renders the fragments into `CHANGELOG.md`. The branch is
   merged like any other change.
2. The merged commit on `main` is tagged `vX.Y.Z`, and the tag is pushed.
3. The tag starts [`release.yml`](.github/workflows/release.yml). It refuses a
   version that has no section in `CHANGELOG.md`, builds the container image
   `ghcr.io/tekmora/diallak:X.Y.Z`, and publishes the release. Its notes are
   the changelog section, followed by the image by tag and by digest. There is
   no `latest` tag.

The image is built in a reusable workflow of its own
([`release-image.yml`](.github/workflows/release-image.yml)) for `linux/amd64`
and `linux/arm64`. The workflow also signs it with cosign, attests its build
provenance and writes one SBOM per platform; both are attached to the release.

Merging to `main` releases nothing; only a pushed tag does.

### Verifying a release

The first image is 0.1.1. Version 0.1.0 has a tag and a changelog section, but
no release page and no image: its package carried a `latest` tag by mistake and
was removed before it was public.

Each image is signed by the release workflow at its own tag and carries build
provenance. Both commands were tried against 0.1.1. They fail when the identity
names another tag or the main branch, and when another repository is named:

```bash
cosign verify ghcr.io/tekmora/diallak:0.1.1 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity https://github.com/tekmora/diallak/.github/workflows/release-image.yml@refs/tags/v0.1.1

gh attestation verify oci://ghcr.io/tekmora/diallak:0.1.1 --repo tekmora/diallak
```

The package is public, so `cosign verify` needs no login. `gh attestation
verify` asks GitHub for the attestation and needs a GitHub login
(`gh auth login`), but none to the registry.

For another version, replace `0.1.1` in all three places. A deployment pins the
image by digest. From 0.1.2 on, the release page of a version names its digest;
the one of 0.1.1 is in [docs/hosting.md](docs/hosting.md).

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

## Legal

The [imprint](docs/compliance/imprint.md) and the
[privacy notice](docs/compliance/privacy-notice.md) are drafts. They are filled
and published before the public launch; the
[compliance documents](docs/compliance/README.md) list what has to happen
first.

## Licence

Code: [EUPL-1.2](LICENSE). Documentation text, which is this file and everything
under [`docs/`](docs/): [CC BY-SA 4.0](LICENSES/CC-BY-SA-4.0.txt).
[`REUSE.toml`](REUSE.toml) states the licence of every path, and `reuse lint`
checks it.
