"""Facet filing v2: re-verify the existing facet shortlists with prompt verify-facets-v2.md (all applicable facets).
Usage: s3c_facets.py metrics|queries  -> out/facets_v2_<what>.json"""
import json
import sys

from lib import OUT, batched_llm, log, prompt
from s2_candidates import view

what = sys.argv[1]
facets = {n["id"]: n for n in json.loads((OUT / "facets.json").read_text())}
filed = json.loads((OUT / f"filed_{what}.json").read_text())
if what == "metrics":
    views = {m["id"]: view(m) for m in json.loads((OUT / "metrics.json").read_text())}
else:
    views = {q["id"]: {"query": q["query"]} for q in json.loads((OUT / "queries.json").read_text())}
work, keymap = [], {}
for iid, r in filed.items():
    short = [n for n, _ in r["facet_short"]]
    keymap[iid] = short
    work.append({"id": iid, "item": views[iid], "candidates": [
        {"id": f"n{j + 1}", "path": facets[n]["path_en"] + " | " + facets[n]["path_zh"], "definition": facets[n]["def"]}
        for j, n in enumerate(short)]})
res = batched_llm(f"verify_{what}_facet_v2", prompt("verify-facets-v2.md"), work, 8,
                  lambda b: {"tree": "metric facet tree", "items": b})
out = {}
for iid, short in keymap.items():
    keys = res.get(iid, {}).get("chosen") or []
    out[iid] = [short[int(k[1:]) - 1] for k in keys if isinstance(k, str) and k[1:].isdigit() and 0 < int(k[1:]) <= len(short)]
(OUT / f"facets_v2_{what}.json").write_text(json.dumps(out))
log("empty:", sum(1 for v in out.values() if not v), "avg chosen:", round(sum(map(len, out.values())) / len(out), 2))
