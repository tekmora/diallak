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

> **Status: skeleton.** The repository holds its governance baseline and this plan. No bot code yet.

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
