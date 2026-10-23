"""Evaluate the category index for known-item metric search against brute-force cosine and BM25."""
import json
import os
from collections import Counter, defaultdict

import numpy as np

from lib import OUT, db, embed, log

DUP_COS = 0.95  # a metric this close to the target counts as the same item (e.g. GB 43150 filed three times)
K = 10

metrics = json.loads((OUT / "metrics.json").read_text())
mids = [m["id"] for m in metrics]
pos = {mid: i for i, mid in enumerate(mids)}
memb = np.load(OUT / "metric_emb.npy")
# Domain sources, chosen separately for metrics (EXP_MDOMAIN) and queries (EXP_QDOMAIN):
#   v1   = original ICS labels, embedding shortlist + LLM verification (s3_file.py)
#   ics2 = ICS with LLM-written alternative labels, same method (EXP_ICS=ics2 s3_file.py)
#   walk = ICS v2, level-by-level LLM walk (s3b_walk.py); for queries, the emb@k modes still use the ics2 shortlist
MD, QD = os.environ.get("EXP_MDOMAIN", "v1"), os.environ.get("EXP_QDOMAIN", "v1")
MSRC = {"v1": "", "ics2": "_ics2", "walk": "", "walk_v2": ""}  # walk: domain replaced below; facets are the same in every file
QSRC = {"v1": "", "ics2": "_ics2", "walk": "_ics2", "walk_v2": "_ics2"}
fm = json.loads((OUT / f"filed_metrics{MSRC[MD]}.json").read_text())
fq = json.loads((OUT / f"filed_queries{QSRC[QD]}.json").read_text())
for f, src, what in ((fm, MD, "metrics"), (fq, QD, "queries")):
    if src.startswith("walk"):
        for iid, nodes in json.loads((OUT / f"{src}_{what}.json").read_text()).items():
            if iid in f:
                f[iid]["domain"] = nodes
if os.environ.get("EXP_FACET") == "v2":  # facet bindings re-verified with verify-facets-v2.md (s3c_facets.py)
    for f, name in ((fm, "facets_v2_metrics.json"), (fq, "facets_v2_queries.json")):
        for iid, nodes in json.loads((OUT / name).read_text()).items():
            if iid in f:
                f[iid]["facet"] = nodes
queries = [q for q in json.loads((OUT / "queries.json").read_text()) if q["id"] in fq and q["target"] in pos]
qemb = embed([q["query"] for q in queries])

parent = {}
for t in ("ics", "facets"):  # ics2 has the same ids and parents
    for n in json.loads((OUT / f"{t}.json").read_text()):
        parent[n["id"]] = n["parent"]


def ancestors(n):
    out = []
    while n:
        out.append(n)
        n = parent.get(n)
    return out


anc = {n: set(ancestors(n)) for n in parent}


def related(b, q):
    """b (a metric's node) is in q's subtree, or is an ancestor of q (the metric was filed coarser)."""
    return q in anc[b] or b in anc[q]


def lift(nodes, depth):
    """Replace each node by its ancestor at the given depth (1 = ICS Field / facet group); 0 = unchanged."""
    if not depth:
        return nodes
    out = set()
    for n in nodes:
        chain = list(reversed(ancestors(n)))  # top-level first
        out.add(chain[min(depth, len(chain)) - 1])
    return list(out)


def matches(mnodes, qnodes, depth=0):
    mnodes, qnodes = lift(mnodes, depth), lift(qnodes, depth)
    if not qnodes or not mnodes:  # no query node: no filter; unresolved metric: root "others", matches all
        return True
    return any(related(b, q) for b in mnodes for q in qnodes)


def relevant(q):
    t = pos[q["target"]]
    return set(np.nonzero(memb @ memb[t] >= DUP_COS)[0]) | {t}


def score(ranked, rel):
    for r, i in enumerate(ranked[:K]):
        if i in rel:
            return 1, 1 / (r + 1)
    return 0, 0


# BM25 through the same ||| operator the search API uses.
with db() as c:
    mid_of = dict(c.execute("SELECT metric_id, id::text FROM kb.metrics").fetchall())
    bm25 = {}
    for q in queries:
        rows = c.execute("""SELECT sa.artifact_id FROM kb.search_artifacts_metric sa
            WHERE sa.search_document ||| %(q)s OR sa.primary_label ||| %(q)s OR sa.secondary_label ||| %(q)s
               OR sa.snippet_basis ||| %(q)s
            ORDER BY pdb.score(sa.artifact_id) DESC LIMIT %(k)s""", {"q": q["query"], "k": K}).fetchall()
        bm25[q["id"]] = [pos[mid_of[r[0]]] for r in rows if mid_of.get(r[0]) in pos]


def query_nodes(q, kind, mode):
    if mode == "llm":
        return fq[q["id"]][kind]
    k = int(mode.split("@")[1])
    return [n for n, _ in fq[q["id"]][kind + "_short"][:k]]


strategies = {"brute-force cosine": None, "bm25": None}
for mode in ("llm", "emb@1", "emb@3", "emb@5"):
    for use in ("domain", "facet", "pair"):
        strategies[f"cat {use} [{mode}]"] = (use, mode, 0)
    for d in (2, 1):  # domain compared at ICS level d (facets unchanged)
        strategies[f"cat pair dL{d} [{mode}]"] = ("pair", mode, d)
        strategies[f"cat domain dL{d} [{mode}]"] = ("domain", mode, d)

rows = []
per_q = defaultdict(dict)
for qi, q in enumerate(queries):
    rel = relevant(q)
    sims = memb @ qemb[qi]
    order = list(np.argsort(-sims))
    for name, spec in strategies.items():
        if name == "brute-force cosine":
            ranked, size, cell_hit = order, len(mids), 1
        elif name == "bm25":
            ranked, size, cell_hit = bm25[q["id"]], None, None
        else:
            use, mode, dl = spec
            dn, fn = query_nodes(q, "domain", mode), query_nodes(q, "facet", mode)
            keep = []
            for i in order:
                r = fm[mids[i]]
                ok_d = use == "facet" or matches(r["domain"], dn, dl)
                ok_f = use == "domain" or matches(r["facet"], fn)
                if ok_d and ok_f:
                    keep.append(i)
            ranked, size, cell_hit = keep, len(keep), int(any(i in rel for i in keep))
        hit, rr = score(ranked, rel)
        per_q[q["id"]][name] = hit
        rows.append((name, hit, rr, size, cell_hit, q["lang"]))

print(f"metric domain: {MD}  query domain: {QD}  facet: {os.environ.get('EXP_FACET', 'v1')}\n"
      f"queries: {len(queries)}  metrics: {len(mids)}  avg relevant per query: "
      f"{np.mean([len(relevant(q)) for q in queries]):.2f}\n")
print(f"{'strategy':28} {'R@10':>6} {'MRR':>6} {'cellRec':>8} {'|C| med':>8} {'|C| p90':>8} {'R@10 en':>8} {'R@10 zh':>8}")
for name in strategies:
    rs = [r for r in rows if r[0] == name]
    sizes = [r[3] for r in rs if r[3] is not None]
    cells = [r[4] for r in rs if r[4] is not None]
    by_lang = {lg: np.mean([r[1] for r in rs if r[5] == lg]) for lg in ("en", "zh")}
    print(f"{name:28} {np.mean([r[1] for r in rs]):6.3f} {np.mean([r[2] for r in rs]):6.3f} "
          f"{(np.mean(cells) if cells else float('nan')):8.3f} "
          f"{(np.median(sizes) if sizes else float('nan')):8.0f} {(np.percentile(sizes, 90) if sizes else float('nan')):8.0f} "
          f"{by_lang['en']:8.3f} {by_lang['zh']:8.3f}")

# Index shape.
print("\nindex shape (LLM-verified metric filing)")
for kind in ("domain", "facet"):
    c = Counter(n for r in fm.values() for n in r[kind])
    sizes = sorted(c.values(), reverse=True)
    print(f"  {kind}: unresolved {sum(1 for r in fm.values() if not r[kind])}, nodes used {len(c)}, "
          f"avg per metric {np.mean([len(r[kind]) for r in fm.values()]):.2f}, largest {sizes[:5]}")
pairs = Counter((d, f) for r in fm.values() for d in (r["domain"] or ["-"]) for f in (r["facet"] or ["-"]))
ps = sorted(pairs.values(), reverse=True)
print(f"  pair cells: {len(pairs)}, median {np.median(ps):.0f}, p90 {np.percentile(ps, 90):.0f}, largest {ps[:5]}")

# Point 2: how often a metric's domain differs from its document's most common domain.
doc_top = {}
by_doc = defaultdict(Counter)
for m in metrics:
    for n in fm[m["id"]]["domain"]:
        by_doc[m["record_id"]][n] += 1
for rid, c in by_doc.items():
    doc_top[rid] = c.most_common(1)[0][0]
diff = [m for m in metrics if fm[m["id"]]["domain"] and doc_top.get(m["record_id"]) not in fm[m["id"]]["domain"]]
print(f"  metrics whose domain is not their document's most common domain: {len(diff)} of {len(metrics)}")

(OUT / "eval_per_query.json").write_text(json.dumps(per_q, indent=1))
log("done")
