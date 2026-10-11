# Record of processing

Draft. This is the record of processing activities that Article 30(1) of
Regulation (EU) 2016/679 (GDPR) asks a controller to maintain. The exemption
for small organisations in Article 30(5) does not fit: it ends where processing
"is not occasional", and the bot processes whenever a member asks.

The record describes the code on `main`. A change that makes the bot store or
pass on more data changes this record in the same pull request, like the
[data inventory](data-inventory.md) it is built from.

## The record

<!-- markdownlint-disable MD013 -->
| Article 30(1) | Entry |
| :--- | :--- |
| (a) Controller and contact | The provider named in the [imprint](imprint.md); the block there is filled before the public launch. No data protection officer is appointed |
| (b) Purposes | Answering a member's question about project documentation; refusing a command that comes from another server |
| (c) Data subjects and data | Members of the community server who use `/ask`: the question they type and the id of the server. Discord attaches further fields to a command (member id, name, roles, locale); the bot reads none of them and keeps none |
| (d) Recipients | Discord, which delivers the reply. In model mode only: the model endpoint the operator runs, which receives the question and documentation passages, no member id and no name |
| (e) Transfers to a third country | The bot and the model endpoint run in Germany and send data to no other place. The reply is delivered by Discord; what this entry has to say about that is part of the legal review named in the [privacy notice](privacy-notice.md) |
| (f) Time limits for erasure | The question: in memory until the reply is sent. Bot logs, which hold no personal data: 30 days. In model mode, at the endpoint: an answer in the memory cache at most 60 minutes, a log line with the matched fragment of a blocked question about 74 hours, the record of a request, which holds no question and no answer, 30 days. Decided for later features: a support ticket 30 days after it is closed, the moderation trail 90 days |
| (g) Security measures | See the next section |
<!-- markdownlint-enable MD013 -->

## Security measures

A general description, as Article 30(1)(g) asks:

- **Nothing is stored.** The bot has no database and writes no file
  ([data inventory](data-inventory.md), "What the bot stores").
- **Logs hold no content.** No question, no answer and no member id reach the
  log; tests pin it (data inventory, "What the bot logs").
- **Credentials are files.** The bot token and the model key are read from
  files, never from the environment, and are never printed
  ([`internal/config`](../../internal/config/config.go)).
- **Least privilege on the platform.** The bot requests no gateway intent, a
  reply is visible only to the member who asked, and a reply can mention nobody
  ([`internal/discord`](../../internal/discord/disgo.go)).
- **One destination for member text.** In model mode the question goes to the
  configured endpoint only; the client follows no redirect
  ([`internal/model`](../../internal/model/model.go)).
- **A verifiable image.** The release image is signed, carries build provenance
  and an SBOM, and runs as a user without rights on a base without a shell
  ([hosting note](../hosting.md), "Image").

## Sources

- Regulation (EU) 2016/679, Article 30(1) and (5), as published, read on
  2026-10-10 from the EU Publications Office:
  <http://publications.europa.eu/resource/celex/32016R0679>
