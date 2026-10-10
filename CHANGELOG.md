# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]


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

