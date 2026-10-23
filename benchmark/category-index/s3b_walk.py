"""File items into the ICS (v2, with alternative labels) by an LLM walk, one level at a time.

At each parent, one call shows all children once and files a batch of items that reached that parent.
An item whose walk stops at a parent (no child fits) is bound to that parent (its "others").
Usage: s3b_walk.py metrics|queries
"""
import json
import os
import sys
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor

from lib import OUT, RUN_TAG, batched_llm, log, prompt
from s2_candidates import view

MAX_FANOUT = 3
WALK = os.environ.get("EXP_WALK", "v1")  # v2 = generous multi-membership (prompt-catidx-walk-v2.md)
TAG = ("" if WALK == "v1" else "_" + WALK) + RUN_TAG
what = sys.argv[1]
ics = json.loads((OUT / "ics2.json").read_text())
node = {n["id"]: n for n in ics}
kids = defaultdict(list)
for n in ics:
    kids[n["parent"]].append(n)

if what == "metrics":
    items = {m["id"]: view(m) for m in json.loads((OUT / "metrics.json").read_text())}
else:
    items = {q["id"]: {"query": q["query"]} for q in json.loads((OUT / "queries.json").read_text())}


def child_view(c):
    return {"id": c["code"], "label": f"{c['en']} / {c['zh']}", "definition": c["def"][:200], "typical": c.get("alt", [])[:12]}


frontier = {iid: [None] for iid in items}  # None = root
leaves = defaultdict(list)
for level in range(1, 4):
    work = defaultdict(list)  # parent -> item ids
    for iid, parents in frontier.items():
        for p in parents:
            if kids.get(p):
                work[p].append(iid)
            else:
                leaves[iid].append(p)
    if not work:
        break
    nxt = defaultdict(list)

    def file_under(p, iids, level=level):
        pv = {"label": "ICS root (all fields)"} if p is None else {"label": f"{node[p]['path_en']} / {node[p]['zh']}", "definition": node[p]["def"][:200]}
        cv = [child_view(c) for c in kids[p]]
        cache = f"walk{TAG}_{what}_L{level}_{(node[p]['code'] if p else 'root').replace('.', '_')}"
        return p, iids, batched_llm(cache, prompt(f"walk-{WALK}.md"), [{"id": iid, "item": items[iid]} for iid in iids], 8,
                                    lambda b: {"parent": pv, "children": cv, "items": b}, workers=24 if p is None else 6)

    with ThreadPoolExecutor(8) as ex:
        for p, iids, res in ex.map(lambda kv: file_under(*kv), list(work.items())):
            code_to_id = {c["code"]: c["id"] for c in kids[p]}
            for iid in iids:
                chosen = [code_to_id[c] for c in (res.get(iid, {}).get("chosen") or []) if c in code_to_id][:MAX_FANOUT]
                if chosen:
                    nxt[iid].extend(chosen)
                else:
                    leaves[iid].append(p)  # stops here: bound to p's "others" (root if p is None)
    frontier = nxt
    log(f"level {level}: {sum(len(v) for v in work.values())} item-parent pairs in {len(work)} parents")
for iid, parents in frontier.items():
    leaves[iid].extend(parents)

out = {iid: sorted({p for p in leaves[iid] if p}) for iid in items}
(OUT / f"walk{TAG}_{what}.json").write_text(json.dumps(out, ensure_ascii=False))
depth = [len(node[n]["code"].split(".")) for v in out.values() for n in v]
log("unresolved (root):", sum(1 for v in out.values() if not v), "of", len(out),
    "| avg nodes:", round(sum(len(v) for v in out.values()) / len(out), 2),
    "| depth 1/2/3:", [depth.count(d) for d in (1, 2, 3)])
