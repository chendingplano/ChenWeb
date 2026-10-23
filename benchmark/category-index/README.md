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

- **cell recall**: share of queries whose cells still contain the target;
- **R@10**: target in the top 10 after ranking by cosine;
- **|C|**: metrics left after filtering (median).

| Domain filing (metrics and queries) | No domain | Domain cell recall | Pair cell recall | Pair R@10 | Pair \|C\| |
|---|---|---|---|---|---|
| Original ICS labels, embedding shortlist + LLM check | 34% | (inflated) | 0.68 | 0.67 | 71 |
| ICS with alternative labels, same method | 12% | (inflated) | 0.68 | 0.65 | 64 |
| Level-by-level LLM walk, one best child | 0% | 0.68 | 0.59 | 0.56 | 20 |
| **Level-by-level LLM walk, up to 3 children** | 0% | **0.97** | **0.86** | **0.83** | 62 |

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
2. **The walk works and is cheap.** One LLM call per parent node, with items batched, came to
   about 0.5 calls per metric.
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
(`v1`, `ics2`, `walk`, `walk_v2`) and `EXP_FACET` (`v1`, `v2`).
