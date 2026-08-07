# External Terminology Resource Portfolio for Tier-6 Reconciliation

**Status:** Approved direction and implementation design (Option 3: governed dual-track portfolio, 2026-08-07); Stage-0/1 tooling implemented and live-accepted 2026-08-07 — production activation still requires operator bootstrap (§13.3)<br>
**Task:** Resolve the selection/design portion of the authoritative-catalog gap in Tier-6 design §10.9.1; the operational blocker remains until the portfolio is built and passes §10<br>
**Scope:** External terminology research and ingestion policy; no Tier-6 implementation changes<br>
**Governing design:** `docs/superpowers/specs/2026-08-07-model-agnostic-tier6-validation-design.md`<br>
**Governing keyword spec:** `KnowledgeStore/doc-repo/specs/202608/2026080403-spec-keyword-canonicalization-and-reconciliation.md`

## 1. Executive decision

SemOS will use a **governed, staged portfolio**, not a single universal terminology source.

“Dual-track” means that imported evidence travels through two explicitly different governance tracks:

- The **authoritative-identity track** contains scoped sources and relations allowed to establish exact identity after their configured conditions pass: initially SIRP, configured QUDT exact identities/crosswalks and reviewed IEV mappings; later LOINC and licensed standards.
- The **proposal/discovery track** contains sources that may retrieve labels and candidate relationships but can never authorize their own promotion: initially Wikidata; later IATE, CC-CEDICT and other optional retrieval sources.

UCUM, device taxonomies and semantic facets form a supporting-evidence plane. They can validate units, add context or veto an incompatible proposal, but they are not a third identity track and do not independently authorize metric identity.

The portfolio separates four jobs that no evaluated resource performs safely by itself:

1. **Authoritative quantity identity:** BIPM SI Reference Point (SIRP).
2. **Operational quantity ontology and crosswalks:** QUDT.
3. **Authoritative bilingual domain decisions:** a curated IEC IEV seed, followed by licensed IEC/ISO content where rights permit.
4. **Multilingual candidate generation:** revision-pinned Wikidata and other proposal-only resources.

The first bootstrap package is:

```text
BIPM SIRP 1.0.0        authoritative SI quantity and unit identities
        ↕ explicit exact crosswalks only
QUDT 3.5.0             operational quantities, units, dimensions, mappings
        +
curated IEC IEV seed   governed English–Chinese identity and distinction decisions
        +
Wikidata snapshot      multilingual proposals; never authority by itself
        +
UCUM 2.2               unit-code normalization; never quantity identity
```

The first machine import should extend the existing QUDT importer and add a SIRP adapter. The first bilingual bootstrap should be a small, reviewed seed catalog of high-frequency pilot metrics mapped to stable IEV references. Wikidata may accelerate discovery of Chinese labels, but it must not independently authorize a merge.

Clinical observation terminology is a separate second-stage lane led by LOINC 2.82 and its official `zh_CN` linguistic variant. SNOMED CT, UMLS, NCIt, PATO and IUPAC are conditional or specialized later additions. ISO OBP must not be harvested because its terms prohibit systematic automated extraction.

This design is model-agnostic: embeddings may retrieve candidates, but no embedding model, cosine value, translation model or lexical source determines identity.

## 2. Question being resolved

Tier 6 requires authoritative exact-identity evidence before it may merge two keyword concepts automatically. The approved validator can consume generic evidence providers, but useful cross-lingual operation still depends on a catalog strategy and bootstrap process.

The research therefore asks:

- Which sources can establish exact identity within a declared scope?
- Which sources have durable identifiers and usable release/version semantics?
- Which sources contain authoritative English–Chinese terms?
- Which sources may legally and operationally be imported?
- Which sources are useful only for proposals, context, unit validation or analyst discovery?
- What should SemOS import first without coupling Tier 6 to a particular model or source schema?

The research does not select an embedding model, change merge thresholds, authorize live Internet queries during reconciliation, or implement the selected portfolio. It resolves the strategy and bootstrap design; §10.9.1 remains an operational blocker until the importers, governed seed and acceptance tests exist.

## 3. Evaluation framework

Every resource was evaluated on eight dimensions.

| Dimension | Required question |
|---|---|
| Domain authority | Is the publisher competent to define identity in the relevant domain? |
| Identity semantics | Does one identifier actually represent one concept, observation or unit? |
| Relation semantics | Does the source distinguish exact, broader, narrower, related and translation relations? |
| Identifier stability | Are identifiers durable, and are replacement or retirement states represented? |
| Release reproducibility | Can an imported snapshot be pinned, hashed and replayed? |
| English–Chinese coverage | Are Chinese terms official, curated, complete enough and attached to the same identity? |
| Machine access | Is there a supported bulk file, API or knowledge graph? |
| Reuse rights | Do the terms permit the intended storage, processing and redistribution? |

### 3.1 Authority classes

SemOS assigns each source or source subset one of four roles. The role is persisted governance configuration, not an adapter inference.

| Role | May authorize a Tier-6 merge? | Meaning |
|---|---:|---|
| `exact_identity_authority` | Yes, within configured scope | Same governed source identity explicitly means the same concept |
| `conditional_identity_authority` | Only after documented conditions pass | Identity is authoritative, but context, license, language edition or semantic axes must be validated |
| `proposal_only` | No | Useful for candidate retrieval, labels or cross-checking |
| `context_only` | No | Identifies units, devices, events or facets rather than the complete metric concept |

Authority attaches to a configured **source subset and relation**, not to an organization globally. For example, QUDT quantity-kind identity may be authoritative while a generic `wikidataMatch` is not; UCUM is authoritative for unit codes but not for quantity concepts.

### 3.2 Exact identity is narrower than similarity

The following never independently prove exact metric identity:

- equal or compatible units;
- equal normalized strings;
- a translation dictionary gloss;
- one resource's broader, narrower, related or generic cross-reference;
- embedding proximity, rank, percentile or multi-model agreement;
- two entries that cite the same standard but say `modified` or `adapted`;
- labels attached to community-edited entities without an approved authoritative crosswalk.

## 4. Horizontal comparison: resource landscape

### 4.1 Core quantitative authorities

| Resource | Strength | Material limitation | Portfolio role |
|---|---|---|---|
| BIPM SIRP 1.0.0 | Highest-authority digital SI reference; persistent identifiers; Turtle, API and SPARQL; includes luminance | English/French, selected quantity coverage, new resource | Exact SI identity anchor |
| QUDT 3.5.0 | Broad quantities, units and dimensions; stable IRIs; versioned graphs; rich crosswalks; existing SemOS importer | Chinese labels are sparse; relation predicates have different strengths | Operational exact identities and explicit exact crosswalks |
| IEC Electropedia/IEV | Expert electrotechnical concepts; stable IEV references; multilingual equivalents including Chinese | Copyright/database rights; no supported open bulk feed found | Curated bilingual authority now; licensed bulk authority later |
| CIE e-ILV | Definitive light, vision and photometry distinctions; stable term numbers | Online access is not bulk-reuse permission; copyrighted standard content | Manual expert reference and veto source |
| UCUM 2.2 | Normative machine unit codes and grammar | Unit identity is not quantity or metric identity; Chinese translation is non-normative | Unit-code authority only |

The [BIPM SI Reference Point](https://www.bipm.org/en/-/2026-06-18-the-first-stable-version-of-the-si-reference-point) reached version 1.0.0 in June 2026. Its service exposes persistent identifiers, downloadable Turtle graphs, APIs and SPARQL. The knowledge model and web-service versions are different version axes and must not be conflated. The repository publishes the data under [CC BY 3.0 IGO](https://github.com/TheBIPM/SI_Digital_Framework/blob/main/LICENCE).

[QUDT 3.5.0](https://www.qudt.org/doc/DOC_VOCAB-QUANTITY-KINDS.html) contains 1,223 quantity-kind resources in a versioned graph, publishes stable vocabulary IRIs and declares CC BY 4.0. QUDT is the most implementation-ready source because SemOS already imports its Turtle files into the governed `quantity` module. The current importer, however, chooses one label using a character heuristic, hard-codes it as English and discards language-tagged alternatives. It also does not populate the keyword external-identity/evidence tables. Those limitations must be corrected before the importer can serve the bilingual portfolio.

[IEC Electropedia](https://www.electropedia.org/) publishes more than 20,000 electrotechnical entries in English and French with equivalents in languages including Chinese. One IEV reference identifies the entry across its language equivalents. This is excellent bilingual identity evidence, but IEC asserts copyright and database rights; substantial reproduction or systematic reuse requires permission under the [IEC copyright policy](https://webstore.iec.ch/en/copyright). The pilot therefore uses reviewed IEV identifier mappings and minimal provenance, not an unlicensed bulk copy of definitions.

[CIE e-ILV](https://www.cie.co.at/e-ilv) is especially valuable for the motivating display/photometry scope. It is an expert reference for semantic distinctions and negative evidence, but its individual online entries remain copyrighted CIE content.

[UCUM](https://ucum.org/ucum) defines normative unit codes and compound-unit grammar. Its [license](https://ucum.org/license) permits use subject to its conditions. UCUM identity may validate `cd/m2`, but compatible units alone cannot distinguish two different quantities sharing a dimension or unit.

### 4.2 Multilingual discovery and proposal sources

| Resource | Strength | Limitation | Portfolio role |
|---|---|---|---|
| Wikidata | CC0, stable Q-IDs, large multilingual label/alias coverage, weekly dumps and revisions | Community-edited; references and precision vary | Primary multilingual proposal source |
| IATE | Concept-oriented, provenance/reliability metadata, 24 EU languages, CSV/TBX export | Chinese is not in the promised language set; continuously changing export | European terminology proposals |
| CC-CEDICT | Downloadable Simplified/Traditional Chinese, pinyin and English glosses | General dictionary, mutable lexical entries, no governed biomedical concept identity, share-alike obligations | Low-weight lexical proposals only |
| Open Multilingual WordNet | Broad multilingual synset retrieval | Not regulated technical identity; component licenses and versions vary | Optional offline retrieval experiment |
| OM 2 | Open quantity/unit ontology with stable namespace and Chinese spreadsheets | Weak immutable release practice; Chinese labels include machine-generated history | Secondary cross-check only |

[Wikidata structured data](https://www.wikidata.org/wiki/Wikidata:Licensing) is CC0 and available through [versioned dumps and APIs](https://www.wikidata.org/wiki/Wikidata:Database_download). It is the strongest open multilingual proposal source. A pinned entity revision or dump date must be recorded. Wikidata is not exact authority by default; promotion requires a reviewed allowlist and an authoritative external crosswalk or domain decision.

[CC-CEDICT](https://cc-cedict.org/editor/editor.php?handler=Download) is valuable for recall but its entries are dictionary headwords and glosses, not stable regulated concepts. The current download page states CC BY-SA 4.0 while an older project page still states 3.0. The license discrepancy and share-alike implications should be reviewed before any bulk product integration.

### 4.3 Clinical and biomedical sources

| Resource | Identity scope | Chinese position | Portfolio decision |
|---|---|---|---|
| LOINC 2.82 | Clinical observations, laboratory tests, documents | Official updated `zh_CN` linguistic variant | `conditional_identity_authority` in Stage 2 |
| SNOMED CT | Clinical concepts with permanent SCTIDs and defining relationships | Requires an identified licensed language edition/refset | `conditional_identity_authority`; defer pending rights and edition approval |
| UMLS 2026AA | Cross-source CUIs, native IDs and terminology integration | Chinese coverage exists but is small and source-concentrated | `proposal_only` in a licensed analyst workflow |
| NCI Thesaurus | Oncology, biomedical research and selected FDA/CDISC partner content | No broad official Chinese layer established | `conditional_identity_authority` for allowlisted subsets/scopes |
| PATO | Phenotypic qualities and semantic facets | No official Chinese layer found | `context_only` |
| IUPAC Gold Book 5.0.0 | Chemistry and metrology terminology with stable term IDs/API | Primarily English | Later chemistry authority, license-scoped |

[LOINC 2.82](https://loinc.org/kb/loinc-release-notes) is the strongest second-stage source. A LOINC code identifies a clinical observation through multiple semantic axes—component, property, timing, system/specimen, scale and method—not merely a name. Its official Chinese linguistic variant can provide bilingual evidence for the same code. The [LOINC license](https://loinc.org/kb/license) allows broad use and redistribution subject to attribution, identifier/display-name, version and third-party-content conditions. LOINC must not be applied to engineering metrics such as display luminance unless a genuine LOINC observation with all necessary axes matches.

[SNOMED CT](https://www.snomed.org/licensing) has permanent concept identifiers and robust release semantics, but deployment and redistribution require territory-appropriate licensing. Chinese terms are authoritative only when taken from an identified licensed edition and language reference set.

[UMLS 2026AA](https://www.nlm.nih.gov/research/umls/licensedcontent/umlsknowledgesources.html) integrates 195 source vocabularies and 31 languages. Its source-specific restrictions make it useful for licensed analyst discovery and native-ID crosswalks, not a clean redistributed pilot catalog. Native LOINC, SNOMED or NCIt identifiers should remain primary; a UMLS CUI is a cross-source cluster identifier.

[NCI Thesaurus](https://evs.nci.nih.gov/ftp1/NCI_Thesaurus/ThesaurusTermsofUse.htm) is CC BY 4.0 and supports stable concept codes, APIs and bulk downloads. It is strong within NCIt's declared biomedical scope and selected, explicitly tagged FDA/CDISC partner subsets; partner use or tagging does not make all NCIt content regulatory authority. It does not supply the general English–Chinese metric layer needed by this pilot.

[PATO](https://obofoundry.org/ontology/pato.html) is CC BY 3.0 and useful for qualities such as size or temperature, but a quality facet does not encode the subject, method, timing, unit or device state required for a complete metric definition.

The [IUPAC Gold Book](https://goldbook.iupac.org/) exposes stable IDs and JSON/XML APIs. Individual terms are CC BY-SA 4.0; commercial collection licensing may require contact with IUPAC. It is a later chemistry/metrology source, not an initial bilingual medical-device source.

### 4.4 Device-context sources

| Resource | What it identifies | Portfolio role |
|---|---|---|
| IMDRF Adverse Event Terminology 2026 | Device problems, investigations, health effects and components | Context-only; machine-readable and reusable |
| EMDN v.2026 | Medical-device classes | Context-only; downloadable and versioned |
| IEC 60601 / ISO 80601 | Safety/performance terms within edition-pinned standards | Conditional exact authority when licensed |

[IMDRF adverse-event terminology](https://www.imdrf.org/documents/terminologies-categorized-adverse-event-reporting-aer-terms-terminology-and-codes) has stable hierarchical codes, official JSON/XLSX releases and annual change history. [EMDN](https://health.ec.europa.eu/medical-devices-topics-interest/european-medical-devices-nomenclature-emdn_en) similarly provides downloadable, versioned medical-device classification codes. Both are useful contextual taxonomies, but neither identifies a measurement concept such as luminance.

IEC 60601 and ISO 80601 terminology can be authoritative only when tied to a licensed, complete publication identity, edition/amendment set and clause. Particular standards may supplement or modify base-standard definitions; matching labels or citations do not imply exact identity.

### 4.5 Sources excluded from automated ingestion

The [ISO Online Browsing Platform terms](https://www.iso.org/fr/footer-links/obp-tems-of-use.html) prohibit systematic extraction and automated reuse, including data-mining/AI uses. OBP is also a partial browsing interface rather than the official complete publication. It may be used for human discovery and citation, not as an importer source.

IEC and CIE web interfaces are likewise not evidence of bulk-reuse permission. No scraper is part of this design. Licensed publications or written feed permissions are separate future inputs.

## 5. Longitudinal view: why a staged portfolio is now practical

The resource landscape has changed materially over time:

| Period | Development | Consequence for SemOS |
|---|---|---|
| 2020 | IEC 60050-845 and CIE S 017:2020 harmonized lighting terminology | Stable expert distinction between luminance and brightness |
| 2024 | UCUM 2.2 and broader FAIR metrology work matured | Reliable unit-code layer, still not metric identity |
| 2025 | IUPAC Gold Book 5.0.0 and expanding machine APIs | More domain terminology can be adapter-driven |
| Feb 2026 | LOINC 2.82 updated its Chinese linguistic variant | Practical clinical bilingual lane |
| Jun 2026 | BIPM released SIRP 1.0.0 | A machine-readable authoritative SI identity anchor now exists |
| Jul 2026 | QUDT 3.5.0 published a broad current quantity graph | Immediate operational bridge and crosswalk layer |

Earlier designs often had to choose between a standards PDF and a community ontology. In 2026, SemOS can anchor identity in SIRP, operationalize it through QUDT, and reserve copyrighted multilingual standards for governed decisions. This reduces—but does not eliminate—the need for human curation.

## 6. Staged portfolio

### 6.1 Stage 0: source-policy registry and adapter contract

No source may influence Tier 6 before its policy is registered. At minimum, governance must record:

```text
provider_id
source
source_subset
release_or_revision
retrieved_at
content_checksum
license_id_or_uri
license_review_status
authority_role
authoritative_relations
allowed_scopes
language_editions
adapter_version
provenance_locator
approved_by / approved_at
```

The existing `kb.keyword_sources` table records source, release, license, retrieval time and notes, but it does not structurally encode authority role, allowed scope, relation policy, checksum or adapter version. Implementation planning should either extend that table or add a separate immutable source-policy table. Encoding these fields only in free-form `notes` is insufficient for fail-closed validation.

Each adapter converts its native representation into the generic evidence-provider contract from the Tier-6 design. The core validator never parses a QUDT IRI, IEV number, LOINC code or Wikidata Q-ID.

### 6.2 Stage 1: quantitative and bilingual pilot

Stage 1 has five coordinated components: four terminology/identity imports plus a supporting UCUM unit-code import.

#### 6.2.1 SIRP 1.0.0

Import the SIRP quantity and unit graphs using persistent resource identifiers. Pin:

- knowledge-model version `1.0.0`;
- each downloaded Turtle checksum;
- retrieval timestamp;
- adapter version;
- CC BY 3.0 IGO attribution.

Example identity:

```text
(bipm-sirp-quantity, https://si-digital-framework.org/quantities/LUMA, 1.0.0)
```

SIRP quantity identity is authoritative within the imported SIRP scope. English/French labels are catalog surfaces; SIRP does not supply the Chinese layer.

#### 6.2.2 QUDT 3.5.0

Extend `server/cmd/qudt-import` rather than creating a competing quantity importer. The extension must:

- preserve RDF language tags and all approved `rdfs:label`/`skos:altLabel` values;
- stop treating non-CJK text as equivalent to English language detection;
- preserve source-native IRIs and the defining graph version;
- distinguish `siExactMatch`/`exactMatch` from `wikidataMatch`, broader, narrower and related mappings;
- populate or feed the keyword external-identity/evidence adapter;
- record input checksums and adapter version;
- produce deterministic, idempotent re-import results;
- represent deprecation and replacement instead of silently losing history.

Example identity:

```text
(qudt-quantity-kind, http://qudt.org/vocab/quantitykind/Luminance, 3.5.0)
```

Only configured exact predicates may authorize cross-source identity. A generic `qudt:wikidataMatch` remains non-authoritative until reviewed.

#### 6.2.3 Curated IEC IEV bilingual seed

Build a reviewed seed of approximately 50 high-frequency or high-risk metric concepts from the pilot corpus. The final count is driven by corpus coverage, not a quota. Selection priority is:

1. top occurrence frequency;
2. known English–Chinese duplicates;
3. terms whose ordinary-language translation is technically ambiguous;
4. metrics used by current comparison matrices;
5. safety-critical or regulated terminology.

For each seed concept, a domain reviewer records:

- the stable IEV reference;
- source entry publication/status and retrieval date;
- the local English and Chinese surfaces observed in SemOS documents;
- whether the surfaces map exactly, contextually or not at all;
- applicable scope and unit/dimension constraints;
- reviewer and decision timestamp;
- minimal citation/provenance consistent with permitted use.

Until an IEC reuse license is obtained, SemOS must not bulk-copy IEV definitions or language tables. The governed record is the reviewer-approved mapping from local surfaces to the IEV identifier, not a scraped mirror of Electropedia.

For the live pilot:

```text
IEC IEV 845-21-050  luminance  / 亮度
IEC IEV 845-22-059  brightness / 视亮度
```

These are separate concepts. [CIE 17-21-050](https://www.cie.co.at/eilvterm/17-21-050) defines luminance as a measured density of luminous intensity with respect to projected area. [CIE 17-22-059](https://www.cie.co.at/eilvterm/17-22-059) defines brightness as an attribute of visual perception. In the photometry/display scope, their separation is authoritative negative evidence; compatible language usage or embedding similarity must not merge them.

Bare Chinese `亮度` remains context-sensitive outside that scope. A source document about a display measurement in `cd/m²` supports luminance; everyday prose about perceived brightness does not.

#### 6.2.4 Wikidata proposal snapshot

Import only the entities and fields needed for proposal generation, pinned to dump/revision identity:

- Q-ID and entity revision;
- language-tagged labels and aliases;
- relevant external identifiers and their references;
- explicit `different from`, broader/narrower and unit statements;
- retrieval/dump timestamp.

For the pilot, [Wikidata Q355386](https://www.wikidata.org/wiki/Q355386) represents luminance and links to QUDT and IEV identifiers; the separate brightness entity must not be collapsed with it. Wikidata evidence remains proposal-only unless an authoritative crosswalk and curator decision promote a specific mapping.

#### 6.2.5 UCUM 2.2 supporting unit import

Import the official UCUM 2.2 source/“essence” XML artifacts and pin:

- specification version `2.2` and publication date `2024-06-17`;
- each input checksum and retrieval timestamp;
- UCUM License 1.1 metadata;
- adapter version.

The output is a deterministic local unit-code registry that validates normative UCUM codes and compound expressions. It may emit `unit-compatible` or `unit-incompatible` supporting evidence only. It must not create or merge keyword concepts, treat print names or translations as normative, or emit authoritative quantity-identity claims. Where QUDT already carries a UCUM code, the adapter may validate that code but may not infer that two QUDT quantity kinds are identical merely because their units match.

### 6.3 Stage 2: clinical observations

Add LOINC 2.82 for clinical metrics. Preserve:

- LOINC code and status;
- all identity-defining axes;
- official language variant and display-name fields;
- release version;
- replacement/deprecation mappings;
- license notices and third-party restrictions.

Automatic identity is permitted only for a validated source-native code or a uniquely matching active code whose required semantic axes are evidenced. A bilingual label pair remains a proposal unless evidence establishes every axis material to the target code, including an explicitly methodless or unspecified axis where the selected LOINC defines one.

### 6.4 Stage 3: licensed standards and specialized domains

Subject to need and rights:

- obtain an IEC IEV bulk/reuse arrangement and replace manual seed maintenance with a licensed snapshot adapter;
- import edition-pinned IEC 60601/ISO 80601 term identities from licensed publications;
- add SNOMED CT only after territory, redistribution and Chinese refset licensing are resolved;
- enable UMLS only in a licensed analyst workflow;
- add selective NCIt, PATO and IUPAC adapters for declared scopes;
- import IMDRF/EMDN as separate contextual taxonomies, never as metric identity authorities.

## 7. Identity and promotion rules

### 7.1 Same-source identity

Two surfaces may receive the same authoritative target when they are approved labels or local mappings for the same source-native identifier in the same pinned release and scope.

```text
(source, external_id, release) → one active kb.keyword_concepts.concept_id
```

This is the triple representation already supported by `kb.keyword_external_ids`. The external ID remains opaque to the core reconciler.

### 7.2 Cross-source identity

Cross-source equivalence requires an explicitly authoritative relation and a governed mapping. It is never inferred from matching labels.

Example:

```text
BIPM SIRP LUMA @ 1.0.0
        ↕ approved exact crosswalk
QUDT quantitykind:Luminance @ 3.5.0
        ↕ reviewed domain mapping
IEC IEV 845-21-050
        → internal concept kwc_luminance
```

Each native identity remains stored separately in `kb.keyword_external_ids`; multiple triples may map to the same internal concept.

### 7.3 Proposal promotion

A proposal-only source can contribute a label or candidate but cannot promote itself. Promotion requires:

1. an authoritative source identity or approved internal governed term;
2. a reviewed exact relation to that identity;
3. compatible scope and semantic definition;
4. no conflict with unit, dimension, `never_merge`, alignment or source-negative evidence;
5. persisted reviewer or policy authority.

After promotion, the audit must still say which evidence was authoritative and which evidence merely suggested the candidate.

### 7.4 Negative evidence

The portfolio must preserve explicit distinctions, deprecated senses and `modified`/`adapted` source relationships. Negative evidence is not represented by lowering a score. It is a hard veto or a scoped non-equivalence assertion consumed by the deterministic validator.

## 8. Bootstrap workflow

```text
Select corpus slice and metric scopes
        ↓
Register source policy, release, rights and checksums
        ↓
Import SIRP/QUDT/UCUM into local immutable snapshots
        ↓
Generate multilingual proposals from pinned Wikidata data
        ↓
Domain reviewer maps high-value local EN/ZH surfaces to IEV/SIRP/QUDT IDs
        ↓
Validate exact, non-exact and veto relations
        ↓
Publish a versioned SemOS seed release
        ↓
Run Tier-6 reconciliation against local PostgreSQL evidence only
        ↓
Audit unresolved pairs and grow the seed catalog
```

No bootstrap step calls a live external API inside a Tier-6 decision transaction. Network access belongs to explicit import/snapshot jobs; reconciliation uses local, transactionally stable evidence.

## 9. Importer and storage requirements

### 9.1 Required adapter behavior

Every adapter must:

- be deterministic and idempotent;
- consume an explicit local input artifact or immutable API snapshot;
- validate the declared release/revision and checksum before writes;
- retain native identifiers as opaque strings;
- preserve language, term status, relation strength and source provenance;
- reject unknown relation predicates rather than upgrading them;
- retain tombstones/replacements needed to interpret prior decisions;
- write source metadata before evidence rows;
- produce a dry-run summary and stable audit counts;
- fail closed on malformed or incomplete authority data.

### 9.2 Storage fit

The existing triple tables remain the first supported representation:

- `kb.keyword_sources` — imported source release and license metadata;
- `kb.keyword_external_ids` — external identity to internal concept;
- `kb.keyword_surface_evidence` — source evidence for a surface.

They can encode every recommended source because `external_id` and `release` are provider-defined opaque strings. Source-specific detail that does not fit the triple is retained in adapter-owned staging tables or evidence payloads and normalized through the generic provider contract, consistent with Tier-6 design §10.9.2.

The governance gap is not identifier shape; it is structured source policy. Authority role, scope, allowed relations, snapshot checksum and adapter version must become queryable data before automatic merges are enabled.

### 9.3 Release semantics examples

| Source | `source` example | `external_id` example | `release` example |
|---|---|---|---|
| SIRP | `bipm-sirp-quantity` | full PID for `LUMA` | `1.0.0` |
| QUDT | `qudt-quantity-kind` | full `quantitykind/Luminance` IRI | `3.5.0` |
| IEC live IEV | `iec-iev` | `845-21-050` | immutable snapshot ID such as `snapshot-2026-08-07.sha256_prefix`; full checksum stored in source metadata |
| IEC publication | `iec-60050-845` | `845-21-050` | `2020` |
| Wikidata | `wikidata` | `Q355386` | dump date or revision ID |
| LOINC | `loinc` | native LOINC code | `2.82` |
| UCUM | `ucum` | normative unit code | `2.2` |

An entry publication date is not automatically a catalog release. Although an IEV reference is durable, its entry can be revised through the IEC maintenance process; an empty release would therefore conflate historical meanings. Adapters document which immutable version/snapshot axis is placed in `release` and preserve entry publication date, status, replacement history and full artifact checksum in source metadata.

## 10. Acceptance criteria

The portfolio bootstrap is complete only when all of the following pass.

### 10.1 Governance and reproducibility

- Every imported source has a reviewed role, scope, license, release/revision, retrieval timestamp, checksum and adapter version.
- The same input artifacts produce the same identities, surfaces, relations and audit counts on re-import.
- A source whose license or authority configuration is missing cannot emit authoritative claims.
- A superseded release remains interpretable for historic audit records.

### 10.2 Semantic safety

- `luminance`, `Luminance` and the approved display-metric use of `亮度` converge on one concept.
- Reviewer-approved uses of `brightness` and `视亮度` mapped to IEV `845-22-059` converge on a separate perceptual concept; the bare surfaces remain context-sensitive because product “brightness” can also name a control setting or another contextual metric.
- Luminance and brightness do not merge even if an embedding ranks them first or a product document uses the terms loosely.
- Unit compatibility supports but never independently authorizes a merge.
- Broader, narrower, related, translation-only and generic Wikidata/QUDT mappings never become exact implicitly.

### 10.3 Model agnosticism

- Re-running the same local evidence with different embedding models may change candidate order but not authoritative identity decisions.
- A candidate omitted from embedding top K can still be found by the exhaustive identity-evidence provider.
- A high cosine without authoritative identity remains deferred.

### 10.4 Operational coverage

- The seed catalog covers an agreed percentage of high-frequency pilot metric occurrences; the target is measured from the corpus before implementation planning.
- Uncovered bilingual pairs enter a review backlog with proposals and provenance rather than being silently merged.
- Source update jobs report additions, retirements, replacements, changed labels, changed relations and policy-impacting license changes.

## 11. Risks and controls

| Risk | Control |
|---|---|
| Treating an open source as authoritative everywhere | Scope authority by source subset and relation |
| Scraping copyrighted standards | Use identifier-only reviewed mappings; require written rights for bulk content |
| Chinese lexical ambiguity | Require scope/context and preserve separate IEV concepts |
| QUDT/Wikidata cross-reference overpromotion | Allow only explicitly configured exact predicates plus review |
| Silent source drift | Pin release/revision, checksum and adapter version |
| Losing provenance during normalization | Preserve native IDs, relations, language tags and evidence references |
| One giant catalog becoming a dependency | Keep adapters independent behind the generic provider contract |
| Clinical ontology applied to engineering metrics | Enforce declared source scope and semantic axes |
| Unit equality mistaken for meaning equality | Keep UCUM/unit gates supporting and non-authorizing |
| Human seed becoming stale | Publish versioned seed releases and review upstream change reports |

## 12. Decisions and remaining work

### 12.1 Decisions made

1. Use the governed dual-track staged portfolio.
2. Use SIRP as the highest-authority SI identity anchor.
3. Retain QUDT as the operational quantity ontology and first importer extension.
4. Use a curated IEV seed for the initial English–Chinese authority gap.
5. Use Wikidata for proposals only by default.
6. Use UCUM for unit-code identity only.
7. Add LOINC as the first clinical expansion.
8. Exclude ISO OBP from automated ingestion.
9. Keep all provider schemas behind the model-agnostic identity-evidence contract.
10. Keep the approved Tier-6 design unchanged; this document resolves the catalog selection/design portion as a separate task, while the operational §10.9.1 blocker remains until the portfolio is built and passes §10.

### 12.2 Work deferred to implementation planning

- exact database shape for structured source-policy governance;
- corpus query and target coverage percentage for the seed catalog;
- reviewer workflow and administrative UI;
- IEC permission/licensing request;
- detailed SIRP adapter and QUDT importer change plan;
- source-update scheduling and rollback procedure;
- legal review of each intended distribution model;
- whether negative source assertions require a dedicated table or reuse the existing `never_merge` mechanism with scoped provenance.

### 12.3 Resolved by the Stage-0/1 implementation

- Structured source-policy governance: `project_migrations/20260807000001_govern_keyword_identity_sources.sql` adds role, scopes, relations, checksum, license review, adapter version, provenance, approval and `identity_authority` columns, immutability triggers, artifact/catalog/label/relation/negative-decision/UCUM staging tables, and the audited `keyword_identity_deployments` pointer.
- Negative source assertions: a dedicated `kb.keyword_catalog_negative_decisions` staging table (adapter-owned) feeds a provenance-preserving promotion that writes the scoped `never_merge` veto; non-equivalence is never a lowered score.
- QUDT importer change plan: shared `ParseQUDTGraph` + `QUDTAdapter` (Task 7) preserves language tags, deprecation/replacement, exact vs `wikidataMatch`, and `siExactMatch` → SIRP persistent-identifier crosswalks; the legacy `cmd/qudt-import` command remains functional.
- SIRP adapter: `SIRPAdapter` plus the explicit SIRP/QUDT Luminance → LUMA fixture prove the exact crosswalk normalizes while the adjacent Wikidata mapping stays proposal-only.

## 13. Implementation status, operational commands, and acceptance

### 13.1 Stage-0/1 tooling

All Stage-0/1 tooling from the portfolio plan
(`docs/superpowers/plans/2026-08-07-external-terminology-resource-portfolio.md`) is
implemented and committed: corpus/coverage measurement, the governed source
registry and generic catalog schema, the source-agnostic identity-evidence
contract, transaction-scoped merge/audit primitives, deterministic Tier-6
validation, the immutable terminology import runner with diff/rollback, the
stage-1 adapters (SIRP, IEC seed, Wikidata, UCUM, QUDT), and reviewed
positive/negative promotion into `keyword_external_ids`,
`keyword_surface_evidence`, and the provenance-linked `never_merge` veto.

### 13.2 Operational commands

```text
# Download one freely available resource (network-enabled bootstrap only;
# writes local artifact + SHA-256 + unapproved draft manifest).
terminology-fetch list
terminology-fetch status [--dir <dir>] [--source <id>]
terminology-fetch fetch --source <id> --dir <dir> [--titles a,b,c]
# Import one immutable local source release (never fetches live URLs).
terminology-import import --manifest <manifest.json>
# Report a release diff between two manifests (json|summary).
terminology-import diff --base <base.json> --candidate <candidate.json> [--format json|summary]
# Move the audited deployment pointer; rollback restores the prior pointer.
terminology-import activate --deployment-key tier6-primary --source iec-60050-845 --release 2020 --changed-by <operator>
terminology-import rollback --deployment-key tier6-primary --changed-by <operator>
# Measure pilot-scope coverage against a reviewed seed release (read-only).
terminology-coverage --acceptance acceptance.json [--format json|summary]
# Offline Tier-6 reconciliation with exact-identity authority.
keyword-reconcile --scope display --identity-deployment-key tier6-primary
# Legacy quantity-module QUDT import (unchanged behavior).
qudt-import --units units.ttl --quantity-kinds quantity-kinds.ttl --dimensions dimensions.ttl
# Admin page API (System Admin > Resources > External Terminology Resources):
#   GET  /api/v1/terminology-resources
#   POST /api/v1/terminology-resources/:source/download
# Each response includes review_status (pending_review | approved | "") read
# from the source's manifest.draft.json. The Review page (System Admin >
# Resources > Review External Resources) lists only downloaded resources whose
# review_status is still pending_review.
# Storage defaults to TERMINOLOGY_DIR, else <DATA_HOME_DIR>/terminology.
```

Freely downloadable sources (no permission required): QUDT
(`https://qudt.org/download/3.5.0/qudt-all.ttl`, CC-BY-4.0), UCUM
(`https://raw.githubusercontent.com/ucum-org/ucum/v2.2/ucum-essence.xml`, UCUM
License 1.1), BIPM SIRP (`https://si-digital-framework.org/quantities`,
CC-BY-3.0-IGO), and a revision-pinned Wikidata pilot entity subset
(`wbgetentities`, CC0). IEC 60050-845 (IEV) is copyright-gated: retrieval is
refused by the tool and the page shows "Requires license"; only a licensed,
reviewed seed file is acceptable. Every fetched artifact records its retrieval
time and SHA-256; the draft manifest is written with
`license_review_status=pending_review` and no approval fields, so
`terminology-import` fails closed until an operator completes the license
review and approval (the only remaining manual gate).

### 13.3 Source manifest, diff, and rollback format

Each manifest is a reproducible source file plus its release policy:

```json
{
  "adapter": "iec-seed",
  "policy": {
    "provider_id": "iec", "source": "iec-60050-845", "source_subset": "pilot",
    "release": "2020", "retrieved_at": "2026-08-07T00:00:00Z",
    "content_checksum": "<sha256>", "license": "iec-reviewed-seed-2020",
    "license_review_status": "approved", "authority_role": "exact_identity_authority",
    "authoritative_relations": ["exact_equivalent"], "allowed_scopes": ["display"],
    "languages": ["en", "zh"], "adapter_version": "0.1.0",
    "provenance_locator": "https://example.test/iec-seed/v1",
    "approved_by": "ontology-board", "approved_at": "2026-08-07T00:00:00Z",
    "identity_authority": true
  },
  "artifacts": [
    {"id": "seed.json", "path": "seed.json", "sha256": "<sha256>",
     "media_type": "application/json", "provenance_locator": "https://example.test/iec-seed/seed.json"}
  ]
}
```

An exact replay is idempotent; any changed checksum, policy, scope, rights,
approval, or payload fails and requires a new release/snapshot identity.
`diff` reports added/retired/replaced entries, labels, relations, negative
decisions, UCUM codes, artifacts, and policy changes between two manifests.
`rollback` restores the previous audited `keyword_identity_deployments`
pointer and appends a `rollback` history row; every activation/rollback
records `changed_by` and `changed_at`.

### 13.4 Acceptance status

Live acceptance ran against a freshly rebuilt `chenweb_test`: all 220 project
migrations applied (plus guarded Down/Up round-trip), all five stage-1
fixtures imported twice with idempotent replay, and `tier6-primary` enabled on
`iec-60050-845/2020`. The integration test suite
(`reconcile_identity_integration_test.go`) proved exact-identity merges
independent of cosine, deferral without a reviewed mapping, conflict
rejection, audit-row invariants, and family-lock serialization.

The pilot-scope coverage report (`docs/superpowers/reports/2026-08-07-external-terminology-resource-portfolio/coverage-report.json`)
against the seed release reports `ready=false`: coverage 83.33% meets the
80% target, but risk terms `brightness`/`contrast` are not covered, the
bilingual backlog (`kw:brightness` missing `en`, `kw:contrast` missing `zh`)
is recorded, and approval remains `operator_required`. Per the acceptance
criterion, production seed activation requires the operator to review the
backlog, approve the acceptance criteria, and publish a versioned seed
release — tooling reports readiness and cannot self-approve.

## 14. Recommended implementation sequence

## 13. Recommended implementation sequence

1. Measure the pilot corpus and select the seed scope.
2. Design and migrate the structured source-policy registry.
3. Add adapter conformance fixtures and fail-closed tests.
4. Extend the QUDT importer to preserve multilingual and relation semantics.
5. Add the SIRP 1.0.0 adapter and exact SIRP–QUDT crosswalk handling.
6. Add UCUM unit validation without merge authority.
7. Build the curated IEV seed workflow and luminance/brightness acceptance fixture.
8. Add the revision-pinned Wikidata proposal index.
9. Publish the first versioned seed release and enable it for Tier-6 validation.
10. Evaluate coverage and false proposals before adding LOINC or another domain source.

## 15. Primary-source index

- BIPM, [SI Reference Point 1.0.0 release](https://www.bipm.org/en/-/2026-06-18-the-first-stable-version-of-the-si-reference-point) and [service](https://si-digital-framework.org/SI)
- BIPM, [SI Brochure, 9th edition, updated 2026](https://www.bipm.org/en/publications/si-brochure/)
- QUDT, [Quantity Kind Vocabulary 3.5.0](https://www.qudt.org/doc/DOC_VOCAB-QUANTITY-KINDS.html) and [catalog](https://www.qudt.org/catalog/qudt-catalog.html)
- IEC, [Electropedia](https://www.electropedia.org/) and [copyright policy](https://webstore.iec.ch/en/copyright)
- CIE, [e-ILV](https://www.cie.co.at/e-ilv), [luminance](https://www.cie.co.at/eilvterm/17-21-050) and [brightness](https://www.cie.co.at/eilvterm/17-22-059)
- ISO, [OBP terms of use](https://www.iso.org/fr/footer-links/obp-tems-of-use.html)
- UCUM, [specification](https://ucum.org/ucum), [internationalization](https://ucum.org/docs/internationalization) and [license](https://ucum.org/license)
- Wikidata, [licensing](https://www.wikidata.org/wiki/Wikidata:Licensing), [data access](https://www.wikidata.org/wiki/Wikidata:Data_access/en) and [database downloads](https://www.wikidata.org/wiki/Wikidata:Database_download)
- LOINC, [release notes](https://loinc.org/kb/loinc-release-notes), [mapping guidance](https://loinc.org/kb/faq/mapping/) and [license](https://loinc.org/kb/license)
- SNOMED International, [release formats](https://docs.snomed.org/snomed-ct-practical-guides/snomed-ct-starter-guide/13-release-schedule-and-file-formats) and [licensing](https://www.snomed.org/licensing)
- U.S. National Library of Medicine, [UMLS Metathesaurus](https://www.nlm.nih.gov/research/umls/knowledge_sources/metathesaurus/index.html) and [2026AA downloads](https://www.nlm.nih.gov/research/umls/licensedcontent/umlsknowledgesources.html)
- NCI EVS, [NCI Thesaurus terms of use](https://evs.nci.nih.gov/ftp1/NCI_Thesaurus/ThesaurusTermsofUse.htm)
- OBO Foundry, [PATO](https://obofoundry.org/ontology/pato.html)
- IUPAC, [Gold Book](https://goldbook.iupac.org/) and [API](https://goldbook.iupac.org/pages/api)
- European Commission, [IATE](https://iate.europa.eu/) and [EMDN](https://health.ec.europa.eu/medical-devices-topics-interest/european-medical-devices-nomenclature-emdn_en)
- IMDRF, [adverse-event terminology releases](https://www.imdrf.org/documents/terminologies-categorized-adverse-event-reporting-aer-terms-terminology-and-codes)
- CC-CEDICT, [download and license](https://cc-cedict.org/editor/editor.php?handler=Download)
- Open Multilingual WordNet, [project overview](https://omwn.org/)

Licensing assessments in this document are engineering risk classifications, not legal advice. A source must receive a use-case-specific license review before production import or redistribution.
