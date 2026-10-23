# Category-index experiment (metrics)

Can a category tree replace a vector database for metric search? The idea: file every metric
under categories, resolve a search query to categories too, and compute cosine only over the
metrics in those categories (capped, e.g. at 5,000), so no vector index has to fit in memory.

This folder is the offline test of that idea, run on 2026-10-23 against the 2,660 metrics then in
`kb.metrics` (16 documents). It reads the database and calls models, but writes nothing to `kb`.
It is exploratory, not a frozen benchmark: the facet tree is a draft and the queries are written
by an LLM.

Background: devdoc `2026102003-devdoc-categories.md` (category trees) in KnowledgeStore.

## Design under test

- **Domain:** the ICS tree loaded as `cat:general-tech-categories`. A metric is filed by the
  *thing* it is about, not by its document: a ventilator display's luminance goes under
  *31.120 Electronic display devices*. The *01.040 Vocabularies* subtree is left out.
- **Facet:** what the metric measures, from a draft tree in `facets.json` (17 groups, 70 leaves),
  seeded from the 477 category keys the extractor had produced.
- **Binding:** a metric gets a (domain node, facet node) pair, so the facet tree is not copied
  under every ICS leaf. Each pair is one index cell.

## Results

Known-item search over 212 queries. Each query was written by an LLM to find one specific
metric, in its own words, alternating English and Chinese. A metric counts as found if it is the
target or a near-duplicate of it (cosine ≥ 0.95; GB 43150 appears three times).

Each query is handled in three steps:

1. **Resolve the query to nodes.** The query is filed like a metric: it gets a set of ICS nodes
   and a set of facet nodes, by the same method as the metrics in that row.
2. **Filter.** A metric stays in the candidate set C when it passes the domain test and, for the
   "pair" strategy, also the facet test. ("Domain only" and "facet only" strategies apply one
   test.) A metric passes a test when one of its nodes and one of the query's nodes are on the
   same branch of the tree:
   - the metric's node is the query's node or lies below it (the query is broader), or
   - the metric's node lies above the query's node (the metric was filed more coarsely, e.g.
     stopped at an inner node's "others").

   Siblings and cousins do not match: a metric filed under *17.100 Pressure measurement* fails a
   query filed under *11.040.55 Diagnostic equipment*. Two special cases: a query that got no
   node on a side is not filtered on that side; a metric that got no node on a side counts as
   filed at the root and passes every query (this inflates the rows where many metrics are
   unresolved). The `dL1` and `dL2` variants in `results/` first replace every ICS node by its
   ancestor at level 1 or 2, so the test becomes "same Field" or "same Group".
3. **Rank.** The metrics in C are ranked by cosine similarity between the query's bge-m3
   embedding and each metric's existing embedding in `kb.search_artifacts_metric`.

The columns:

- **cell recall**: the share of queries for which C still contains the target (or a
  near-duplicate). It measures only the filter, before any ranking. A target missing from C can
  never be returned however good the ranking is, so cell recall is the ceiling for everything
  after it. Brute force has cell recall 1.0 by definition.
- **R@10**: the share of queries whose target is among the top 10 of C after step 3. The ranking
  is pure vector search: cosine only, with no BM25 and no fusion. The brute-force baseline is the
  same ranking with C = all metrics. The BM25 baseline is pure BM25 over all metrics (the `|||`
  operator the search API uses), with no vectors and no category filter. A hybrid of the two was
  not tested.
- **|C|**: the size of the candidate set, as the median over queries; `results/` also gives the
  90th percentile. It shows how much cosine work the filter saves.

Query nodes come from the LLM (filing method of that row) unless a row in `results/` is marked
`emb@k`, where they are the k nearest tree nodes by embedding, without any LLM.

The metric fields the LLM sees are name, English name, subject, English subject, description,
context, unit, value, condition and document title. `kb.metrics.frame` was added afterwards
(2026-10-23), so these results do not include it; on that date only 23 of 2,660 metrics had a
frame.

| Domain filing (metrics and queries) | No domain | Domain cell recall | Pair cell recall | Pair R@10 | Pair \|C\| |
|---|---|---|---|---|---|
| Original ICS labels, embedding shortlist + LLM check | 34% | (inflated) | 0.68 | 0.67 | 71 |
| ICS with alternative labels, same method | 12% | (inflated) | 0.68 | 0.65 | 64 |
| Level-by-level LLM walk, one best child | 0% | 0.68 | 0.59 | 0.56 | 20 |
| **Level-by-level LLM walk, up to 3 children** | 0% | **0.97** | **0.86** | **0.83** | 62 |
| Same, model reasoning off | 0% | 0.93 | 0.84 | 0.80 | 80 |

Metrics with no domain match every query, which inflates the domain figures in the first two
rows. Baselines: brute-force cosine R@10 0.95, BM25 0.45 (0.16 for English queries, since most
metrics are Chinese). Facets used verification v2 (several facets allowed) in every row except
the first.

At a fixed share of the corpus scanned (`results/budget_walk2_v2.txt`):

| Method | 2% | 5% | 10% | 20% |
|---|---|---|---|---|
| Category pair, walk with up to 3 children | 0.72 | 0.82 | 0.87 | 0.89 |
| k-means IVF, k=100 (blind clustering) | 0.81 | 0.93 | 0.95 | 0.95 |

What we learned:

1. **The flat embedding shortlist fails on ICS.** Short names hit the wrong words in formal
   titles ("blood pressure monitor" → *Pressure vessels*). Alternative labels listing the
   typical things under each node helped, but not enough.
2. **The walk works, and is cheap once reasoning is off.** One LLM call per parent node, with
   items batched, came to about 0.5 calls per metric. DeepSeek Flash reasons unless the request
   sends `thinking: disabled`, and every row above except the last ran with reasoning on: the
   metric walk alone used 3.7M completion tokens, about 99% of them hidden reasoning, and a
   call took about 70 s. With reasoning off, the walk and facet filing together used 0.26M
   completion tokens (about 100 per metric) and a call took about 2 s. The prompt tokens
   (about 2,000 per metric) are unchanged and are now the main cost. Reasoning bought 4 to 5
   points of domain cell recall. Without it, picks are more often swayed by the document's
   topic (a container rain-test metric in a waste-treatment standard filed under waste
   installations).
3. **A metric needs several ICS homes.** ICS classifies one thing by product, technique,
   component and use. Allowing up to three children per level raised domain cell recall from
   0.68 to 0.97.
4. **The facet side is now weaker (cell recall 0.89).** The draft mixes the quantity measured
   (pressure, concentration) with the metric's role (safety limit, calibration, clinical score).
   Splitting it into two facets is the next fix.
5. **Queries need the walk too.** Routing a query by embedding alone gives pair cell recall
   0.48. The walk costs three sequential LLM rounds per query.
6. **At the same scan budget, blind k-means did better.** Queries written from the metric's own
   text favour embedding methods, so human queries may narrow the gap.

Not tested: cell sizes at millions of metrics (16 documents are too few to extrapolate),
queries written by people, a hybrid of ANN search with category filters or boosts.

## Layout

| Path | Contents |
|---|---|
| `facets.json` | Draft facet tree |
| `lib.py` | Database, LLM, embedding and cache helpers |
| `s1_data.py` | Dumps metrics (with their existing bge-m3 embeddings) and both trees; embeds tree nodes |
| `s1b_enrich_ics.py` | ICS v2: LLM-written alternative labels per node, re-embedded |
| `s2_candidates.py` | LLM domain and facet candidates per metric |
| `s3_file.py` | Filing by embedding shortlist + LLM verification |
| `s3b_walk.py` | Filing by level-by-level LLM walk over ICS v2 |
| `s3c_facets.py` | Facet verification v2 (all applicable facets) |
| `s4_queries.py` | Query generation and query candidates |
| `s5_eval.py` | Main evaluation |
| `s6_misses.py` | Why the pair filter drops targets |
| `s7_budget.py` | Recall at a fixed scan budget vs k-means IVF |
| `datasets/queries-v1.json` | The 212 queries and their target metric IDs |
| `datasets/ics-alt-labels-v1.jsonl` | The LLM-written ICS alternative labels |
| `results/` | Evaluation outputs (file names give metric domain, query domain, facet version) |
| `out/` | Working data, LLM caches and embeddings (gitignored, about 34 MB) |

Prompts are in `ChenWeb/prompts/prompt-catidx-*.md`.

## Run

Needs the ChenWeb database, `.models.toml` (DeepSeek Flash, local bge-m3 on port 18083) and
`uv`. Run from this directory with the ChenWeb `PG_*` variables set:

```sh
uv venv .venv && uv pip install --python .venv/bin/python numpy "psycopg[binary]" httpx
eval "$(cd ../.. && mise env | grep -E '^export PG_')"
.venv/bin/python s1_data.py
.venv/bin/python s1b_enrich_ics.py
.venv/bin/python s2_candidates.py
.venv/bin/python s4_queries.py
.venv/bin/python s3_file.py queries && .venv/bin/python s3_file.py metrics
EXP_WALK=v2 .venv/bin/python s3b_walk.py queries && EXP_WALK=v2 .venv/bin/python s3b_walk.py metrics
.venv/bin/python s3c_facets.py queries && .venv/bin/python s3c_facets.py metrics
EXP_MDOMAIN=walk_v2 EXP_QDOMAIN=walk_v2 EXP_FACET=v2 .venv/bin/python s5_eval.py
EXP_MDOMAIN=walk_v2 EXP_QDOMAIN=walk_v2 EXP_FACET=v2 .venv/bin/python s7_budget.py
```

LLM results are cached per item in `out/*.jsonl`, so a rerun only calls models for new items;
delete a cache file to redo that step. Options: `EXP_ICS=ics2` (s3_file.py, enriched ICS),
`EXP_WALK=v1|v2` (s3b_walk.py), and for the evaluation `EXP_MDOMAIN` and `EXP_QDOMAIN`
(`v1`, `ics2`, `walk`, `walk_v2`, `walk_v2_nothink`) and `EXP_FACET` (`v1`, `v2`, `v2_nothink`).

`EXP_THINKING` controls model reasoning: `disabled` (the default since 2026-10-23) sends
`thinking: disabled` and tags output files `_nothink`; `default` sends nothing, so the model
reasons. The untagged files in `out/` and `results/` were produced with reasoning on.
