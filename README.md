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
> documentation with cited sources, `/areas` lets a member choose interest,
> language and notification roles, and moderators remove a message or time a
> member out through the bot, which keeps a trail of every act. It has been
> tested against a fake Discord only; the live test and the switch from the bot that runs today are still to
> come.

## What it will do

- **Support and tickets:** a member opens a ticket, the bot keeps the thread and
  its state.
- **Bug reports:** a report in Discord becomes a GitHub issue draft with a link
  back to the thread.
- **Votes and wishes:** community polls whose results feed the projects'
  planning.
- **Thanks to supporters:** a cosmetic role and a public credit in plain text.
  The credit is on a page the operator keeps outside the repository, names
  only with the supporter's consent, release notes link it and never name
  anyone ([issue 125](https://github.com/tekmora/diallak/issues/125)).
  Nothing is sold: the perks are a thank-you only, there is no priority
  support, and early builds are announced to everybody.
- **Answers from the docs:** a locally run model answers from the projects'
  documentation. Every answer cites its sources and is labelled as AI-generated.

Of this list only the answers from the docs are built. Besides the list, a
member chooses interest, language and notification roles with `/areas`
([ADR-0011](docs/adr/0011-area-roles-from-member-choice.md)), and a moderator
acts with `Remove message` and `/timeout`
([ADR-0021](docs/adr/0021-moderation-commands-and-trail.md),
[moderator guide](docs/moderator-guide.md)). A member reports a bug with `/bug`:
the bot answers with a link that opens a filled-in new issue of the repository
the server definition names, and the member submits it under their own GitHub
account; the bot calls no GitHub interface for it
([ADR-0001](docs/adr/0001-architecture-and-boundaries.md)). In the support forum the bot can
reply first to a new post that carries the tag "Automated answer", with
excerpts from the documentation only
([ADR-0009](docs/adr/0009-support-forum-first-answer.md),
[ADR-0020](docs/adr/0020-forum-answers-only-opted-in-posts.md)); it is off
until a deployment sets the two forum variables. Issue drafts, wishes and
polls will not be built in this repository: they belong to a shared core the
projects plan to use from every channel, and the bot will be its Discord
adapter. That core has no record here yet.

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
| Gate for model answers | The bot refuses a model reply that does not name the deployment's guardrail policy, and a member can ask for a receipt of the guardrails that ran ([ADR-0013](docs/adr/0013-guardrail-policy-and-receipt.md)); both are built; model mode stays off in the live server until the operator switches it on |
| Discord application | The existing bot application and its token are reused |
| Community questions | GitHub Discussions for searchable answers, the hub for live chat; the bot links them |
| Moderation | The bot filters spam and obvious abuse; people decide unclear cases |
| Compliance | Discord's developer terms and policy, GDPR, and the transparency duty of EU AI Act Art. 50; the provider-or-deployer role assessment is written before launch |
<!-- markdownlint-enable MD013 -->

The target state of the server itself, its roles, channels and permissions, is
data with a validator: [docs/server-definition.md](docs/server-definition.md).
It has not been applied to a server. The commands that read the server,
compare it, plan the changes and apply one plan are built and were never run
against Discord: [docs/server-apply.md](docs/server-apply.md).

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
| `DIALLAK_MODEL_POLICY` | with the model endpoint | Name of the gateway policy every chat reply must name |
| `DIALLAK_MODEL_GUARDRAIL` | no, with the model endpoint | Name of the gateway guardrail the bot masks each question with before the model sees it; off when unset |
| `DIALLAK_MODEL_KEY_FILE` | no | Path of the file that holds the endpoint's credential |
| `DIALLAK_EMBED_BASE_URL` | with the alias | Base URL of an OpenAI-compatible on-premises embedding endpoint |
| `DIALLAK_EMBED_ALIAS` | with the base URL | Name the embedding endpoint routes on, never a model name |
| `DIALLAK_EMBED_KEY_FILE` | no | Path of the file that holds the embedding endpoint's credential |
| `DIALLAK_SUPPORT_CHANNEL` | no | Wording for where a person helps, put into every reply; default "the support channel" |
| `DIALLAK_HEALTH_ADDR` | no | Listen address "host:port" for health probes; off by default |
| `DIALLAK_FORUM_CHANNEL_ID` | with the tag | Id of the support forum whose new opted-in posts get a first reply; the forum answer is off without it |
| `DIALLAK_FORUM_OPT_IN_TAG_ID` | with the forum | Id of the forum's tag "Automated answer"; only a post that carries it from its creation is read |
| `DIALLAK_NEWS_STATE_FILE` | no | Absolute path of the file in which the news function keeps what it has posted; the news function is off without it |
| `DIALLAK_NEWS_GITHUB_TOKEN_FILE` | no, with the state file | Path of the file that holds a read-only GitHub token, sent only to `api.github.com` for the advisory reads; without it those reads go unauthenticated |
| `DIALLAK_DISCUSSIONS_APP_CLIENT_ID` | with the key file and the state file | Client ID of the GitHub App that reads Discussions; the bridge is off without it |
| `DIALLAK_DISCUSSIONS_APP_KEY_FILE` | with the client ID and the state file | Path of the file that holds the app's private key; sent nowhere, it signs the app's short-lived tokens |
| `DIALLAK_TRAIL_RETENTION` | no | How long a line of the moderation trail stays, as a Go duration from `24h` to `8760h`; default 90 days |
<!-- markdownlint-enable MD013 -->

Without the model variables the bot replies with cited excerpts and says
that no model wrote them. In the live server that is the only mode for now: no
deployment sets the variables until the gates of
[ADR-0013](docs/adr/0013-guardrail-policy-and-receipt.md) are passed: every
reply of the model has to name the deployment's guardrail policy, and the
member can ask for a receipt of the guardrails that ran. Both are built; the
operator decides when the live server switches model mode on.

With the model variables set the bot sends the endpoint one small request at start
([ADR-0012](docs/adr/0012-model-state.md)):

- **The endpoint refuses it**, for example with status 401 or 404 for a wrong
  key or an unknown alias: the bot ends with exit code 1 and names the four
  model variables.
- **The reply does not name the policy** of `DIALLAK_MODEL_POLICY` in its
  header `x-litellm-applied-policies`: the bot ends with exit code 1, as for a
  refused key.
- **The endpoint is away**, with a server error, status 408 or 429, or no reply
  within 10 seconds: the bot starts and answers with excerpts. It sends a
  member's question to the endpoint again at most once every 30 seconds until
  one succeeds.

The same check runs on every later reply. A reply that does not name the
policy is discarded, the member gets the excerpts, and the bot sends no
further question to the endpoint until it is restarted
(`TestAsk_Negative_ReplyWithoutThePolicyEndsInExcerpts`). The name is compared
exactly, among at most 64 entries of the header.

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
| `DIALLAK_EMBED_TIMEOUT` | Time one request to the embedding endpoint may take | `10s` | `1s` to `60s` |
| `DIALLAK_EMBED_MIN_SIMILARITY` | Minimum cosine similarity for a passage to match semantically; the default is measured for embeddinggemma | `0.75` | above `0`, at most `1` |
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

It loads the documentation and the server definition, reads the server's roles
and channels and its own user, connects to Discord and answers `/ask`,
`/areas`, `/bug`, `Remove message` and `/timeout` until it receives a termination
signal. With the two forum variables set it also replies to each new post in
that forum that carries the opt-in tag, with excerpts and never with the model.
The forum answer needs the right to read message content, switched on in
Discord's developer portal; without it the start stops. A reply is visible only to the member who asked and can never mention
anyone. A failed read stops the start. Once a day it deletes its own lines of
the moderation trail that are older than `DIALLAK_TRAIL_RETENTION`.

### Health checks

When `DIALLAK_HEALTH_ADDR` is configured, the bot starts an HTTP listener for
deployment health and readiness probes ([`internal/health`](internal/health),
[ADR-0007](docs/adr/0007-health-listener.md)). The listener is off by default.

- `GET /healthz` reports process liveness and answers `200 ok` while the server
  runs.
- `GET /readyz` reports readiness. It answers `503 not ready` until the Discord
  gateway connection opens, then `200 ready`, and `503 not ready` again once
  shutdown begins.
- `GET /modelz` reports the model's state and always answers 200: `model: not
  configured`, `model: ready` or `model: not ready`
  ([ADR-0012](docs/adr/0012-model-state.md)).

Readiness means the gateway opened and the bot is not shutting down; it does
not follow later reconnects and it does not follow the model: a bot whose model
is not ready still answers with excerpts.

### Commands are registered by hand

```bash
diallak -register-commands
```

This sets the server's command list for the application to `/ask`, `/areas`,
`/bug`, `Remove message` and `/timeout` and exits. It
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
   the changelog section, followed by the image by tag and by digest, and the
   credits note linking the page from variable `CREDITS_URL`. There is
   no `latest` tag.

The image is built in a reusable workflow of its own
([`release-image.yml`](.github/workflows/release-image.yml)) for `linux/amd64`
and `linux/arm64`. The workflow also signs it with cosign, attests its build
provenance and writes one SBOM per platform; both are attached to the release.

A release candidate is cut for a test before a version: the merged commit
on `main` is tagged `vX.Y.Z-rc.N`, with no changelog step. The workflow builds
`ghcr.io/tekmora/diallak:X.Y.Z-rc.N` only and publishes a prerelease that never
becomes the latest release. Its notes are the fragments in `changelog.d/` that
are not yet in `CHANGELOG.md`
([`release-candidate-notes.sh`](.github/scripts/release-candidate-notes.sh)),
followed by the image by tag and by digest, and the credits note.

Merging to `main` releases nothing; only a pushed tag does. Before its first
release, a deployment sets repository variable `CREDITS_URL` (the https URL of
the live credits page) and repository secret `SUPPORTER_NAMES` (the supporter
names to refuse in release notes, one name per line; blank lines and lines
starting with `#` are skipped, so a list with no supporters yet is the single
line `# none`). The release workflow stops if either is unset.

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

The two SBOM files on a release page carry build provenance of their own from
0.2.0 on (step `Attest the SBOM files` in
[`release-image.yml`](.github/workflows/release-image.yml)). For a downloaded
file:

```bash
gh attestation verify diallak-X.Y.Z-linux-amd64.spdx.json --repo tekmora/diallak
```

The command looks the file up by its SHA-256, so a changed file is a file
without an attestation. Tried against 0.2.0 on 2026-10-11:

- Both files are accepted, also when the command names the workflow and the
  tag (`--signer-workflow tekmora/diallak/.github/workflows/release-image.yml
  --source-ref refs/tags/v0.2.0`).
- A copy with one byte added is refused with exit code 1 and `HTTP 404`.
- Either file is refused the same way when another repository is named.
- The files of 0.1.2 are refused the same way.

The attestation says that the release workflow of this repository wrote the
file in the run of that version's tag. It does not name the image digest.

The SBOM files of 0.1.1 and 0.1.2 carry no signature and no attestation. Their
only check is the SHA-256 the release page shows.

## Develop

```bash
make verify-all   # context check, audit, lint, build and tests
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

Dependency updates come from Renovate as pull requests
([`renovate.json`](renovate.json)): Go modules, the actions in the workflows
and the images in the `Dockerfile`. A release is proposed three days after it
was published, every action stays pinned to a commit, and nothing merges on
its own. The generated devcontainer files are left out, because the audit
refuses an edit to them.

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
