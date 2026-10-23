"""Why the pair filter drops a query's target: domain side, facet side, or both. Same env vars as s5_eval.py."""
import os
from collections import Counter

os.environ.setdefault("EXP_QUIET", "1")
import s5_eval as ev  # noqa: E402  (runs the evaluation, then reuses its state)

nodes = {}
for t in ("ics2", "facets"):
    for n in ev.json.loads((ev.OUT / f"{t}.json").read_text()):
        nodes[n["id"]] = f"{n['code']} {n['en'][:34]}"
metrics = {m["id"]: m for m in ev.metrics}
why = Counter()
examples = []
for q in ev.queries:
    t = q["target"]
    r, qr = ev.fm[t], ev.fq[q["id"]]
    d_ok, f_ok = ev.matches(r["domain"], qr["domain"]), ev.matches(r["facet"], qr["facet"])
    if d_ok and f_ok:
        continue
    side = "both" if not (d_ok or f_ok) else ("domain" if not d_ok else "facet")
    why[side] += 1
    examples.append((side, q["query"][:60], (metrics[t]["name_en"] or metrics[t]["name"])[:50],
                     [nodes.get(n, n) for n in qr[side if side != "both" else "domain"]],
                     [nodes.get(n, n) for n in r[side if side != "both" else "domain"]]))
print("\npair misses by side:", dict(why))
for side, qtext, mname, qn, mn in examples[:40]:
    print(f"- [{side}] Q: {qtext}\n    query -> {qn}\n    metric '{mname}' -> {mn}")
