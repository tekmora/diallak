# Hosting

What a deployment needs to know to run the bot. Deployment manifests live
outside this repository ([ADR-0001](adr/0001-architecture-and-boundaries.md));
this page is the contract they rest on. Each statement names where it can be
checked. A value that was not measured says so.

## In short

- One container, one replica, no database. No volume to write to, unless the
  news function is on: then one small volume holds its state file.
- Outbound: Discord, the model endpoint when one is configured, and GitHub
  plus the definition's feeds when the news function is on. Inbound: nothing,
  or the platform's probes when the health listener is switched on.
- Configuration comes from environment variables; the two credentials come from
  mounted files.
- The documentation the bot answers from is a directory the deployment fills
  before the bot starts.

## Image

<!-- markdownlint-disable MD013 -->
| Property | Value | Check |
| :--- | :--- | :--- |
| Name | `ghcr.io/tekmora/diallak:X.Y.Z`, to be pinned by digest | [`release-image.yml`](../.github/workflows/release-image.yml); there is no `latest` tag |
| Digest | On the release page of the version, from 0.1.2 on. 0.1.1: `sha256:65aacd457c9df4cd331c7ad356e9410ee241e188b36feb9d76b2b26eb2fa4e01` (the index of both platforms) | [`release.yml`](../.github/workflows/release.yml) writes it; `crane digest ghcr.io/tekmora/diallak:X.Y.Z` returns the same value |
| Platform | `linux/amd64` and `linux/arm64`, one index | `platforms` in the same workflow; the Dockerfile compiles for the target platform without emulation |
| Size | 22,053,900 bytes unpacked, `linux/amd64`, for the released image 0.1.1. The files in an image built from the tree of 2026-10-17 add up to 14,569,664 bytes (`linux/amd64`) and 13,639,872 bytes (`linux/arm64`) | The released image 0.1.1; the `image` jobs in [`ci.yml`](../.github/workflows/ci.yml) build the image on each platform for every pull request, and [`image_smoke.py`](../tools/ci/image_smoke.py) fails above 16,030,000 bytes |
| User | `65532:65532`, not root | [`Dockerfile`](../Dockerfile); the `image` jobs fail on any other user |
| Contents | The static binary on a distroless base without a shell | [`Dockerfile`](../Dockerfile) |
| Signature, provenance, SBOM | Signed by the release workflow at its tag, with build provenance; one SBOM per platform on the release page | [README](../README.md), "Verifying a release": tried against 0.1.1 |
| Visibility | Public: a pull needs no login | `crane digest ghcr.io/tekmora/diallak:0.1.1` without credentials returned the digest above on 2026-10-11 |
<!-- markdownlint-enable MD013 -->

A deployment may set repository variable `CREDITS_URL` (the address of the
credits page) and repository secret `SUPPORTER_NAMES` (the supporter names to
refuse in release notes, one name per line) for the release workflow
([README](../README.md), "A release is cut in three steps").

The root filesystem can be read-only and no temporary directory is needed. The
only file the bot writes is the news state file, when the news function is on;
it lies on a writable volume that user `65532` owns, and the bot replaces it
through a temporary file in the same directory
([data inventory](compliance/data-inventory.md), "What the bot stores").

## Configuration

The [README](../README.md) lists every variable with its default and range:
"Run" for the required ones, "Optional settings" for the rest. A variable that
is missing or out of range stops the start with a message that names it.

- **Credentials are files.** `DIALLAK_TOKEN_FILE` and, when the model endpoint
  needs one, `DIALLAK_MODEL_KEY_FILE` name files of at most 4096 bytes that the
  user `65532` can read ([`internal/config`](../internal/config/config.go)).
  They are read once at start.
- **The guild id is configuration.** It never appears in this repository.
- **Model mode is off unless the model variables are set.**
  `DIALLAK_MODEL_BASE_URL`, `DIALLAK_MODEL_ALIAS` and `DIALLAK_MODEL_POLICY`
  (for example `example-policy`) configure the endpoint, the routing alias and
  the gateway policy. In the live server they stay unset until the gates of
  [ADR-0013](adr/0013-guardrail-policy-and-receipt.md) are passed
  ([compliance](compliance/README.md), "Gates for model-written answers").
- **The bot's own masking is off unless `DIALLAK_MODEL_GUARDRAIL` is set.** It
  names a guardrail of the gateway, for example `presidio-pii`. The bot then
  calls the gateway's route `/guardrails/apply_guardrail` at the root of the
  base URL, without `/v1`, so the deployment must let that route through as
  well as the chat route. At start it sends a fixed sentence with an invented
  address there: an address that comes back unmasked, or a refusal, ends the
  start with exit code 1.
- **Embedding mode is off unless the embedding variables are set.**
  `DIALLAK_EMBED_BASE_URL` and `DIALLAK_EMBED_ALIAS` configure the endpoint
  and the routing alias ([ADR-0022](adr/0022-embeddings-for-paraphrased-questions.md)).
  The start verifies that the base URL points to an on-premises destination
  (loopback or RFC 1918 / fc00::/7 private IP literal, `localhost`, a single-label
  host name, or a host ending in `.svc`, `.cluster.local`, `.internal`,
  `.home.arpa`, or `.lan`) and refuses any other host. When embeddings are not
  configured or when the embedding call fails, retrieval is keyword only and
  the reply carries a notice saying so. Support forum posts are always searched
  by keyword only and never reach the embedding endpoint. At start the bot
  computes embedding vectors for all documentation chunks in memory; if the
  endpoint fails at start, the start stops with an exit code of 1.
- **The news function is off unless `DIALLAK_NEWS_STATE_FILE` is set**
  ([ADR-0019](adr/0019-news-posts.md), issue #180). It names an absolute path
  on the writable volume. A damaged state file stops the start, because an
  empty state would post every release again. The bot finds the announcement
  channel, the release role and the feed channels by their names in the server
  definition before every read, so a renamed channel needs no restart.
- **The GitHub token is optional and read-only.**
  `DIALLAK_NEWS_GITHUB_TOKEN_FILE` names a file of at most 4096 bytes with a
  fine-grained token that can only read public repositories. The bot sends it
  only to `api.github.com`, for the advisory reads, and never logs it. Without
  it those reads are unauthenticated and may be refused; the bot then waits for
  the reset the reply names.
- **The Discussions bridge is off unless its GitHub App is set**
  ([ADR-0004](adr/0004-community-questions.md), issue #99).
  `DIALLAK_DISCUSSIONS_APP_CLIENT_ID` and `DIALLAK_DISCUSSIONS_APP_KEY_FILE`
  are set together and need the news state file; one without the other stops
  the start. The operator creates the app with the repository permission
  Discussions read-only and no other, installs it on the selected public
  repositories whose discussions the hub should see, and mounts its private
  key as a file of at most 16384 bytes. The bot never sends or logs the key: it
  signs a token of nine minutes with it, exchanges that for an installation
  token of one hour limited to discussions and metadata, and sends both only
  to `api.github.com`. Every 15 minutes it posts each new Q&A or Ideas
  discussion into the channel `news.discussions_channel` of the server
  definition, the lounge today, with no mention and no crosspost. The first
  read after the state file is new posts nothing.

## Cluster deployment contract

A cluster deployment supplies the bot's settings through environment variables,
mounted secret files, and volumes
([ADR-0001](adr/0001-architecture-and-boundaries.md)). The table below lists
every variable the application reads, how it reaches the pod, and when it is
needed ([`internal/config`](../internal/config/config.go)). A Go test keeps
this contract synchronized with the codebase
([`deploy_contract_test.go`](../cmd/diallak/deploy_contract_test.go)).

<!-- markdownlint-disable MD013 -->
| Variable | Reaches the pod as | Needed when |
| :--- | :--- | :--- |
| `DIALLAK_ASK_TIMEOUT` | environment | optional: tuning question timeout |
| `DIALLAK_CORPUS_DIR` | read-only volume | always |
| `DIALLAK_DISCUSSIONS_APP_CLIENT_ID` | environment | discussions bridge |
| `DIALLAK_DISCUSSIONS_APP_KEY_FILE` | secret file | discussions bridge |
| `DIALLAK_EMBED_ALIAS` | environment | embedding mode |
| `DIALLAK_EMBED_BASE_URL` | environment | embedding mode |
| `DIALLAK_EMBED_KEY_FILE` | secret file | embedding mode with authentication |
| `DIALLAK_EMBED_MIN_SIMILARITY` | environment | optional: tuning embedding similarity threshold |
| `DIALLAK_EMBED_TIMEOUT` | environment | optional: tuning embedding timeout |
| `DIALLAK_EXCERPT_CHARS` | environment | optional: tuning excerpt length |
| `DIALLAK_FORUM_CHANNEL_ID` | environment | support forum answer |
| `DIALLAK_FORUM_OPT_IN_TAG_ID` | environment | support forum answer |
| `DIALLAK_GUILD_ID` | environment | always |
| `DIALLAK_HEALTH_ADDR` | environment | health probes |
| `DIALLAK_MAX_CONCURRENT_ASKS` | environment | optional: tuning concurrent asks limit |
| `DIALLAK_MIN_SCORE` | environment | optional: tuning minimum score threshold |
| `DIALLAK_MODEL_ALIAS` | environment | model mode |
| `DIALLAK_MODEL_BASE_URL` | environment | model mode |
| `DIALLAK_MODEL_GUARDRAIL` | environment | model mode with gateway guardrail |
| `DIALLAK_MODEL_KEY_FILE` | secret file | model mode with endpoint authentication |
| `DIALLAK_MODEL_POLICY` | environment | model mode |
| `DIALLAK_MODEL_TIMEOUT` | environment | optional: tuning model timeout |
| `DIALLAK_NEWS_GITHUB_TOKEN_FILE` | secret file | news function with GitHub authentication |
| `DIALLAK_NEWS_STATE_FILE` | writable volume | news function |
| `DIALLAK_QUESTION_MAX_CHARS` | environment | optional: tuning question length limit |
| `DIALLAK_REPLY_MAX_CHARS` | environment | optional: tuning reply length limit |
| `DIALLAK_SOURCE_LINK_TEMPLATE` | environment | optional: custom source link template |
| `DIALLAK_SUPPORTER_GRACE` | environment | supporter function |
| `DIALLAK_SUPPORTER_ROLES` | environment | supporter function |
| `DIALLAK_SUPPORT_CHANNEL` | environment | optional: custom support channel wording |
| `DIALLAK_TOKEN_FILE` | secret file | always |
| `DIALLAK_TOP_PASSAGES` | environment | optional: tuning retrieved passages limit |
| `DIALLAK_TRAIL_RETENTION` | environment | optional: tuning moderation trail retention |
<!-- markdownlint-enable MD013 -->

- **User `65532:65532`.** The container runs as unprivileged user `65532` and
  group `65532` ([`Dockerfile`](../Dockerfile), and "Image" above).
- **Read-only root filesystem.** The container needs no writable root
  filesystem; only the news state file requires a writable volume when enabled
  ("Image" above, and
  [`compliance/data-inventory.md`](compliance/data-inventory.md)).
- **One replica.** One token serves one gateway session
  ([ADR-0001](adr/0001-architecture-and-boundaries.md)). The deployment
  replaces the instance rather than rolling it ("Start, stop and replace"
  below).
- **Image pinned by digest.** The deployment pins the image by content digest
  from the release page rather than a mutable tag ("Image" above, and
  [README](../README.md)).
- **Health probes.** When `DIALLAK_HEALTH_ADDR` is set, probes target
  `/healthz` for liveness and `/readyz` for readiness ("Health checks" below,
  and [ADR-0007](adr/0007-health-listener.md)).

## Start, stop and replace

- **Start.** The entry point is the bot itself. In model mode it first sends
  the endpoint one request; a refusal such as status 401 or 404 ends the start
  with exit code 1, an endpoint that is away does not
  ([ADR-0012](adr/0012-model-state.md)). It prints one line, `ready: <n>
  sources, <n> chunks, model <state>`, on standard output and then connects to
  Discord. The state is `not configured`, `ready` or `not ready`. Logs are
  JSON lines on standard error and hold no question, no answer and no member
  id ([data inventory](compliance/data-inventory.md), "What the bot logs").
  The deployment keeps them for 30 days, the period the
  [privacy notice](compliance/privacy-notice.md) names.
- **Exit codes.** 0 after a clean stop, 1 with the reason on standard error
  ([`cmd/diallak`](../cmd/diallak/main.go), `report`).
- **Stop.** On `SIGTERM` the bot takes no new question, waits up to 15 seconds
  for the ones in flight and closes the gateway
  ([`internal/discord`](../internal/discord/disgo.go), `shutdownClient`). A
  grace period of 30 seconds leaves room for that.
- **One replica.** One token serves one gateway session
  ([ADR-0001](adr/0001-architecture-and-boundaries.md)). A new instance starts
  only after the old one has stopped: the deployment replaces, it does not roll.
- **Upgrade.** A new release starts on the configuration of the release before
  it, unchanged. It stops only where a breaking change says so in its
  `Migration:` note: since v0.2.0 the host of `DIALLAK_SOURCE_LINK_TEMPLATE` is
  written literally, and since v0.3.0 a model needs `DIALLAK_MODEL_POLICY`.
  Configurations of v0.1.2 and v0.4.2 under
  [`internal/config/testdata/upgrade`](../internal/config/testdata/upgrade)
  hold this in a test ([`upgrade_test.go`](../internal/config/upgrade_test.go)).
  Nothing else carries over: the bot stores no state but the news state file
  (see "Database").
- **Commands are registered by a separate run.** `diallak -register-commands`
  with the same environment sets the server's command list and exits. Starting
  the bot never does. It is the operator's act at the cutover, and it is needed
  again after a change of `DIALLAK_QUESTION_MAX_CHARS` and after an update that
  adds a command, such as `/areas` or the moderation commands
  ([README](../README.md),
  "Commands are registered by hand").

## Two shapes of deployment

The contract on this page holds for both. What differs is who restarts the bot
and who probes it.

<!-- markdownlint-disable MD013 -->
| | Container platform with probes | Single server with a container runtime |
| :--- | :--- | :--- |
| Restart | The platform restarts a container that exits or fails its liveness probe | The runtime's restart policy restarts a container that exits |
| Probes | `/healthz` and `/readyz`, see "Health checks" | None needed. The image holds no shell and no HTTP client, so a probe has to come from outside the container |
| Credentials | Files projected into the container, read-only | Files on the server that only user `65532` can read, mounted read-only |
| Documentation directory | A step that runs before the bot fills a volume | A script or a one-off container fills a directory before the bot starts |
| Logs | Collected by the platform | Kept by the runtime; how long is set there |
<!-- markdownlint-enable MD013 -->

On both, the bot ends with exit code 1 when it cannot start or when its run
ends with an error ([`cmd/diallak`](../cmd/diallak/main.go), `report`), so a
restart policy brings it back without a probe.

## Health checks

The listener is off unless `DIALLAK_HEALTH_ADDR` names an address such as
`:8081` ([ADR-0007](adr/0007-health-listener.md)). It is meant for the
platform's probes; a deployment does not publish the port.

<!-- markdownlint-disable MD013 -->
| Probe | Request | Passes when |
| :--- | :--- | :--- |
| Liveness | `GET /healthz` | the process serves requests |
| Readiness | `GET /readyz` | the gateway has been opened and shutdown has not begun |
| None: information only | `GET /modelz` | always. The body is `model: not configured`, `model: ready` or `model: not ready` |
<!-- markdownlint-enable MD013 -->

- **A stopped bot fails its probe.** The listener closes with the bot, so a
  probe gets no connection (`TestServer_Negative_ListenerDownAfterContextEnded`
  in [`internal/health`](../internal/health/health_test.go)).
- **Readiness does not follow reconnects.** A later loss of the gateway
  connection leaves it at ready; the library reconnects by itself.
- **The address is bound first and answered after the documentation is loaded.**
  A bind failure stops the start ([`cmd/diallak`](../cmd/diallak/main.go),
  `executeBotRun`).
- **The listener says little about the bot.** It reads no request body, writes
  no log line per request and answers with a fixed word. `/modelz` answers with
  one of three fixed lines, which tells whether a model is configured; it gives
  no alias and no address.
- **The model does not move readiness.** A bot whose model is not ready
  answers with excerpts and stays in service
  (`TestHandler_Positive_IndependenceOfReadyzAndModelz` in
  [`internal/health`](../internal/health/health_test.go)).

## Network

<!-- markdownlint-disable MD013 -->
| Direction | Destination | When | Check |
| :--- | :--- | :--- | :--- |
| Out | `discord.com`, port 443 | Always: the REST API | `rest/rest_endpoints.go` of the pinned library |
| Out | The gateway host names Discord returns, port 443 | Always: the gateway connection | The address is not in the code: the library asks the platform for it and the gateway later sends a resume address (`bot/config.go`, `gateway/gateway_config.go`) |
| Out | The host of `DIALLAK_MODEL_BASE_URL` | Model mode only | [`internal/model`](../internal/model/model.go); the client follows no redirect |
| Out | The host of `DIALLAK_EMBED_BASE_URL` | Hybrid retrieval mode only | [`internal/embed`](../internal/embed/embed.go); the client follows no redirect |
| Out | DNS | Always | |
| Out | `github.com`, port 443 | News on: the releases feed of each own repository, every 10 minutes | [`internal/news`](../internal/news/advisory.go), `ReleasesAddress`; redirects only to https, at most a fixed number |
| Out | `api.github.com`, port 443 | News on: the published advisories of each own repository, every 15 minutes | `AdvisoriesAddress` in the same file; the token goes to this host only |
| Out | `api.github.com`, port 443 | Bridge on: `GET /app/installations` once per run, `POST /app/installations/{id}/access_tokens` about once an hour, `GET /installation/repositories` and one `POST /graphql` per repository every 15 minutes | [`internal/ghapp`](../internal/ghapp/ghapp.go) refuses redirects and every other host |
| Out | The hosts of the definition's feeds, port 443 | News on and feeds listed: each feed once an hour | `news.feeds` in [`server/definition.json`](../server/definition.json); none listed today |
| In | The health listener | Only when `DIALLAK_HEALTH_ADDR` is set | See "Health checks" |
<!-- markdownlint-enable MD013 -->

The bot contacts nothing else. It has no telemetry and fetches no documentation
itself. Every news read obeys the back-off the reply asks for.

## Resources

Nothing here is measured on a running bot: it has never run against Discord.

<!-- markdownlint-disable MD013 -->
| Fact | Value | How it was obtained |
| :--- | :--- | :--- |
| Binary | 12,202,144 bytes (`linux/amd64`), 11,272,352 bytes (`linux/arm64`) | Image builds of 2026-10-17, read by [`image_smoke.py`](../tools/ci/image_smoke.py), which fails above 13,420,000 bytes on every pull request |
| Memory of building the index and answering the evaluation questions | 16.7 MiB peak for the whole test process; the work itself holds about 0.3 MB and allocates about 3.4 MB (4.2 MB with `-race`) since the chunker parses Markdown with goldmark, and a test fails above 700,000 bytes held or 5,000,000 bytes allocated | The evaluation test process over the fixture corpus (18 files, 44,616 bytes), 2026-10-10; the heap figures and limits from `TestEvalMemory_Positive_UnderTheLimits` in `internal/eval/eval_memory_test.go`, 2026-10-18 |
<!-- markdownlint-enable MD013 -->

Starting values, chosen, not measured: a memory request of 64 MiB with a limit
of 256 MiB, and a CPU request of 50 millicores without a limit. The live test
replaces them with measured values. Memory grows with the size of the
documentation, because the index is held in memory
([ADR-0003](adr/0003-answer-pipeline.md)).

## Database

None. The bot stores nothing but the news state file when the news function is
on ([data inventory](compliance/data-inventory.md)).
Features that need stored state come with their own record
([ADR-0001](adr/0001-architecture-and-boundaries.md), "Open decisions").

## The documentation directory

`DIALLAK_CORPUS_DIR` names a directory with one checkout per source and a
`manifest.json` that names each source and its revision ([README](../README.md),
"The documentation directory").

- **The deployment fills it before the bot starts.** A step that runs first
  fetches each source at a pinned revision into a volume and writes the manifest
  with the same revisions. The bot mounts that volume read-only.
- **The revision is pinned, never a branch.** It appears in every citation, so a
  member can open exactly the text a reply was written from.
- **A refresh is a new revision and a restart.** The bot reads the directory
  once at start and has no reload ([`internal/app`](../internal/app/app.go),
  `Build`). How often to refresh is the operator's choice; a new documentation
  release is a natural moment.
- **Symbolic links stop the start.** The bot refuses a listed document that is
  not a regular file and any path that leaves the directory
  ([`internal/corpus`](../internal/corpus/corpus.go)). The fetch step must write
  plain files.

## Not verified yet

- The gateway connection and the registration call have never run: both need
  Discord and are the operator's acts.
- The arm64 image runs in CI only on an arm64 runner without a network and
  without configuration, where it starts and refuses to run for lack of
  configuration; it has not served a request.
- The SBOM files have not been inspected beyond their presence.
- Memory and CPU of a running bot have not been measured.
