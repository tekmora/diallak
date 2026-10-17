# ADR-0023: Contributor agreement beside the EUPL

- **Status**: Accepted
- **Date**: 2026-10-17
- **Authors**: Maintainers

This record is an engineering record, not legal advice.

## Context

The repository adopted the European Union Public Licence (EUPL v1.2) on
2026-10-10 ([issue 123](https://github.com/tekmora/diallak/issues/123)).
Three root files define the legal and contribution baseline of the repository:

- [`LICENSE`](../../LICENSE) contains the authoritative text of EUPL v1.2.
- [`REUSE.toml`](../../REUSE.toml) declares `EUPL-1.2` as the repository's
  default licence for software and `CC-BY-SA-4.0` for documentation
  ([`README.md`](../../README.md) and [`docs/**`](../)).
- [`CONTRIBUTING.md`](../../CONTRIBUTING.md) defines contribution standards,
  testing discipline, and commit sign-off requirements.

### Why EUPL alone is not enough

Under EUPL v1.2, Article 2 ("Scope of the rights granted by the Licence")
grants licensees a worldwide, royalty-free, non-exclusive, sublicensable
licence to use, reproduce, modify, and distribute the work. However,
Article 5 ("Obligations of the Licensee", Copyleft clause) restricts
distribution:

> If the Licensee distributes or communicates copies of the Original Works
> or Derivative Works, this Distribution or Communication will be done under
> the terms of this Licence or of a later version of this Licence [...]
> The Licensee (becoming Licensor) cannot offer or impose any additional
> terms or conditions on the Work or Derivative Work that alter or restrict
> the terms of the Licence.

Article 6 ("Chain of Authorship") further provides that contributors grant
licences to their contributions to the work "under the terms of this Licence".

Because contributors retain copyright in their submissions and grant rights
strictly under the copyleft terms of the EUPL, an "inbound = outbound" rule
under the EUPL alone gives the project maintainers no right to license
others' contributions under alternative, proprietary, or commercial terms.

### Why the Developer Certificate of Origin alone is not enough

Commits in this repository carry a `Signed-off-by:` line referencing the
Developer Certificate of Origin 1.1 (<https://developercertificate.org/>).
The DCO certifies that a contributor created the patch or has the right to
submit it under the open source licence indicated in the file. The DCO
certifies origin and chain of submission, but it grants no copyright or
patent licence to the project and confers no relicensing rights.

## Decision

The project adopts a formal contributor agreement to accompany contributions
beside the EUPL.

### Harmony HA-CLA-I v1.0 with Outbound Option Five

The contributor agreement is the Harmony Individual Contributor License
Agreement, version 1.0 (2011-07-04), unchanged, with outbound licence
Option Five (source: operator decision of 2026-10-11, ledger id
`contributor-agreement-text`).

### Plain summary above unchanged text

A five-line plain summary stands above the unchanged agreement text to
explain its core terms clearly. Nothing in the publisher's formal agreement
text is rewritten or modified (source: operator decision of 2026-10-11,
ledger id `contributor-agreement-text`).

### Scope of the grant and the promise to contributors

As established by the publisher's document:

- The contributor retains ownership of the copyright in their contribution
  (Section 2.1(a)).
- The contributor grants the project a perpetual, worldwide, non-exclusive,
  transferable, royalty-free, irrevocable licence under the copyright covering
  the contribution, with the right to sublicense through multiple tiers
  (Section 2.1(b)).
- Outbound Option Five specifies: "Based on the grant of rights in Sections
  2.1 and 2.2, if We include Your Contribution in a Material, We may license
  the Contribution under any license, including copyleft, permissive,
  commercial, or proprietary licenses."
- Option Five conditions this right upon a reciprocal commitment: "As a
  condition on the exercise of this right, We agree to also license the
  Contribution under the terms of the license or licenses which We are using
  for the Material on the Submission Date."

This condition guarantees that every contribution remains available under the
licence in effect on the date of submission (here EUPL v1.2). That condition
is the promise to contributors; no separate promise clause is written
(source: Harmony HA-CLA-I v1.0 Section 2.3 Option Five; operator decision
of 2026-10-11, ledger id `contributor-agreement-text`).

### No corporate agreement at this time

No corporate (entity) contributor agreement (HA-CLA-E) is adopted now. The
choice will be revisited when the first corporate entity asks to contribute
(source: operator decision of 2026-10-11, ledger id
`contributor-agreement-text`).

### Commit sign-off remains

The commit sign-off (`Signed-off-by:`) on every commit stays in place. It
serves as a separate certification of origin under DCO 1.1, distinct from
the contributor licence agreement (source: operator decision of 2026-10-11,
ledger id `contributor-agreement-text`).

### Recording by the fleet gate engine

Acceptance of the contributor agreement is recorded by the gate engine the
repository already runs, `praetorctl` (see the [`Makefile`](../../Makefile)),
as its plan item G48; not by a hosted CLA service and not by the archived
GitHub Action for CLA recording (source: operator decision of 2026-10-11,
ledger id `contributor-agreement-text`). The engine is named praetor, after
the Roman magistrate who judged disputes (pronounced PREE-tor).

## Consequences

- **Positive**: The project secures the right to sublicense and relicense
  contributions (commercial distributions included) without needing to track
  down past contributors for relicensing votes.
- **Positive**: Contributors retain copyright in their submissions and receive
  an enforceable guarantee that their work remains licensed under the open
  source licence in use on the submission date (EUPL v1.2).
- **Positive**: No contributor data, tokens, or personal records are entrusted
  to external hosted third-party CLA services.
- **Positive**: Automated gating runs locally and deterministically via the
  fleet gate engine once G48 is deployed.
- **Negative**: Introduces contribution overhead: outside contributors must
  complete the agreement acceptance in addition to commit sign-offs.
- **Negative (record's own derivation)**: The repository adopts agreement
  recording once praetor plan item G48 ships. Until G48 ships, no contribution
  from outside the maintainers is merged without recorded acceptance.
- **Negative**: Corporate contributions cannot be accepted under an entity
  framework until a corporate agreement is evaluated and adopted.

## Alternatives considered

- **Apache ICLA (Individual Contributor License Agreement)**: Rejected. It
  carries Apache-specific organizational terminology and patent retaliation
  clauses tailored to the Apache ecosystem rather than a lightweight
  companion to the EUPL.
- **FSFE FLA (Fiduciary Licence Agreement) or Copyright Assignment**:
  Rejected. Copyright assignment requires contributors to assign all copyright
  ownership to the project entity. Harmony preserves contributor copyright
  ownership while granting a broad non-exclusive licence.
- **Developer Certificate of Origin (DCO) only**: Rejected. DCO 1.1 certifies
  origin and authority under the existing project licence, but it grants no
  relicensing rights and cannot support outbound licensing flexibility.
- **Hosted CLA service (e.g. CLA Assistant SaaS)**: Rejected. Hosted SaaS
  platforms store contributor records on third-party servers, create external
  network dependencies in CI/CD, and do not fit private, local-first gating.
- **Archived CLA recording action**: Rejected. The community CLA recording
  action (`contributor-assistant/github-action`) is archived; its last
  release is of 2024-09-26 (read 2026-10-11). Unmaintained workflow actions
  present supply-chain and reliability risks.

## Open points

- The agreement file and the pointer in
  [`CONTRIBUTING.md`](../../CONTRIBUTING.md) follow in a subsequent change.
  The publisher's template carries placeholders that the project fills
  without rewriting the text around them: `[PROJECT_NAME]`,
  `[SUBMISSION_INSTRUCTIONS]`, `[NONOWNER_INSTRUCTIONS]`, `[JURISDICTION]`,
  `[LIST_OF_MEDIA_LICENSES]` and the optional `[or Your Affiliates]`; the
  four outbound options Option Five replaces are left out. The fill values
  are decided in that change.
- The automated verification path will be configured when praetor plan item
  G48 ships.
- The introduction of a corporate contributor agreement (HA-CLA-E) will be
  triggered when the first enterprise contributor requests one.

## Decided in this record

The operator decided the contributor agreement policy on 2026-10-11 (ledger
id `contributor-agreement-text`, paraphrased): the agreement is Harmony
HA-CLA-I v1.0 unchanged with outbound licence Option Five; a five-line plain
summary stands above the unchanged text; Option Five is the promise to
contributors without a separate promise clause; no corporate agreement is
adopted now, to be revisited when the first company asks; commit sign-offs
stay; and acceptance is recorded by the fleet gate engine (praetor G48)
rather than a hosted CLA service or an archived action.

Choices derived and decided in this record:

- The operational consequence that until praetor G48 ships, no contribution
  from outside the maintainers is merged without recorded acceptance.
- The legal analysis of EUPL v1.2 Articles 2 and 5 demonstrating why inbound
  licensing under EUPL alone does not grant relicensing rights.

## Sources

- Project Harmony, Harmony Individual Contributor License Agreement
  (HA-CLA-I) v1.0 (2011-07-04):
  - Agreements directory: <https://www.harmonyagreements.org/agreements>
    (read 2026-10-17; lists version 1.0 of 2011-07-04).
  - OpenDocument Text (ODT):
    <https://www.harmonyagreements.org/docs/ha-cla-i-v1.odt> (read
    2026-10-17; SHA-256:
    `319226a66a26ee8e5a2d126bb5a738d51adfb788291d628cbb2657dceaea5085`).
  - PDF:
    <https://www.harmonyagreements.org/docs/ha-cla-i-v1.pdf> (read
    2026-10-17; SHA-256:
    `814aa5910efe90f9aba8737de0b31a0c21a2cbcc105a35eaea2205fc7b6ae452`).
  - Licence: Creative Commons Attribution 3.0 Unported (CC BY 3.0),
    <https://creativecommons.org/licenses/by/3.0/>.
- European Union Public Licence (EUPL v1.2):
  - Official text:
    <https://interoperable-europe.ec.europa.eu/collection/eupl/eupl-text-eupl-12>
    (read 2026-10-17).
  - Commission Implementing Decision (EU) 2017/863, EUR-Lex:
    <https://eur-lex.europa.eu/eli/dec_impl/2017/863/oj> (read 2026-10-17).
  - Local repository copy:
    [`LICENSES/EUPL-1.2.txt`](../../LICENSES/EUPL-1.2.txt).
- Developer Certificate of Origin (DCO) Version 1.1:
  - Source: <https://developercertificate.org/> (read 2026-10-17).
- Contributor Assistant GitHub Action (archived):
  - Repository `contributor-assistant/github-action`, archived, last
    release 2024-09-26 (read 2026-10-11).
- Diallak repository issue #123:
  - <https://github.com/tekmora/diallak/issues/123>.
