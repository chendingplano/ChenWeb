"""Shared helpers for the category-index experiment (offline; writes nothing to kb)."""
import json
import os
import sys
import time
import tomllib
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import httpx
import numpy as np
import psycopg

E = Path(__file__).resolve().parent
CHENWEB = E.parents[1]
OUT = E / "out"
OUT.mkdir(exist_ok=True)
MODELS = tomllib.loads(Path(os.environ.get("MODEL_DEF_FILE", CHENWEB / ".models.toml")).read_text())
LLM = MODELS[os.environ.get("EXP_LLM", "deepseek-flash-processor")]
EMB = MODELS["bge-m3-llama-cpp"]


def log(*a):
    print(time.strftime("%H:%M:%S"), *a, file=sys.stderr, flush=True)


def db():
    return psycopg.connect(host=os.environ["PG_HOST"], port=os.environ["PG_PORT"], user=os.environ["PG_USER_NAME"],
                           password=os.environ["PG_PASSWORD"], dbname=os.environ["PG_DB_NAME"])


def prompt(name):
    """Experiment prompts live in ChenWeb/prompts as prompt-catidx-<name>."""
    return (Path(os.environ.get("PROMPT_DIR", CHENWEB / "prompts")) / f"prompt-catidx-{name}").read_text()


_http = httpx.Client(timeout=300)


def llm_json(system, payload, retries=4):
    body = {"model": LLM["model_name"], "temperature": 0, "response_format": {"type": "json_object"},
            "messages": [{"role": "system", "content": system},
                         {"role": "user", "content": json.dumps(payload, ensure_ascii=False)}]}
    url = LLM["base_url"].rstrip("/") + "/chat/completions"
    err = None
    for i in range(retries):
        try:
            r = _http.post(url, json=body, headers={"Authorization": "Bearer " + LLM["api_key"]})
            r.raise_for_status()
            j = r.json()
            return json.loads(j["choices"][0]["message"]["content"]), j.get("usage", {})
        except Exception as e:  # noqa: BLE001 - retried, then raised
            err = e
            time.sleep(2 * (i + 1))
    raise RuntimeError(f"llm failed: {err}")


def embed(texts, batch=32):
    url = EMB["base_url"].rstrip("/") + "/v1/embeddings"
    out = []
    for i in range(0, len(texts), batch):
        chunk = [t[:EMB.get("max_chars", 6000)] or " " for t in texts[i:i + batch]]
        r = _http.post(url, json={"model": EMB["model_name"], "input": chunk})
        r.raise_for_status()
        out.extend(d["embedding"] for d in sorted(r.json()["data"], key=lambda d: d["index"]))
    v = np.asarray(out, dtype=np.float32)
    return v / np.linalg.norm(v, axis=1, keepdims=True)


def batched_llm(cache_name, system, items, size, make_payload, workers=24):
    """Run items through the LLM in batches, caching results per id in out/<cache_name>.jsonl."""
    path = OUT / f"{cache_name}.jsonl"
    done = {}
    if path.exists():
        for line in path.read_text().splitlines():
            r = json.loads(line)
            done[r["id"]] = r
    todo = [it for it in items if it["id"] not in done]
    batches = [todo[i:i + size] for i in range(0, len(todo), size)]
    log(f"{cache_name}: {len(done)} cached, {len(todo)} to do in {len(batches)} batches")
    usage = {"prompt_tokens": 0, "completion_tokens": 0}

    def run(b):
        try:
            res, u = llm_json(system, make_payload(b))
        except RuntimeError as e:
            log("batch failed:", e)
            return [], {}
        want = {it["id"] for it in b}
        return [r for r in res.get("items", []) if str(r.get("id")) in want], u

    with ThreadPoolExecutor(workers) as ex, path.open("a") as f:
        for n, (rows, u) in enumerate(ex.map(run, batches), 1):
            for k in usage:
                usage[k] += u.get(k, 0)
            for r in rows:
                r["id"] = str(r["id"])
                done[r["id"]] = r
                f.write(json.dumps(r, ensure_ascii=False) + "\n")
            if n % 10 == 0:
                log(f"  {n}/{len(batches)} batches")
    log(f"{cache_name}: usage {usage}; missing {len([it for it in items if it['id'] not in done])}")
    return done
