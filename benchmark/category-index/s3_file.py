"""File items into (domain, facet) cells: embedding shortlist per tree, then LLM verification.

Usage: s3_file.py metrics|queries
"""
import json
import os
import sys

import numpy as np

from lib import OUT, batched_llm, embed, log, prompt

ICS = os.environ.get("EXP_ICS", "ics")  # "ics2" = ICS with LLM-written alternative labels
SUFFIX = "" if ICS == "ics" else "_" + ICS
TREES = {"domain": (ICS, 12), "facet": ("facets", 8)}


def load_tree(name):
    nodes = json.loads((OUT / f"{name}.json").read_text())
    return nodes, np.load(OUT / f"{name}_emb.npy")


def shortlists(cands, kind):
    """cands: {item_id: [phrase dicts]} -> {item_id: [(node_index, score)] best first}."""
    tree, k = TREES[kind]
    _, emb = load_tree(tree)
    ids, texts = [], []
    for iid, phrases in cands.items():
        for p in phrases:
            ids.append(iid)
            texts.append(" / ".join(x for x in (p.get("en"), p.get("zh")) if x))
    best = {}
    if texts:
        sims = embed(texts) @ emb.T
        for iid, row in zip(ids, sims):
            cur = best.setdefault(iid, np.full(emb.shape[0], -1.0, dtype=np.float32))
            np.maximum(cur, row, out=cur)
    out = {}
    for iid in cands:
        s = best.get(iid)
        out[iid] = [] if s is None else [(int(i), float(s[i])) for i in np.argsort(-s)[:k]]
    return out


def verify(cache, kind, items, short):
    tree, _ = TREES[kind]
    nodes, _ = load_tree(tree)
    work = []
    for it in items:
        cands = [{"id": f"n{j + 1}", "path": nodes[i]["path_en"] + " | " + nodes[i]["path_zh"], "definition": nodes[i]["def"][:240]}
                 for j, (i, _) in enumerate(short[it["id"]])]
        work.append({"id": it["id"], "item": it["view"], "candidates": cands})
    res = batched_llm(cache, prompt("verify-v1.md"), work, 8,
                      lambda b: {"tree": "ICS domain tree" if kind == "domain" else "metric facet tree", "items": b})
    chosen = {}
    for it in items:
        keys = res.get(it["id"], {}).get("chosen", []) or []
        sl = short[it["id"]]
        chosen[it["id"]] = [nodes[sl[int(k[1:]) - 1][0]]["id"] for k in keys
                            if isinstance(k, str) and k[1:].isdigit() and 0 < int(k[1:]) <= len(sl)]
    return chosen


if __name__ == "__main__":
    what = sys.argv[1]
    from s2_candidates import view
    if what == "metrics":
        src = json.loads((OUT / "metrics.json").read_text())
        items = [{"id": m["id"], "view": view(m)} for m in src]
    else:
        src = json.loads((OUT / "queries.json").read_text())
        items = [{"id": q["id"], "view": {"query": q["query"]}} for q in src]
    cand = {}
    for line in (OUT / f"cand_{what}.jsonl").read_text().splitlines():
        r = json.loads(line)
        cand[r["id"]] = r
    items = [it for it in items if it["id"] in cand]
    result = {it["id"]: {} for it in items}
    for kind in TREES:
        short = shortlists({it["id"]: cand[it["id"]].get(kind, []) for it in items}, kind)
        nodes, _ = load_tree(TREES[kind][0])
        for iid, sl in short.items():
            result[iid][kind + "_short"] = [[nodes[i]["id"], round(s, 4)] for i, s in sl]
        for iid, ch in verify(f"verify_{what}_{kind}" + (SUFFIX if kind == "domain" else ""), kind, items, short).items():
            result[iid][kind] = ch
    (OUT / f"filed_{what}{SUFFIX}.json").write_text(json.dumps(result, ensure_ascii=False))
    for kind in TREES:
        log(kind, "empty:", sum(1 for r in result.values() if not r[kind]), "of", len(result),
            "avg chosen:", round(sum(len(r[kind]) for r in result.values()) / max(len(result), 1), 2))
