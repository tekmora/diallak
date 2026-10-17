# Hosting

What a deployment needs to know to run the bot. Deployment manifests live
outside this repository ([ADR-0001](adr/0001-architecture-and-boundaries.md));
this page is the contract they rest on. Each statement names where it can be
checked. A value that was not measured says so.

## In short

- One container, one replica, no database, no volume to write to.
- Outbound: Discord, and the model endpoint when one is configured. Inbound:
  nothing, or the platform's probes when the health listener is switched on.
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
| Size | 22,053,900 bytes unpacked, `linux/amd64`, for the released image 0.1.1. The files in an image built from the tree of 2026-10-13 add up to 13,459,649 bytes (`linux/amd64`) and 12,656,833 bytes (`linux/arm64`) | The released image 0.1.1; the `image` jobs in [`ci.yml`](../.github/workflows/ci.yml) build the image on each platform for every pull request, and [`image_smoke.py`](../tools/ci/image_smoke.py) fails above 14,800,000 bytes |
| User | `65532:65532`, not root | [`Dockerfile`](../Dockerfile); the `image` jobs fail on any other user |
| Contents | The static binary on a distroless base without a shell | [`Dockerfile`](../Dockerfile) |
| Signature, provenance, SBOM | Signed by the release workflow at its tag, with build provenance; one SBOM per platform on the release page | [README](../README.md), "Verifying a release": tried against 0.1.1 |
| Visibility | Public: a pull needs no login | `crane digest ghcr.io/tekmora/diallak:0.1.1` without credentials returned the digest above on 2026-10-11 |
<!-- markdownlint-enable MD013 -->

The bot writes no file, so the root filesystem can be read-only and no temporary
directory is needed ([data inventory](compliance/data-inventory.md), "What the
bot stores").

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
  Nothing else carries over: the bot stores no state (see "Database").
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
| Out | DNS | Always | |
| In | The health listener | Only when `DIALLAK_HEALTH_ADDR` is set | See "Health checks" |
<!-- markdownlint-enable MD013 -->

The bot contacts nothing else. It has no telemetry and fetches no documentation
itself.

## Resources

Nothing here is measured on a running bot: it has never run against Discord.

<!-- markdownlint-disable MD013 -->
| Fact | Value | How it was obtained |
| :--- | :--- | :--- |
| Binary | 11,092,128 bytes (`linux/amd64`), 10,289,312 bytes (`linux/arm64`) | Image builds of 2026-10-13, read by [`image_smoke.py`](../tools/ci/image_smoke.py), which fails above 12,200,000 bytes on every pull request |
| Memory of building the index and answering the evaluation questions | 16.7 MiB peak for the whole test process; the work itself holds about 0.33 MB and allocates about 2 MB, and a test fails above 700,000 bytes held or 4,000,000 bytes allocated | The evaluation test process over the fixture corpus (18 files, 44,616 bytes), 2026-10-10; the heap figures and limits from `TestEvalMemory_Positive_UnderTheLimits` in `internal/eval/eval_memory_test.go`, 2026-10-13 |
<!-- markdownlint-enable MD013 -->

Starting values, chosen, not measured: a memory request of 64 MiB with a limit
of 256 MiB, and a CPU request of 50 millicores without a limit. The live test
replaces them with measured values. Memory grows with the size of the
documentation, because the index is held in memory
([ADR-0003](adr/0003-answer-pipeline.md)).

## Database

None. The bot stores nothing ([data inventory](compliance/data-inventory.md)).
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
