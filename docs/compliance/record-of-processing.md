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
| (b) Purposes | Answering a member's question about project documentation with `/ask`; refusing a command that comes from another server; showing a member the receipt of a reply and, when the member asks, sending it as a direct message; giving and taking the interest, language and notification roles a member chooses with `/areas`, and refusing a member who has not passed the server's screening; carrying out a moderator's act with `Remove message` or `/timeout` and keeping its trail; when the support forum is configured, giving a first answer from the documentation to a new post in it that carries the opt-in tag |
| (c) Data subjects and data | Members of the community server who use `/ask`: the question they type and the id of the server. Discord attaches further fields to a command (member id, name, roles, locale); the bot reads none of them and keeps none. Members who press for the receipt as a direct message: their member id. Members who use `/areas`: their member id, the roles they hold, whether they have passed the server's screening, and the options they choose. Members a moderator acts on with `Remove message` or `/timeout`, and the moderators: the member ids, the roles the moderator holds, the message, channel and moderator ids, and the moderator's reason, in the trail line. Members who open a post in the support forum with the opt-in tag: its title and the text of its first message |
| (d) Recipients | Discord, which delivers the reply and, on request, the receipt as a direct message; gives or takes the role a member chose with `/areas`, with a fixed reason, shows it to the other members and keeps an entry in the server's audit log for 45 days; carries out a moderator's act and holds the trail line in the log channel, which moderators and the operator read, and shows the public reply in a post of the support forum. In model mode only: the model endpoint the operator runs, which receives the question and documentation passages, no member id and no name; with the bot's own masking, its masking route receives the question and the model only the masked text. The model endpoint receives nothing from the support forum |
| (e) Transfers to a third country | The bot and the model endpoint run in Germany and send data to no other place. The reply is handed to Discord, which delivers it. For members in the EEA Discord Netherlands BV is the controller of what Discord holds; Discord Inc. in the United States, which states that it takes part in the EU-U.S. Data Privacy Framework, also processes data. Named as a possible transfer, on the ground of the adequacy decision (EU) 2023/1795 (Article 45 GDPR); whether it is a transfer by the operator is not settled ([legal research](legal-research.md), point 5) |
| (f) Time limits for erasure | The question: in memory until the reply is sent. With `/areas`, the member id, roles and choice: in memory until the reply is sent; the role stays on the member until the member takes it off with `/areas`. The receipt of a reply, which holds no member id, and no text of the member unless the bot's own masking is on, when it holds the masked text of the question: in memory for 15 minutes, for at most 1000 replies at once, lost on a restart. The member id for a receipt sent as a direct message: in memory until it is sent. Title and text of a post in the support forum: in memory until the reply is sent or the attempt ends. Bot logs, which hold no personal data: 30 days. In model mode, at the endpoint: an answer in the memory cache at most 60 minutes, a log line with the matched fragment of a blocked question about 74 hours, the record of a request, which holds no question and no answer, 30 days. The moderation trail: 90 days by default, 1 to 365 days as configured, swept once a day. Decided for a later feature: a support ticket 30 days after it is closed |
| (g) Security measures | See the next section |
<!-- markdownlint-enable MD013 -->

## Security measures

A general description, as Article 30(1)(g) asks:

- **Nothing about members is stored.** The bot has no database. The only
  file it writes is the news state file when the news function is on, and it
  holds public feed items and read times only
  ([data inventory](data-inventory.md), "What the bot stores").
- **Logs hold no content.** No question, no answer and no member id reach the
  log; tests pin it (data inventory, "What the bot logs").
- **Credentials are files.** The bot token and the model key are read from
  files, never from the environment, and are never printed
  ([`internal/config`](../../internal/config/config.go)).
- **Least privilege on the platform.** The bot requests no gateway intent, a
  reply is visible only to the member who asked, and a reply can mention nobody
  ([`internal/discord`](../../internal/discord/disgo.go)). When the support
  forum is configured the session requests the one standard intent that
  carries thread events, so no message event reaches the bot; it requests one
  message per new opted-in post in one channel. The application holds the
  platform's right to read message content and uses it for that request only
  ([`internal/discord`](../../internal/discord/disgo_forum.go)).
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
