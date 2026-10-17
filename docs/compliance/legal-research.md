# Legal research: open points of the privacy notice and the forum answer

This note examines the nine open points that the
[privacy notice](privacy-notice.md) ("Legal points researched") and the
[support forum answer](support-forum-answer.md) ("Before launch") list. For each
point it gives a finding, the sources it rests on and what would change in the
drafts.

It is not legal advice. No lawyer was consulted: the project's direction is
research instead of a lawyer until somebody funds the project. Every quotation
below was compared word for word with a copy of its source fetched on
2026-10-15, and the sources added to point 5 on 2026-10-17. A statement no
fetched source carries is marked **Unverified**.

The research informs the drafts; it is not a gate for the launch. Three
findings are tracked as issues for the support forum feature: the right to
object in the guidelines text (#145), data of the kinds Article 9(1) lists in a
post (#146) and the reply delivered through Discord (#147).

<!-- markdownlint-disable MD013 -->
| # | Point | Short finding | Confidence |
| :- | :--- | :--- | :--- |
| [1](#1-identity-block-and-imprint) | Identity block and imprint | MStV § 18 applies; DDG § 5 likely not | medium |
| [2](#2-legitimate-interests-for-ask) | Legitimate interests for `/ask` | Fits; the interest is named | high |
| [3](#3-no-data-protection-officer) | No data protection officer | None required | high |
| [4](#4-complaint-without-a-named-authority) | Complaint, no authority named | Allowed | high |
| [5](#5-where-the-bot-runs) | Where the bot runs | Move needs processor contract and notice change; Discord reply named as a possible transfer under the adequacy decision | medium |
| [6](#6-right-to-object-at-first-contact) | Right to object at first contact | Own section suffices if linked before first use | medium |
| [7](#7-reading-forum-text-not-addressed-to-the-bot) | Forum text not addressed to the bot | Can be carried, with the safeguards | medium |
| [8](#8-informing-a-forum-member-in-time) | Informing a forum member in time | Art. 13 met; guidelines lack the right to object | medium |
| [9](#9-special-category-data-in-a-forum-post) | Special-category data in a post | Prohibition applies; no clear exception | low |
<!-- markdownlint-enable MD013 -->

## 1. Identity block and imprint

**Question.** What do the identity block and the imprint have to hold for a
non-commercial project run by a private person in Germany?

**Finding.** Article 13(1)(a) GDPR asks for the identity and the contact
details of the controller. Under German law, MStV § 18(1) applies to every
telemedium that does not serve exclusively personal or family purposes and asks
for name and address. DDG § 5(1) applies to digital services offered
"geschäftsmäßig" and as a rule against payment; a free, non-commercial bot is
likely outside it, so MStV § 18(1) is the rule that binds. The current block
(full name, c/o address for legal mail, e-mail) names name, address and an
electronic contact.

**Unverified:** that a c/o address satisfies "Anschrift" in MStV § 18(1); German
case law on the address that must accept service was not fetched. Also
unverified: whether the community server, not only the bot, counts as the
telemedium.

**Sources.**

- GDPR Art. 13(1)(a), <https://eur-lex.europa.eu/eli/reg/2016/679/oj>: "the
  identity and the contact details of the controller and, where applicable, of
  the controller's representative;"
- MStV § 18(1), <https://www.gesetze-bayern.de/Content/Document/MStV-18>:
  "Anbieter von Telemedien, die nicht ausschließlich persönlichen oder
  familiären Zwecken dienen, haben folgende Informationen leicht erkennbar,
  unmittelbar erreichbar und ständig verfügbar zu halten: 1. Name und Anschrift
  sowie 2. bei juristischen Personen auch Name und Anschrift des
  Vertretungsberechtigten."
- DDG § 5(1), <https://www.gesetze-im-internet.de/ddg/__5.html>:
  "Diensteanbieter haben für geschäftsmäßige, in der Regel gegen Entgelt
  angebotene digitale Dienste folgende Informationen, die leicht erkennbar und
  unmittelbar erreichbar sein müssen, ständig verfügbar zu halten"

**Effect on the drafts.** [Imprint](imprint.md): its "Legal research" section
can name MStV § 18(1) as the rule it follows.

## 2. Legitimate interests for `/ask`

**Question.** Does Article 6(1)(f) GDPR fit answering a question a member puts
to the bot, and must the interest be named?

**Finding.** Yes. The member calls the bot and expects an answer, which is the
reasonable expectation recital 47 asks about. The question is needed to find the
excerpts, it is held in memory only until the reply, and nothing is stored or
logged with a member id. Article 13(1)(d) requires the notice to name the
interest; the notice does ("to answer the question a member puts to the bot").

**Sources.**

- GDPR Art. 13(1)(d): "where the processing is based on point (f) of Article
  6(1), the legitimate interests pursued by the controller or by a third party;"
- GDPR recital 47: "At any rate the existence of a legitimate interest would
  need careful assessment including whether a data subject can reasonably
  expect at the time and in the context of the collection of the personal data
  that processing for that purpose may take place."
- EDPB Guidelines 1/2024 on processing based on Article 6(1)(f), version 1.0
  adopted 8 October 2024 for public consultation, paragraph 67,
  <https://www.edpb.europa.eu/system/files/2024-10/edpb_guidelines_202401_legitimateinterest_en.pdf>:
  "Furthermore, when the processing is based on Article 6(1)(f) GDPR, the
  specific legitimate interest(s) pursued must be precisely identified and
  communicated to the data subject in accordance with Article 13(1)(d) and
  14(2)(b) GDPR."
- WP29 Guidelines on transparency (WP260 rev.01), annex, page 36,
  <https://ec.europa.eu/newsroom/article29/redirection/document/51025>: "The
  specific interest in question must be identified for the benefit of the data
  subject."

**Unverified:** whether the EDPB has adopted a final version of Guidelines
1/2024 with other paragraph numbers.

**Effect on the drafts.** None.

## 3. No data protection officer

**Question.** Must the controller designate a data protection officer?

**Finding.** No. Article 37(1) GDPR names public bodies, large-scale monitoring
and large-scale special-category processing as core activity; none applies.
BDSG § 38(1) adds a duty from 20 persons constantly engaged in automated
processing, or for processing that needs a data protection impact assessment.
The project has one maintainer.

**Unverified:** that the forum feature needs no impact assessment under
Article 35; the German authorities' list of processing that always needs one
was not fetched.

**Sources.**

- GDPR Art. 37(1): "The controller and the processor shall designate a data
  protection officer in any case where:"
- BDSG § 38(1), <https://www.gesetze-im-internet.de/bdsg_2018/__38.html>:
  "Ergänzend zu Artikel 37 Absatz 1 Buchstabe b und c der Verordnung (EU)
  2016/679 benennen der Verantwortliche und der Auftragsverarbeiter eine
  Datenschutzbeauftragte oder einen Datenschutzbeauftragten, soweit sie in der
  Regel mindestens 20 Personen ständig mit der automatisierten Verarbeitung
  personenbezogener Daten beschäftigen."

**Effect on the drafts.** None.

## 4. Complaint without a named authority

**Question.** May the notice state the right to complain without naming a
supervisory authority?

**Finding.** Yes. Article 13(2)(d) asks for the right, not for an authority.
Article 77(1) lets the member choose the authority of their residence, work
place or the place of the infringement, and WP260 asks the notice to explain
that choice.

**Sources.**

- GDPR Art. 13(2)(d): "the right to lodge a complaint with a supervisory
  authority;"
- WP260 rev.01, annex, page 39: "This information should explain that, in
  accordance with Article 77, a data subject has the right to lodge a complaint
  with a supervisory authority, in particular in the Member State of his or her
  habitual residence, place of work or of an alleged infringement of the GDPR."

**Effect on the drafts.** [Privacy notice](privacy-notice.md), "Your rights":
the sentence on complaints can name the member's choice of Member State.

## 5. Where the bot runs

**Question.** What must the notice say about where the bot runs, now and after
the move to a rented server?

**Finding.** Now: no processor, nothing to disclose beyond Discord. After the
move: the hosting provider processes data on the controller's behalf, so a
contract under Article 28(3) is needed, and the provider is a recipient the
notice names (Article 13(1)(e); WP260 counts processors as recipients). A data
centre outside the EEA would add the statement of Article 13(1)(f). The
notice already plans this block change.

**The reply through Discord (#147, sources fetched 2026-10-17).** Chapter V
applies when three criteria are met: an exporter subject to the GDPR, which
discloses or makes data available to another controller or processor, which is
in a third country (EDPB Guidelines 05/2021). The operator is subject to the
GDPR and hands the reply to Discord. For members in the EEA the controller on
Discord's side is Discord Netherlands BV, which is not in a third country;
Discord Inc. in the United States states that it complies with the EU-U.S.
Data Privacy Framework. The General Court upheld the adequacy decision for that
framework on 3 September 2025 (T-553/23, Latombe v Commission). No supervisory
authority or EDPB text about a bot that replies through a platform's interface
was found. The drafts therefore name the reply as a possible transfer and the
adequacy decision as its ground, without claiming that it is one.

**Unverified:** which Discord entity is the developer's counterpart under the
Developer Terms of Service and which transfer clauses they contain (the page
refused automated fetches on 2026-10-17), whether Discord Inc.'s entry on the
Data Privacy Framework list is active today (the list needs a browser), and the
state of an appeal against T-553/23.

**Sources.**

- GDPR Art. 13(1)(e): "the recipients or categories of recipients of the
  personal data, if any;"
- GDPR Art. 28(3): "Processing by a processor shall be governed by a contract
  or other legal act under Union or Member State law, that is binding on the
  processor with regard to the controller and that sets out the subject-matter
  and duration of the processing, the nature and purpose of the processing, the
  type of personal data and categories of data subjects and the obligations and
  rights of the controller."
- WP260 rev.01, annex, page 37: "Therefore, other data controllers, joint
  controllers and processors to whom data is transferred or disclosed are
  covered by the term “recipient” and information on such recipients should be
  provided in addition to information on third party recipients."
- EDPB Guidelines 05/2021 on the interplay between the application of Article 3
  and the provisions on international transfers as per Chapter V of the GDPR,
  version 2.0, adopted 14 February 2023, section 2.2: "this second criterion
  cannot be considered as fulfilled when there is no controller or processor
  sending or making the data available (i.e. no “exporter”) to another
  controller or processor, such as when data are disclosed directly by the
  data subject to the recipient."
  <https://www.edpb.europa.eu/system/files/documents/2023-02/edpb_guidelines_05-2021_interplay_between_the_application_of_art3-chapter_v_of_the_gdpr_v2_en_0.pdf>
- Discord Privacy Policy, last updated 29 August 2025, section 1: "If you
  reside in the European Economic Area (“EEA”), Discord Netherlands BV is the
  “data controller” of your personal information collected through the
  services"; section 9: "When transferring data outside the EEA, we use
  standard contract clauses (Module 1 and Module 2)", and Discord's U.S.
  entities "comply with the EU-U.S. and Swiss-U.S. Data Privacy Frameworks".
  <https://discord.com/privacy>
- General Court, judgment of 3 September 2025, T-553/23, Latombe v Commission,
  on Commission Implementing Decision (EU) 2023/1795 (OJ 2023 L 231, p. 118);
  date and parties from the InfoCuria case list, judgment text not fetched.

**Effect on the drafts.** [Privacy notice](privacy-notice.md), "Where the bot
runs" and "Who receives your data": the provider and its country after the
move, as already planned. The reply through Discord is named as a possible
transfer, in the notice and in entry (e) of the
[record of processing](record-of-processing.md).

## 6. Right to object at first contact

**Question.** Must the bot itself point a member to the right to object in its
first communication?

**Finding.** Article 21(4) asks that the right is brought to the member's
attention at the latest at the first communication, clearly and separately
from other information. It does not say the bot's own message must carry it.
The notice gives the right its own section, second after the summary. WP260
accepts a layered approach in a digital setting. The project's reading: the
duty is met when that section is linked where the member meets the bot (bot
profile, server rules) before the first answer. That linking is already a gate
of the public launch.

**Unverified:** that a linked notice, rather than a line in the reply, counts as
bringing the right to the member's attention; WP260 states the duty but gives
no example for this case.

**Sources.**

- GDPR Art. 21(4): "At the latest at the time of the first communication with
  the data subject, the right referred to in paragraphs 1 and 2 shall be
  explicitly brought to the attention of the data subject and shall be
  presented clearly and separately from any other information."
- WP260 rev.01, annex, page 39: "In particular, the right to object to
  processing must be explicitly brought to the data subject’s attention at the
  latest at the time of first communication with the data subject and must be
  presented clearly and separately from any other information."
- WP260 rev.01, paragraph 35: "In the digital context, in light of the volume
  of information which is required to be provided to the data subject, a
  layered approach may be followed by data controllers where they opt to use a
  combination of methods to ensure transparency."

**Effect on the drafts.** None beyond the existing launch gate "publication +
links" in the [compliance overview](README.md).

## 7. Reading forum text not addressed to the bot

**Question.** Does Article 6(1)(f) carry reading the first message of a forum
post the member did not address to the bot, and does the opt-out tag change
the balance?

**Finding.** It can, but the balance is closer than for `/ask`. A member who
posts in a support forum expects people to read it; the Court holds that
interests may override where data subjects do not reasonably expect the
processing. The guidelines text tells the member before posting, and the
safeguards limit the impact: one read at creation, in memory only, no model,
nothing stored or logged, and a tag that keeps the bot away. The EDPB names
such mitigating measures as part of the balance.

**Sources.**

- GDPR recital 47: "The legitimate interests of a controller, including those
  of a controller to which the personal data may be disclosed, or of a third
  party, may provide a legal basis for processing, provided that the interests
  or the fundamental rights and freedoms of the data subject are not
  overriding, taking into consideration the reasonable expectations of data
  subjects based on their relationship with the controller."
- CJEU, judgment of 4 July 2023, C-252/21, Meta Platforms and Others,
  paragraph 112, ECLI:EU:C:2023:537, read through
  <http://publications.europa.eu/resource/celex/62021CJ0252>: "Furthermore, as
  can be seen from recital 47 of the GDPR, the interests and fundamental rights
  of the data subject may in particular override the interest of the data
  controller where personal data are processed in circumstances where data
  subjects do not reasonably expect such processing."
- EDPB Guidelines 1/2024 (consultation version), paragraph 56: "However, if the
  data subject’s interests, rights and freedoms seem to override the legitimate
  interest(s) being pursued, the controller may consider introducing mitigating
  measures to limit the impact of the processing on data subjects, in view of
  achieving a fair balance between the rights, freedoms and interests
  involved."

**Effect on the drafts.** None.

## 8. Informing a forum member in time

**Question.** Do the forum guidelines text and the privacy notice inform a
member in time (Articles 13(1) and 21(4))?

**Finding.** For Article 13(1): yes. The guidelines are shown before a member
posts, name the purpose and link the notice, which matches the first layer
WP260 describes. For Article 21(4): not yet. The opt-out tag is a way to keep
the bot away from one post; it is not the right to object of Article 21(1). The
guidelines text names the tag but not the right, and the bot's public reply in
the post is a communication with the member.

**Sources.**

- GDPR Art. 13(1): "Where personal data relating to a data subject are
  collected from the data subject, the controller shall, at the time when
  personal data are obtained, provide the data subject with all of the
  following information:"
- WP260 rev.01, paragraph 27: "Where Article 13 applies, under Article 13.1 the
  information must be provided “at the time when personal data are obtained”."
- WP260 rev.01, paragraph 36: "As regards the content of the first modality
  used by a controller to inform data subjects in a layered approach (in other
  words the primary way in which the controller first engages with a data
  subject), or the content of the first layer of a layered privacy statement/
  notice, WP29 recommends that the first layer/ modality should include the
  details of the purposes of processing, the identity of controller and a
  description of the data subject’s rights."
- GDPR Art. 21(4), as quoted under point 6.

**Effect on the drafts.** The guidelines text in
[support forum answer](support-forum-answer.md) and ADR-0009 gains one
separate sentence: the member may object to the reading at any time, with a
link to "Your right to object". ADR-0009 is accepted, so the change needs a
record that amends it.

## 9. Special-category data in a forum post

**Question.** What follows when a forum post holds data of the kinds Article
9(1) lists?

**Finding.** The prohibition of Article 9(1) applies whatever the bot is
looking for: the Court holds it independent of the controller's aim, and data
that reveal such a category indirectly are covered too. Article 6(1)(f) alone
does not lift it. The exception for data "manifestly made public by the data
subject" (Article 9(2)(e)) is read strictly: it covers only the poster's own
data, never data about other people, and only where the poster clearly chose to
make it accessible to the general public. Whether a post in the server's forum
is accessible to the general public depends on how the server is joined. No
other exception of Article 9(2) fits on the facts. The safeguards (one read,
nothing kept, opt-out tag) lower the impact; they are not an exception.

**Unverified:** whether the community server is open to anyone with a public
invite; how the German authorities treat incidental special-category data in
automated reading of posts.

**Sources.**

- GDPR Art. 9(1): "Processing of personal data revealing racial or ethnic
  origin, political opinions, religious or philosophical beliefs, or trade
  union membership, and the processing of genetic data, biometric data for the
  purpose of uniquely identifying a natural person, data concerning health or
  data concerning a natural person's sex life or sexual orientation shall be
  prohibited."
- C-252/21, paragraph 69: "As the Advocate General observed, in essence, in
  points 40 and 41 of his Opinion, that fundamental prohibition, laid down in
  Article 9(1) of the GDPR, is independent of whether or not the information
  revealed by the processing operation in question is correct and of whether
  the controller is acting with the aim of obtaining information that falls
  within one of the special categories referred to in that provision."
- C-252/21, paragraph 75: "Accordingly, it is not applicable to data concerning
  persons other than the person who made those data public."
- C-252/21, paragraph 77: "It follows that, for the purposes of the application
  of the exception laid down in Article 9(2)(e) of the GDPR, it is important to
  ascertain whether the data subject had intended, explicitly and by a clear
  affirmative action, to make the personal data in question accessible to the
  general public."
- CJEU, judgment of 1 August 2022, C-184/20, OT, paragraph 127, read through
  <http://publications.europa.eu/resource/celex/62020CJ0184>: "Consequently,
  those provisions cannot be interpreted as meaning that the processing of
  personal data that are liable indirectly to reveal sensitive information
  concerning a natural person is excluded from the strengthened protection
  regime prescribed by those provisions, if the effectiveness of that regime
  and the protection of the fundamental rights and freedoms of natural persons
  that it is intended to ensure are not to be compromised."

**Effect on the drafts.** [Support forum answer](support-forum-answer.md),
"What you should not send": the added sentence names health data and the other
kinds of Article 9(1), and data about other people. This lowers the risk; it
does not settle the point, which stays open for the forum feature.

## Not covered

- Youth protection law.
- The role under the AI Act: [ADR-0006](../adr/0006-ai-act-role.md) and
  [ADR-0013](../adr/0013-guardrail-policy-and-receipt.md).
- Discord's developer terms and policies.
- Data protection law of Member States other than Germany.
