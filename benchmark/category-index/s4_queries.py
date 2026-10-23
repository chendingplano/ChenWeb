"""Known-item queries: sample metrics per document, have the LLM write a paraphrased query for each
(alternating en/zh), then extract the queries' category candidates with the same prompt as metrics."""
import json
import random
from collections import defaultdict

from lib import OUT, batched_llm, log, prompt
from s2_candidates import view

PER_DOC = 14
metrics = json.loads((OUT / "metrics.json").read_text())
by_doc = defaultdict(list)
for m in metrics:
    if (m["name"] or m["name_en"]).strip():
        by_doc[m["record_id"]].append(m)
rng = random.Random(7)
sample = []
for rid in sorted(by_doc):
    sample += rng.sample(by_doc[rid], min(PER_DOC, len(by_doc[rid])))
items = [{"id": f"q{m['id']}", "target": m["id"], "lang": "en" if i % 2 == 0 else "zh", "metric": view(m)}
         for i, m in enumerate(sample)]
for it in items:
    it["metric"].pop("doc_title", None)

gen = batched_llm("querygen", prompt("query-gen-v1.md"), items, 10,
                  lambda b: {"items": [{"id": it["id"], "lang": it["lang"], "metric": it["metric"]} for it in b]})
queries = [{"id": it["id"], "target": it["target"], "lang": it["lang"], "query": gen[it["id"]]["query"]}
           for it in items if gen.get(it["id"], {}).get("query")]
(OUT / "queries.json").write_text(json.dumps(queries, ensure_ascii=False, indent=1))
log("queries", len(queries))
batched_llm("cand_queries", prompt("candidates-v1.md"), queries, 15, lambda b: [{"id": q["id"], "query": q["query"]} for q in b])
