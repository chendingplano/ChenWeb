"""Dump metrics (+ their existing bge-m3 embeddings), the ICS tree and the draft facet tree; embed tree nodes."""
import json

import numpy as np

from lib import E, OUT, db, embed, log

with db() as c:
    rows = c.execute("""
        SELECT m.id, m.input_record_id, coalesce(i.title, ''), coalesce(m.metric_name, ''), coalesce(m.metric_name_en, ''),
               coalesce(m.metric_subject, ''), coalesce(m.metric_subject_en, ''), coalesce(m.metric_desc_en, m.metric_desc, ''),
               coalesce(m.metric_context, ''), coalesce(m.metric_unit, ''), coalesce(m.threshold_or_target, m.metric_value, ''),
               coalesce(m.condition, ''), coalesce(m.metric_categories_en, ''), s.embedding::text, m.frame
        FROM kb.metrics m JOIN kb.inputs i ON i.id = m.input_record_id
        JOIN kb.search_artifacts_metric s ON s.artifact_id = m.metric_id
        WHERE m.ref_mention_id IS NULL ORDER BY m.id""").fetchall()
    metrics, vecs = [], []
    for r in rows:
        metrics.append({"id": str(r[0]), "record_id": r[1], "doc_title": r[2][:120], "name": r[3], "name_en": r[4],
                        "subject": r[5], "subject_en": r[6], "description": r[7][:300], "context": r[8][:300],
                        "unit": r[9], "value": r[10][:120], "condition": r[11][:200], "proposals": r[12], "frame": r[14]})
        vecs.append(json.loads(r[13]))
    v = np.asarray(vecs, dtype=np.float32)
    np.save(OUT / "metric_emb.npy", v / np.linalg.norm(v, axis=1, keepdims=True))
    (OUT / "metrics.json").write_text(json.dumps(metrics, ensure_ascii=False))
    log("metrics", len(metrics))

    nodes = c.execute("""SELECT n.id, n.parent_id, n.canonical_name, n.is_others, n.pref_labels, n.definitions, n.scope_notes
        FROM kb.category_nodes n JOIN kb.category_trees t ON t.id = n.tree_id
        WHERE t.canonical_name = 'cat:general-tech-categories' AND n.retired_at IS NULL""").fetchall()

by = {r[0]: r for r in nodes}


def path(nid, lang):
    out = []
    while nid and by[nid][1]:  # stop below the root
        out.append(by[nid][4].get(lang, ""))
        nid = by[nid][1]
    return list(reversed(out))


ics = []
for r in nodes:
    nid, parent, code, others, labels, defs, scope = r
    if others or parent is None or code.startswith("01.040"):
        continue  # "others" leaves, root and the Vocabularies subtree are never offered as candidates
    ics.append({"id": nid, "parent": parent if by[parent][1] else None, "code": code, "en": labels.get("en", ""),
                "zh": labels.get("zh", ""), "path_en": " > ".join(path(nid, "en")), "path_zh": " > ".join(path(nid, "zh")),
                "def": (scope or {}).get("en") or (defs or {}).get("en", "")})

facet_src = json.loads((E / "facets.json").read_text())
facets = []
for g in facet_src["groups"]:
    facets.append({"id": "f:" + g["name"], "parent": None, "code": g["name"], "en": g["en"], "zh": g["zh"],
                   "path_en": g["en"], "path_zh": g["zh"], "def": g["def_en"], "alt": []})
    for lf in g["leaves"]:
        facets.append({"id": f"f:{g['name']}/{lf['name']}", "parent": "f:" + g["name"], "code": lf["name"], "en": lf["en"],
                       "zh": lf["zh"], "path_en": f"{g['en']} > {lf['en']}", "path_zh": f"{g['zh']} > {lf['zh']}",
                       "def": lf["def_en"], "alt": lf.get("alt_en", []) + lf.get("alt_zh", [])})

for name, ns in (("ics", ics), ("facets", facets)):
    texts = [f"{n['path_en']} / {n['path_zh']}. {n['def']} {'; '.join(n.get('alt', []))}" for n in ns]
    np.save(OUT / f"{name}_emb.npy", embed(texts))
    (OUT / f"{name}.json").write_text(json.dumps(ns, ensure_ascii=False))
    log(name, len(ns))
