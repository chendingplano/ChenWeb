#!/usr/bin/env python3
"""One-off backfill: annotate already-parsed mineru consolidated JSON files
(kb.inputs.result_filename) with per-list-item bboxes, using each record's
still-on-disk MinerU *_middle.json.

Context: parser_mineru.py's MineruParser.parse() now calls
annotate_list_item_bboxes() at parse time so new records get correct
per-list-item bboxes baked into their consolidated JSON. Records parsed
before that fix have a consolidated JSON without this annotation. For
records whose doc-processing pipeline has already run (kb.inputs.status has
a "converted" entry), the *.txt line file has already been produced from
the old un-annotated JSON -- those are fixed separately by the Go tool
ChenWeb/server/cmd/backfill-mineru-list-bboxes, which patches the .txt
in place.

This script covers the complementary case: records that were parsed but
whose doc-processing pipeline has NOT run yet (no .txt exists). For those,
patching the stored consolidated JSON now means the eventual pipeline run
will produce a correct .txt on the first try, via the already-fixed Go
converter (ChenWeb/server/api/file-converters/mineru.go).

Safe to run against records that already have a .txt too: it only touches
the intermediate consolidated JSON, never the .txt/.origin/.manual files
that downstream stages and the UI actually read.
"""
import argparse
import glob
import json
import os

import psycopg2

from parser_mineru import annotate_list_item_bboxes


def find_middle_json(json_path: str) -> str | None:
    d = os.path.dirname(json_path)
    base = os.path.basename(json_path)
    root, _ext = os.path.splitext(base)
    stem = root[: -len("_mineru")] if root.endswith("_mineru") else root
    pattern = os.path.join(d, stem, "*", f"{stem}_middle.json")
    candidates = glob.glob(pattern)
    if not candidates:
        return None
    return max(candidates, key=os.path.getmtime)


def resolve_path(data_home: str, result_filename: str) -> str:
    if os.path.isabs(result_filename):
        return result_filename
    return os.path.join(data_home, result_filename) if data_home else result_filename


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--record-id", type=int, default=0)
    args = ap.parse_args()

    data_home = os.environ.get("DATA_HOME_DIR", "")
    conn = psycopg2.connect(
        host=os.environ.get("PG_HOST", "localhost"),
        port=os.environ.get("PG_PORT", "5432"),
        user=os.environ.get("PG_USER_NAME", ""),
        dbname=os.environ.get("PG_DB_NAME", ""),
        password=os.environ.get("PG_PASSWORD") or None,
    )
    cur = conn.cursor()
    if args.record_id:
        cur.execute(
            "select id, result_filename from kb.inputs where id=%s and parser_name='mineru'",
            (args.record_id,),
        )
    else:
        cur.execute(
            "select id, result_filename from kb.inputs "
            "where type='pdf' and parser_name='mineru' "
            "and result_filename is not null and result_filename <> ''"
        )
    rows = cur.fetchall()

    patched = 0
    total_items = 0
    skipped_no_json = 0
    skipped_no_middle = 0
    for rec_id, result_filename in rows:
        json_path = resolve_path(data_home, result_filename)
        if not os.path.isfile(json_path):
            print(f"record {rec_id}: skip, json not found: {json_path}")
            skipped_no_json += 1
            continue
        middle_path = find_middle_json(json_path)
        if not middle_path:
            print(f"record {rec_id}: skip, no middle.json found")
            skipped_no_middle += 1
            continue

        with open(json_path, encoding="utf-8") as f:
            doc = json.load(f)
        with open(middle_path, encoding="utf-8") as f:
            middle = json.load(f)

        changed = 0
        for page in doc.get("pages", []):
            changed += annotate_list_item_bboxes(page.get("items", []), middle)

        if changed == 0:
            continue
        patched += 1
        total_items += changed
        verb = "would annotate" if args.dry_run else "annotated"
        print(f"record {rec_id}: {verb} {changed} list item(s)")
        if not args.dry_run:
            with open(json_path, "w", encoding="utf-8") as f:
                json.dump(doc, f, ensure_ascii=False)

    print(
        f"done: {patched}/{len(rows)} record(s) patched, {total_items} list item(s) annotated, "
        f"{skipped_no_json} skipped (no json), {skipped_no_middle} skipped (no middle.json) "
        f"(dry-run={args.dry_run})"
    )


if __name__ == "__main__":
    main()
