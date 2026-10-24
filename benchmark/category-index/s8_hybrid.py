"""Hybrid search (BM25 + vector, RRF) as production runs it, on testbed copies of the metric search rows.

Variants (one testbed table each, rebuilt on every run):
  base    the current kb.search_artifacts_metric search_document and embedding
  facets  search_document + kb.metrics.metric_categories(_en) keys (underscores as spaces), re-embedded
The query is the ParadeDB hybrid SQL of server/api/kbhandler/search_registry.go (semantic CTE with
ORDER BY distance LIMIT, iterative HNSW scan), so BM25-only, vector-only, hybrid and candidate reach all
come from the same candidate lists. Usage: s8_hybrid.py
"""
import json

import numpy as np

from lib import OUT, db, embed, log

RRF_K, CAND = 60, 200  # rrfK and hybridCandidateLimit in search_registry.go
DUP_COS, K = 0.95, 10  # same relevance rule as s5_eval.py
VARIANTS = ("base", "facets")
LEX_WEIGHTS = (1.0, 0.5, 0.25)  # weight of the BM25 term in RRF; production uses 1.0
BM25_FIELDS = ("search_document", "primary_label", "secondary_label", "snippet_basis", "source_title", "source_filename")

metrics = json.loads((OUT / "metrics.json").read_text())
pos = {m["id"]: i for i, m in enumerate(metrics)}
memb = np.load(OUT / "metric_emb.npy")
queries = json.loads((OUT / "queries.json").read_text())
qemb = embed([q["query"] for q in queries])


def facet_text(raw):
    try:
        keys = json.loads(raw or "[]")
    except json.JSONDecodeError:
        keys = (raw or "").split(",")
    return " ".join(str(k).replace("_", " ").strip() for k in keys if str(k).strip())


def build(c, variant):
    t = f"testbed.catidx_hybrid_{variant}"
    with c.transaction():
        _build(c, t, variant)
    log(variant, "rows:", c.execute(f"SELECT count(*) FROM {t}").fetchone()[0])
    return t


def _build(c, t, variant):
    c.execute(f"DROP TABLE IF EXISTS {t}")
    c.execute(f"""CREATE TABLE {t} AS
        SELECT sa.artifact_type, sa.artifact_id, m.id::text AS metric_pk, sa.search_document, sa.primary_label,
               sa.secondary_label, sa.snippet_basis, sa.source_title, sa.source_filename, sa.embedding,
               coalesce(m.metric_categories_en, m.metric_categories, '') AS cats
        FROM kb.search_artifacts_metric sa JOIN kb.metrics m ON m.metric_id = sa.artifact_id
        WHERE m.id = ANY(%s)""", ([int(m["id"]) for m in metrics],))
    if variant == "facets":
        rows = c.execute(f"SELECT artifact_id, search_document, cats FROM {t} ORDER BY artifact_id").fetchall()
        docs = [(aid, (doc + " " + facet_text(cats)).strip()) for aid, doc, cats in rows]
        vecs = embed([d for _, d in docs])
        with c.cursor() as cur:
            cur.executemany(f"UPDATE {t} SET search_document = %s, embedding = %s::vector WHERE artifact_id = %s",
                            [(d, json.dumps(v.tolist()), aid) for (aid, d), v in zip(docs, vecs)])
    c.execute(f"ALTER TABLE {t} ADD PRIMARY KEY (artifact_id)")
    fields = ", ".join(f'"{f}": {{"tokenizer": {{"type": "jieba"}}}}' for f in BM25_FIELDS)
    c.execute(f"""CREATE INDEX ON {t} USING bm25 (artifact_id, {", ".join(BM25_FIELDS)})
        WITH (key_field = artifact_id, text_fields = '{{{fields}}}')""")
    c.execute(f"CREATE INDEX ON {t} USING hnsw (embedding vector_cosine_ops)")


def search(c, t, qtext, qvec):
    lexical = " OR ".join(f"sa.{f} ||| %(q)s" for f in BM25_FIELDS)
    sql = f"""
WITH lexical AS (
    SELECT sa.artifact_id, ROW_NUMBER() OVER (ORDER BY pdb.score(sa.artifact_id) DESC, sa.artifact_id ASC) AS rnk
    FROM {t} sa WHERE ({lexical}) ORDER BY rnk LIMIT {CAND}
),
semantic AS (
    SELECT nn.artifact_id, ROW_NUMBER() OVER (ORDER BY nn.distance ASC, nn.artifact_id ASC) AS rnk
    FROM (SELECT sa.artifact_id, sa.embedding <=> %(v)s::vector AS distance FROM {t} sa
          WHERE sa.embedding IS NOT NULL ORDER BY sa.embedding <=> %(v)s::vector LIMIT {CAND}) nn
)
SELECT sa.metric_pk, l.rnk, s.rnk,
       COALESCE(1.0 / ({RRF_K} + l.rnk), 0.0) + COALESCE(1.0 / ({RRF_K} + s.rnk), 0.0) AS score
FROM lexical l FULL OUTER JOIN semantic s ON l.artifact_id = s.artifact_id
JOIN {t} sa ON sa.artifact_id = COALESCE(l.artifact_id, s.artifact_id)
ORDER BY score DESC, sa.artifact_id ASC"""
    with c.transaction():
        c.execute("SET LOCAL hnsw.iterative_scan = relaxed_order")
        return c.execute(sql, {"q": qtext, "v": json.dumps(qvec.tolist())}).fetchall()


def hit_rr(ranked, rel):
    for r, mid in enumerate(ranked[:K]):
        if pos.get(mid) in rel:
            return 1, 1 / (r + 1)
    return 0, 0


results = {}
with db() as c:
    c.autocommit = True
    for variant in VARIANTS:
        t = build(c, variant)
        rows = []
        for qi, q in enumerate(queries):
            tgt = pos[q["target"]]
            rel = set(np.nonzero(memb @ memb[tgt] >= DUP_COS)[0]) | {tgt}
            res = search(c, t, q["query"], qemb[qi])
            lex = [r[0] for r in sorted((r for r in res if r[1]), key=lambda r: r[1])]
            sem = [r[0] for r in sorted((r for r in res if r[2]), key=lambda r: r[2])]
            reach = int(any(pos.get(r[0]) in rel for r in res))
            hyb = []
            for w in LEX_WEIGHTS:
                fused = sorted(res, key=lambda r: -((w / (RRF_K + r[1]) if r[1] else 0) + (1 / (RRF_K + r[2]) if r[2] else 0)))
                hyb.append(hit_rr([r[0] for r in fused], rel))
            rows.append((q["lang"], hit_rr(lex, rel), hit_rr(sem, rel), *hyb, reach, len(lex)))
        results[variant] = rows

print(f"queries: {len(queries)}  metrics: {len(metrics)}  candidates per list: {CAND}  RRF k: {RRF_K}\n")
print(f"{'variant':8} {'method':10} {'R@10':>6} {'MRR':>6} {'R@10 en':>8} {'R@10 zh':>8}")
for variant, rows in results.items():
    for j, name in ((1, "bm25"), (2, "vector"), *((3 + i, f"rrf w={w}") for i, w in enumerate(LEX_WEIGHTS))):
        r10 = np.mean([r[j][0] for r in rows])
        mrr = np.mean([r[j][1] for r in rows])
        by = {lg: np.mean([r[j][0] for r in rows if r[0] == lg]) for lg in ("en", "zh")}
        print(f"{variant:8} {name:10} {r10:6.3f} {mrr:6.3f} {by['en']:8.3f} {by['zh']:8.3f}")
    print(f"{variant:8} {'reach':10} {np.mean([r[-2] for r in rows]):6.3f}   (target among the lexical + semantic candidates)"
          f"   median BM25 matches: {int(np.median([r[-1] for r in rows]))}")
