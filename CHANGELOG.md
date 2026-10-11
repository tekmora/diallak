# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]


## [0.3.1] - 2026-10-11

### Fixed

- A permission bit the table does not name on a role no longer stops the server plan: the change clears it and the plan line names it


## [0.3.0] - 2026-10-11

### Changed

- README: supporter perks, the gate for model answers and where drafts, wishes and polls will live
- **BREAKING**: Model mode needs the name of the gateway policy and refuses a reply without it

### Fixed

- A fault in a call to Discord no longer writes reply text or the reply address to the log


## [0.2.1] - 2026-10-11

### Changed

- Say how the provenance of the SBOM files was tried against 0.2.0

### Fixed

- Server read no longer stops on a permission bit Discord's reference does not list; compare reports it and a plan refuses to write over it


## [0.2.0] - 2026-10-11

### Security

- Release: the SBOM files on the release page get build provenance, so a downloaded file can be checked with gh attestation verify

### Added

- Start probe of the model endpoint and a model state on the health listener
- Read a server into a snapshot; plan and compare commands that change nothing
- Propose the record for the first answer in the support forum (#39)
- Add the server definition as data with a validator
- Apply command: make planned server changes gated by the plan digest
- Server definition: observed platform state, with Membership Screening as the first entry
- Decision record 0011: the bot gives area roles from the member's own choice
- Apply path, offline half: compare and plan against a server snapshot
- Dependency updates: Renovate proposes them as pull requests, three days after a release, with actions pinned to a commit

### Changed

- Split the check of the source link template into smaller functions
- Accept the record for the support forum answer and add an opt-out tag (#39)
- Decision record 0013: a guardrail policy on every model call and a receipt for the member replace the gates for model mode
- Say that a language room opens at a threshold and needs a reader
- **BREAKING**: Server definition: the key onboarding becomes choices, done by the bot
- Apply the decided opening measure, contributor ladder and supporter roles
- Server definition: the bot holds three rights, bounded by role order
- Move the private path for requests and appeals to a ticket channel (#37)

### Removed

- Editor and agent client files outside the set in use

### Fixed

- **BREAKING**: Refuse a placeholder before the path of the source link template (#45)


## [0.1.2] - 2026-10-11

### Added

- Name the image digest on the release page

### Changed

- Describe how to verify a release and name the first image
- Update BuildKit in the release workflow to v0.34.0
- Say what each release verification command needs


## [0.1.1] - 2026-10-10

### Added

- Add the record of processing and a separate notice of the right to object
- Draft the imprint with one identity block shared with the privacy notice

### Fixed

- Stop the release from pushing a latest image tag

## [0.1.0] - 2026-10-10

### Added

- Release the container image for linux/arm64 as well, with one SBOM per platform
- Add the REUSE licence check to CI and to the commit hooks
- Add an optional health listener for liveness and readiness probes
- Add the first slice: answer the ask command from project documentation with cited sources
- State that replies are automated in the command description and in the last line of every reply
- Make the answer limits, timeouts and the source link shape configuration values
- Draft the privacy notice and the server rules
- Add the changelog fragment lane and cut a release from a version tag

### Changed

- Apply the operator's decisions to the privacy notice and server rules drafts
- Compile the container image for its target platform, so an arm64 image needs no emulation
- Accept decision records 0001 to 0006 and license the documentation text under CC BY-SA 4.0
- Settle the open points of the privacy notice and server rules drafts and record where the code of conduct lives
- Rename the project to diallak: module path, binary, image and the DIALLAK_ environment prefix
- Stop the build, governance and security checks on a draft pull request in their first step

### Fixed

- Build the release image with the Go toolchain that go.mod names
- Read the application id from the platform instead of deriving it from the token

