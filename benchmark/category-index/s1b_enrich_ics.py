"""ICS v2: add LLM-written alternative labels (typical things covered) to each node, then re-embed."""
import json

import numpy as np

from lib import OUT, batched_llm, embed, log, prompt

ics = json.loads((OUT / "ics.json").read_text())
kids = {}
for n in ics:
    kids.setdefault(n["parent"], []).append(n["en"])
items = [{"id": n["id"], "path": n["path_en"], "definition": n["def"][:300], "children": kids.get(n["id"], [])[:25]} for n in ics]
res = batched_llm("ics_examples", prompt("ics-examples-v1.md"), items, 10, lambda b: {"items": b})
for n in ics:
    r = res.get(n["id"], {})
    n["alt"] = [str(x) for x in (r.get("en") or [])] + [str(x) for x in (r.get("zh") or [])]
log("nodes without alt labels:", sum(1 for n in ics if not n["alt"]))
texts = [f"{n['path_en']} / {n['path_zh']}. {n['def']} {'; '.join(n['alt'])}" for n in ics]
np.save(OUT / "ics2_emb.npy", embed(texts))
(OUT / "ics2.json").write_text(json.dumps(ics, ensure_ascii=False))
