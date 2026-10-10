# ADR-0007: Health listener for probes

- **Status**: Proposed
- **Date**: 2026-10-10
- **Authors**: Maintainers

## Context

The bot will run on a platform that restarts a container when its liveness
probe fails and counts it as started only when its readiness probe passes.
Without a probe the platform cannot tell a bot that hangs from one that works.

[ADR-0001](0001-architecture-and-boundaries.md) chose a gateway connection so
that the bot needs no inbound network path. The runtime image holds the bot's
binary and no shell ([Dockerfile](../../Dockerfile)), so a probe cannot run a
command inside the container. A probe therefore needs something to ask.

## Decision

### An optional listener, off by default

`DIALLAK_HEALTH_ADDR` names an address to listen on. When it is not set,
nothing listens and ADR-0001 holds as written. The listener is meant for the
platform's probes only; a deployment does not publish it.

### Two paths

| Path | Answers 200 when |
| :--- | :--- |
| `/healthz` | the process serves requests |
| `/readyz` | the gateway has been opened and shutdown has not begun |

Readiness does not follow a later loss of the gateway connection: the library
reconnects by itself, and a bot that is taken out of service for a reconnect
gains nothing.

### It reveals and records nothing

The listener reads no request body, writes no log line per request and answers
with a fixed word. It gives no version and no configuration
([`internal/health`](../../internal/health)).

### It fails closed

- An address that cannot be bound stops the start.
- If the listener stops while the bot runs, the process ends with that error,
  so a probe is never silently dead.

## Consequences

- **Positive**: A deployment can restart a bot that hangs and can wait for the
  gateway before it counts a start as done. A deployment that sets no address
  runs exactly as before.
- **Negative**: With the address set the bot has one inbound path inside the
  deployment. The listener is a second thing that can fail.

## Alternatives considered

- **A probe that runs a command.** The image has no shell, and a second mode of
  the binary would need a channel to the running process, such as a file it
  keeps writing.
- **A TCP check without HTTP.** It tells that the process listens, not whether
  the gateway is open.
- **No probe.** The platform then restarts the bot only when the process exits.

## Decided in this record

The health listener was accepted as a small unit with a test for a listener
that is down. The choices below were made here: the listener is off unless
configured, the two paths and what they mean, that readiness does not follow
reconnects, and that a listener that stops ends the process.
