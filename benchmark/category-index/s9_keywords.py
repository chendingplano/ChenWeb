"""BM25 with LLM-extracted query keywords instead of matching any word of the raw query.

Runs on testbed.catidx_hybrid_base (built by s8_hybrid.py). BM25 query forms:
  raw          the raw query with ||| (match any word; what production does)
  kw-any       the keywords' words (English and Chinese) with ||| (match any)
  kw-each      each keyword must match fully (&&&, all its words in one field); keywords OR-ed, so
               metrics matching more keywords score higher
Each form is fused with the same vector top 200 by RRF (k = 60) at several BM25 weights.
Usage: s9_keywords.py
"""
import json
import os
import time
from concurrent.futures import ThreadPoolExecutor

import numpy as np

from lib import CHENWEB, OUT, batched_llm, db, embed, llm_json, log, prompt

TABLE = "testbed.catidx_hybrid_base"
RRF_K, CAND, DUP_COS, K = 60, 200, 0.95, 10
LEX_WEIGHTS = (1.0, 0.5, 0.25)
FIELDS = ("search_document", "primary_label", "secondary_label", "snippet_basis", "source_title", "source_filename")

metrics = json.loads((OUT / "metrics.json").read_text())
pos = {m["id"]: i for i, m in enumerate(metrics)}
memb = np.load(OUT / "metric_emb.npy")
queries = json.loads((OUT / "queries.json").read_text())
qemb = embed([q["query"] for q in queries])

# EXP_KW_PROMPT=prod uses the production prompt (prompts/prompt-search-query-keywords-v1.md) the way
# search does: one plain-text query per call, {"keywords": [...]} back. Default: the batched benchmark prompt.
# EXP_KW_PROMPT=bench1: the benchmark prompt, but one query per call (isolates batching from wording).
MODE = os.environ.get("EXP_KW_PROMPT", "")
PROD = MODE == "prod"
if MODE in ("prod", "bench1"):
    if PROD:
        sys_prompt = (CHENWEB / "prompts" / "prompt-search-query-keywords-v1.md").read_text()
        ask = lambda q: llm_json(sys_prompt, q["query"])[0]
    else:
        sys_prompt = prompt("query-keywords-v1.md")
        ask = lambda q: next(iter(llm_json(sys_prompt, {"items": [{"id": q["id"], "query": q["query"]}]})[0].get("items", [])), {})
    cache = OUT / f"query_keywords_{MODE}{os.environ.get('EXP_KW_RUN', '')}.json"
    kw = json.loads(cache.read_text()) if cache.exists() else {}
    todo = [q for q in queries if q["id"] not in kw]
    with ThreadPoolExecutor(12) as ex:
        for q, res in zip(todo, ex.map(ask, todo)):
            kw[q["id"]] = res
    cache.write_text(json.dumps(kw, ensure_ascii=False))
    t = time.time()
    ask(queries[0])
else:
    kw = batched_llm("query_keywords", prompt("query-keywords-v1.md"), queries, 15,
                     lambda b: {"items": [{"id": q["id"], "query": q["query"]} for q in b]})
    t = time.time()
    llm_json(prompt("query-keywords-v1.md"), {"items": [{"id": "x", "query": queries[0]["query"]}]})
single_call_s = time.time() - t


def keywords(q):
    out = []
    for k in kw.get(q["id"], {}).get("keywords", []) or []:
        for lang in ("en", "zh"):
            s = str(k.get(lang) or "").strip()
            if s and s not in out:
                out.append(s)
    return out


def lexical(c, form, q):
    if form == "raw":
        params = {"q0": q["query"]}
        where = " OR ".join(f"sa.{f} ||| %(q0)s" for f in FIELDS)
    else:
        kws = keywords(q) or [q["query"]]
        if form == "kw-any":
            params = {"q0": " ".join(kws)}
            where = " OR ".join(f"sa.{f} ||| %(q0)s" for f in FIELDS)
        else:
            params = {f"q{i}": k for i, k in enumerate(kws)}
            where = " OR ".join(f"sa.{f} &&& %(q{i})s" for i in range(len(kws)) for f in FIELDS)
    rows = c.execute(f"""SELECT sa.metric_pk FROM {TABLE} sa WHERE ({where})
        ORDER BY pdb.score(sa.artifact_id) DESC, sa.artifact_id ASC LIMIT {CAND}""", params).fetchall()
    n = c.execute(f"SELECT count(*) FROM {TABLE} sa WHERE ({where})", params).fetchone()[0]
    return [r[0] for r in rows], n


def semantic(c, v):
    with c.transaction():
        c.execute("SET LOCAL hnsw.iterative_scan = relaxed_order")
        return [r[0] for r in c.execute(f"""SELECT metric_pk FROM {TABLE} WHERE embedding IS NOT NULL
            ORDER BY embedding <=> %s::vector LIMIT {CAND}""", (json.dumps(v.tolist()),)).fetchall()]


def hit_rr(ranked, rel):
    for r, mid in enumerate(ranked[:K]):
        if pos.get(mid) in rel:
            return 1, 1 / (r + 1)
    return 0, 0


def fuse(lex, sem, w):
    score = {}
    for r, m in enumerate(lex, 1):
        score[m] = score.get(m, 0) + w / (RRF_K + r)
    for r, m in enumerate(sem, 1):
        score[m] = score.get(m, 0) + 1 / (RRF_K + r)
    return sorted(score, key=lambda m: -score[m])


FORMS = ("raw", "kw-any", "kw-each")
rows = {f: [] for f in FORMS}
matches = {f: [] for f in FORMS}
vec_rows = []
with db() as c:
    c.autocommit = True
    for qi, q in enumerate(queries):
        tgt = pos[q["target"]]
        rel = set(np.nonzero(memb @ memb[tgt] >= DUP_COS)[0]) | {tgt}
        sem = semantic(c, qemb[qi])
        vec_rows.append(hit_rr(sem, rel))
        for f in FORMS:
            lex, n = lexical(c, f, q)
            matches[f].append(n)
            rows[f].append((q["lang"], hit_rr(lex, rel), *[hit_rr(fuse(lex, sem, w), rel) for w in LEX_WEIGHTS]))

print(f"prompt: {dict(prod='production (one query per call)', bench1='benchmark (one query per call)').get(MODE, 'benchmark (batched)')}")
print(f"queries: {len(queries)}  table: {TABLE}  avg keywords (en+zh): "
      f"{np.mean([len(keywords(q)) for q in queries]):.1f}  single keyword call: {single_call_s:.1f} s\n")
print(f"vector alone: R@10 {np.mean([r[0] for r in vec_rows]):.3f}  MRR {np.mean([r[1] for r in vec_rows]):.3f}\n")
print(f"{'BM25 query':10} {'matches':>8} {'BM25':>6} {'en':>6} {'zh':>6}" + "".join(f"{'rrf w=' + str(w):>11}" for w in LEX_WEIGHTS))
for f in FORMS:
    rs = rows[f]
    bm = np.mean([r[1][0] for r in rs])
    by = {lg: np.mean([r[1][0] for r in rs if r[0] == lg]) for lg in ("en", "zh")}
    print(f"{f:10} {int(np.median(matches[f])):8d} {bm:6.3f} {by['en']:6.3f} {by['zh']:6.3f}"
          + "".join(f"{np.mean([r[2 + i][0] for r in rs]):11.3f}" for i in range(len(LEX_WEIGHTS))))
print("\n(matches = median number of metrics the BM25 query matches; other columns R@10)")
log("done")
