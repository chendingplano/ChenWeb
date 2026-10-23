"""LLM category candidates (domain + facet) for every metric. Usage: s2_candidates.py [limit]"""
import json
import sys

from lib import OUT, batched_llm, prompt

FIELDS = ("doc_title", "name", "name_en", "subject", "subject_en", "description", "context", "unit", "value", "condition", "frame")


def view(m):
    return {k: m[k] for k in FIELDS if m.get(k)}


if __name__ == "__main__":
    metrics = json.loads((OUT / "metrics.json").read_text())
    if len(sys.argv) > 1:
        metrics = metrics[::len(metrics) // int(sys.argv[1])][:int(sys.argv[1])]
    res = batched_llm("cand_metrics", prompt("candidates-v1.md"), metrics, 15,
                      lambda b: [{"id": m["id"], **view(m)} for m in b])
    for m in metrics[:8]:
        r = res.get(m["id"], {})
        print(m["id"], m["name_en"] or m["name"], "|", m["subject_en"], "=>",
              [d.get("en") for d in r.get("domain", [])], [f.get("en") for f in r.get("facet", [])])
