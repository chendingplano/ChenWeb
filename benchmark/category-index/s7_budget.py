"""Recall at a fixed scan budget: category routing vs k-means IVF (blind clustering) vs brute force.

Category routing probes the query's cells in rank order (walk nodes first, then the embedding shortlist) and
scans their metrics until the budget is used; IVF probes the nearest k-means centroids. Both then rank the
scanned metrics by cosine. Same env vars as s5_eval.py.
"""
import os

os.environ.setdefault("EXP_QUIET", "1")
import numpy as np  # noqa: E402

import s5_eval as ev  # noqa: E402

BUDGETS = (0.02, 0.05, 0.10, 0.20)
N = len(ev.mids)
rng = np.random.default_rng(0)


def kmeans(x, k, iters=25):
    c = x[rng.choice(len(x), k, replace=False)]
    for _ in range(iters):
        a = np.argmax(x @ c.T, axis=1)
        for j in range(k):
            pts = x[a == j]
            if len(pts):
                v = pts.mean(0)
                c[j] = v / np.linalg.norm(v)
    return c, np.argmax(x @ c.T, axis=1)


def ordered_nodes(qid, kind):
    out = list(ev.fq[qid][kind])
    for n, _ in ev.fq[qid][kind + "_short"]:
        if n not in out:
            out.append(n)
    return out


def category_scan(qid, budget, use):
    dn, fn = ordered_nodes(qid, "domain"), ordered_nodes(qid, "facet")
    probes = [(d, None) for d in dn] if use == "domain" else \
        sorted(((d, f) for d in dn for f in fn), key=lambda p: dn.index(p[0]) + fn.index(p[1]))
    seen, scanned = set(), []
    for d, f in probes:
        for i, mid in enumerate(ev.mids):
            if i in seen:
                continue
            r = ev.fm[mid]
            if r["domain"] and ev.matches(r["domain"], [d]) and (f is None or not r["facet"] or ev.matches(r["facet"], [f])):
                seen.add(i)
                scanned.append(i)
        if len(scanned) >= budget:
            break
    return scanned[:max(budget, 1)]


ivf = {k: kmeans(ev.memb.copy(), k) for k in (25, 50, 100)}
rows = {}
for qi, q in enumerate(ev.queries):
    rel = ev.relevant(q)
    qv = ev.qemb[qi]
    for frac in BUDGETS:
        b = int(N * frac)
        cands = {"cat domain": category_scan(q["id"], b, "domain"), "cat pair": category_scan(q["id"], b, "pair")}
        for k, (cent, assign) in ivf.items():
            scanned = []
            for j in np.argsort(-(cent @ qv)):
                scanned += list(np.nonzero(assign == j)[0])
                if len(scanned) >= b:
                    break
            cands[f"ivf k={k}"] = scanned[:b]
        for name, sc in cands.items():
            sc = np.asarray(sc, dtype=int)
            ranked = sc[np.argsort(-(ev.memb[sc] @ qv))] if len(sc) else sc
            rows.setdefault((name, frac), []).append(ev.score(list(ranked), rel)[0])

print(f"\nrecall@10 at scan budget (brute force over 100% = "
      f"{np.mean([ev.score(list(np.argsort(-(ev.memb @ ev.qemb[i]))), ev.relevant(q))[0] for i, q in enumerate(ev.queries)]):.3f})")
names = sorted({n for n, _ in rows}, key=lambda s: (not s.startswith("cat"), s))
print(f"{'method':14}" + "".join(f"{int(f * 100):>7}%" for f in BUDGETS))
for n in names:
    print(f"{n:14}" + "".join(f"{np.mean(rows[(n, f)]):8.3f}" for f in BUDGETS))
