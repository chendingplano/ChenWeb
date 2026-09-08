## 1. Product Metric Reviewer — plain-language overview

This is the human entry point to the Product Metric Reviewer change. It explains what the app
does and how it works in prose, then points you at the formal specs in the order that makes
them readable. Technical names are kept out of the body on purpose and collected in the
reference table at the end.

## 2. What the app does

Give it a product — a name like *ventilator*, optionally with a short description — and it
returns every metric in the knowledge base that applies to that product. Not just metrics with
the product's name on them, but the metrics of the product's parts, and the metrics that
govern the product across its life: storage, transport, maintenance, the environment it runs
in, safety, packaging, disposal. Each metric comes back with the document it was found in, the
exact lines, and a sentence saying why it was included.

## 3. Why a plain search doesn't work

Ask "which metrics apply to a ventilator?" and matching that against metric text finds almost
nothing useful. A product is a nested thing — a product made of modules, made of parts, made
of smaller parts — and most metrics are attached to a part, not to the product. *Display
luminance* is a metric of the display. *Battery endurance* is a metric of the battery.
*Leakage current* is a metric of the power supply. None of those mention the word *ventilator*,
so a name search misses all of them.

The same query misses the other half of the answer: the conditions the product has to satisfy
over its lifetime. A storage-temperature limit or a drop-test requirement is about the
ventilator, but its subject is a condition, not a part, so again the name never appears.

Everything needed to bridge that gap is already in the knowledge base — which products each
document is about, each metric's subject, the reconciled part hierarchy, and a
keyword-plus-meaning search index over everything. What was missing is something that composes
them into one product-scoped answer. That is this app.

## 4. How it works, walked through with a ventilator

### 4.1. It builds a picture of the product

You type *ventilator*. The app turns that into a **scope profile** — a small tree describing
the product:

- a **root** for the product itself;
- **parts** under it — display, blower, breathing circuit, battery, oxygen sensor, and so on,
  nested as deep as the breakdown goes;
- **aspects** attached to the root — storage, transport, maintenance, usage environment,
  safety — drawn from a fixed, translatable list in configuration.

The tree is built in three passes. 

First, a single model call proposes the breakdown from the
name and description. 

Then each proposed part is **grounded**: the app tries to match its name
to a **reconciled object** the knowledge base already tracks across documents (a *display*, a
*battery*), or failing that to a **subject concept** from the governed vocabulary that every
metric's subject is tagged with, so that later steps can match by identity instead of by
spelling. A part that matches nothing stays in the tree and is matched by text only. 

Finally the app **expands** the tree: starting from each grounded part, it follows recorded *part-of*
relationships between those reconciled objects to pull in real parts the model never named.
Those relationships are a separate graph the knowledge base already holds; it is usually thin
for a fresh product, so this pass often adds little.

Then you curate it — accept, reject, edit, add, delete nodes. The profile is saved, and every
edit bumps its version. It is reusable: the same ventilator profile serves many runs, and a
close relative like an anaesthesia machine can start from a copy. A finished run remembers the
exact profile version it used, so later edits never change past results.

**A worked example.** You enter *Ventilator*, described as "ICU mechanical ventilator, mains
and battery powered." The three passes might produce this tree:

```
Ventilator                       — reconciled object "ventilator"          (root)
├─ Breathing circuit             — reconciled object "breathing circuit"
│  ├─ Inspiratory limb           — ungrounded, matched by text only
│  └─ Expiratory valve           — subject concept "expiratory valve"
├─ Blower / turbine              — reconciled object "blower"
├─ Display                       — reconciled object "display unit"
├─ Battery                       — reconciled object "battery pack"
├─ Oxygen sensor                 — subject concept "oxygen concentration"
└─ Humidifier                    — added by the expand pass, not proposed

aspects on the root (from configuration):
  storage · transport · maintenance · usage environment · safety
```

The model proposed every branch except *Humidifier*. Grounding tied most branches to
identities the knowledge base already knows — so a metric recorded against "display unit" in
any document is now reachable by identity, not by the word "display" — while *Inspiratory
limb* matched nothing and will be found only by its text. The expand pass added *Humidifier*
because the part-of graph records it as part of a ventilator, even though the model's
breakdown missed it.

Curating, you might reject *Inspiratory limb* as too fine-grained, rename *Blower / turbine*
to *Blower*, and add *Air filter* by hand — each edit bumping the profile version. The next
run uses the curated tree; a run that already finished keeps the version it ran against.

### 4.2. It finds the metrics, two ways, and combines them

It retrieve metrics in two ways and then combines the results.

**By document.** The app scores every document by how well it matches something in the
profile — the product, a part, or an aspect. A document scores if the knowledge base already
records it as being *about* one of those products (and how: a document whose whole scope is the
ventilator scores higher than one that merely mentions it), or if a keyword-plus-meaning
search over document summaries and topics ties it to a node. Standards, specifications, and
regulations get a score boost so they rise to the top — but it is only a boost, never a
filter, so a service manual that matches still comes through, just lower. Then the app takes
the metrics of every document that made the cut.

**By subject.** Independently, the app takes every metric whose subject *is* one of the
profile's nodes — matched either by exact identity, or by a keyword-plus-meaning search using
the node's name. This route ignores which document the metric sits in. That is deliberate: it
is how a *display luminance* figure buried in a third-party screen datasheet — a document that
never says *ventilator* — still reaches the answer.

A metric found by both routes appears once, and remembers that both routes found it.

### 4.3. It labels and explains every result

Each metric gets one **tier**, decided by *what it matched*, not by which route found it:

- **product** — its subject is the ventilator itself;
- **part** — its subject is a part or module;
- **aspect** — its subject or context is a lifecycle aspect;
- **from a scoped document** — it matched no node, but it lives in a document that was in
  scope.

Every result also carries the node it matched, the routes that found it, a score, and a
readable reason — naming the node and the evidence, or, for the last tier, naming the document
and why the document was in scope. A metric whose subject is the display is always in the
*part* tier, even if the only reason it surfaced is that its document was in scope.

### 4.4. It keeps the volume sane

A single standard can carry hundreds of metrics, most of them landing in the "from a scoped
document" tier. The app caps how many metrics one document may contribute and how many a whole
run returns. When a cap bites, it keeps the highest-scoring, records how many it dropped, and
never discards a product / part / aspect result to make room for a document-only one.

### 4.5. It reports coverage — and gaps

Alongside the results, the run produces a report: a table of how many metrics and documents
each node got, and — often the more useful half — an explicit list of the nodes that got
**nothing**. For someone assembling a conformance file, "no metric found for the humidifier
module" is a finding, not a blank. The node's grounding state tells you whether that gap is a
real hole in the corpus or just a part the app couldn't pin down.

### 4.6. You can re-run it

Runs are repeatable and comparable. Re-run the same request a month later and the app diffs
the new run against the last one: what metrics are new, what's gone, what changed tier.
Because each run pins its profile version, the diff can tell "the corpus changed" apart from
"the scope changed."

## 5. One promise: it's repeatable

The only place the app calls a language model is the initial breakdown of the product (plus,
optionally, mapping an unusual aspect). Everything after that — scoring, matching, tiering, the
report — is plain database work. Given the same profile version, the same documents, and the
same configuration, two runs produce the same results with the same tiers.

## 6. What it deliberately does not do

- It never runs a new extraction pass and never writes to the knowledge base's shared tables.
  It only reads what other pipelines produced, and writes only its own profiles, runs, and
  results.
- It doesn't compare metrics or resolve conflicts between them — that's a different machine's
  job.
- It handles one product family per profile.
- Version 1 finds metrics only. It is built so that provisions, inventory items, or test
  methods can be added later as a small adapter plus configuration, but none of that is in
  scope now.

## 7. The formal documents, and the order to read them

The change is split across several files because OpenSpec models each "capability" as its own
spec file. Read them in this order:

1. **This overview** — the whole app in prose.
2. **proposal.md** — the problem, and a bullet list of what changes. Short.
3. **design.md** — the eight design decisions (D1–D8) and their trade-offs: why the profile is
   persisted and versioned, why the three-pass build, why aspects join to relation types, why
   two retrieval routes, why the results key is what it is.
4. **specs/product-scope-profile** — building and curating the product breakdown: the tree
   model, the propose / ground / expand passes and their limits, the aspect vocabulary,
   curation and versioning.
5. **specs/product-artifact-retrieval** — the heart: scoring documents, the two retrieval
   routes, how results are merged, tiered, scored, and explained. Read the profile spec first;
   this one uses its vocabulary.
6. **specs/product-metric-review-runs** — the request-and-run lifecycle, how results and the
   scoped-document set are stored, the coverage-and-gap report, and re-run-and-diff.
7. **specs/product-metric-reviewer-page** — the screen at `/home3/product-metric-review`: the
   scope-tree editor, the results explorer, the evidence drawer, the report tab.
8. **tasks.md** — the implementation checklist.

## 8. Reference: the names the specs use

| In this document | In the specs and code |
|---|---|
| the scope profile / the product breakdown | `kb.product_profiles` (the tree) + `kb.product_profile_nodes` (each node) |
| a node's kind | `node_kind` ∈ `product`, `module`, `part`, `aspect` |
| where a node came from | `origin` ∈ `llm_proposed`, `graph_expanded`, `user_added` |
| a node is grounded | grounding tries three id spaces in order and keeps the first hit: `object_id` (from `kb.object_nodes`), then `concept_id` (from `kb.keyword_concepts`, scope `metric_subject`), then `term_id` (from `kb.ontology_terms`); no hit → `grounding = ungrounded`, matched by text only. Grounding runs on the node's label, never on a metric row. |
| the metrics | rows in `kb.metrics` |
| a metric's subject identity | `kb.metrics.subject_concept_id` — the same `kb.keyword_concepts` id space a node's `concept_id` lives in, which is why the "by subject" route can compare them for equality |
| a metric linked to a reconciled object | via `kb.artifact_objects`, which bridges an artifact `(artifact_type, artifact_id)` to an `object_id`; used for document scoping and expansion, not for the "by subject" match |
| the metric search index | the `metric` partition of `kb.search_artifacts` (`kb.search_artifacts_metric`) — a lexical `tsvector` plus a vector `embedding` per metric |
| keyword-plus-meaning search | reciprocal-rank fusion (RRF) of lexical and vector matches, via the `kbsearch` helpers |
| the "by document" route | Path D — `kb.metrics` filtered to `input_record_id` in the scoped document set |
| the "by subject" route | Path E — `subject_concept_id` equals a node's concept, or RRF over the metric index by node label; not restricted to scoped documents |
| which products a document is about | `kb.products`, one row per product mention, with `relation_type` (`scope`, `regulated_object`, `requirement_target`, `component_of`, `storage_requirement`, …) |
| the standards boost | `document.doc_kind` in `kb.doc_facet_values` (`da:doc_kind_standard` and kin), falling back to `kb.doc_facets.input_doc_type`; a boost, never a filter |
| the part graph | accepted `kb.semantic_assertions` with predicate `core:part_of` or `core:component_of` |
| the four tiers (product / part / aspect / from a scoped document) | `direct`, `part`, `aspect`, `document_scope` |
| a result's id | the pair `(artifact_type, artifact_id)`; for a metric, `artifact_id` is `<document record id>_mtc_<seq>`, built by `kbsearch.BuildArtifactID` |
| a request / a run | `kb.product_review_requests` (intent) / `kb.product_review_runs` (one execution, carries the report) |
| the requested kinds | `artifact_types` on the request — version 1 accepts only `["metric"]` |
| error codes | `CWB_KB_PMR_0xx` — e.g. `010` cycle in the tree, `011` proposal call failed, `020` unregistered artifact type, `030` run against a draft profile |
| the page | `/home3/product-metric-review` |
