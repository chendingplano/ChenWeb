# MinerU 4.0.9 Parser Migration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Upgrade the MinerU runtime used by ChenWeb's PDF parser to official 4.0.9 while preserving ChenWeb's stored result schema and OCR fallback.

**Architecture:** Update the ThirdParty source checkout and local setup to MinerU 4.0.9. Invoke the stateless `mineru-kit parse` command for ZIP output. A helper running in the MinerU virtualenv will render MiddleJson V2 as Content List V1; the parser will group those items into its existing page wrapper, preserve image assets, and keep the Go converter contract stable.

**Tech Stack:** Python 3.12, MinerU 4.0.9, uv, mise, Python subprocess/ZIP/JSON handling, ChenWeb Go file converter.

---

## Chunk 1: Upgrade MinerU runtime and adapt parser

### File map

- `ThirdParty/mineru/**`: synchronize tracked MinerU source with official 4.0.9 while preserving the workspace's local setup files.
- `ThirdParty/mineru/mise.toml`: install and operate the 4.x runtime, use its new config and CLI, and remove obsolete 3.x task arguments.
- `ThirdParty/mineru/mineru.template.yaml` (or the official 4.x config template chosen during source sync): define the local model source and automatic small-model/VLM engine selection.
- `ChenWeb/python/pdf-parser/parser_mineru.py`: invoke MinerU Kit, unpack its ZIP result, feed MiddleJson V2 through the helper, preserve image handling and progress reporting.
- `ChenWeb/python/pdf-parser/mineru_v4_adapter.py`: run inside the MinerU environment, render MiddleJson V2 to Content List V1, and emit normalized JSON grouped into ChenWeb's existing page/item shape.
- `ChenWeb/python/pdf-parser/pdf_parser.py`: keep the existing garbled-text fallback trigger; select the 4.x basic tier and OCR mode without obsolete backend, method, or language flags.
- `ChenWeb/python/pdf-parser/README.md`: document 4.0.9 installation, CLI selection, tiers, OCR fallback, and the stable ChenWeb output contract.

### Task 1: Update the ThirdParty MinerU checkout and setup

**Files:** `ThirdParty/mineru/**`

- [x] Inspect the official `mineru-4.0.9` source tag and compare it with the tracked checkout. Preserve ChenWeb-specific setup files, and use the official package version and dependency groups.
- [x] Replace the obsolete 3.x installation task with the official 4.0.9 install, using the supported `torch` extra for the current Apple Silicon/MPS environment.
- [x] Replace the old JSON configuration environment variable and template with the 4.x `MINERU_CONFIG` YAML configuration. Keep model source configurable and let 4.x select MPS/llama.cpp automatically.
- [x] Update version, model-download, and update tasks to 4.x commands; remove commands that invoke the legacy `mineru-gradio`, `-b pipeline`, or `-m all` interfaces.
- [x] Verify the configured virtualenv reports MinerU 4.0.9 and that `mineru-kit parse --help` exposes the required `--pages`, `--tier`, `--ocr-mode`, `--format`, and `--output` options.
- [x] Commit the ThirdParty runtime update through `jj` in that repository.

### Task 2: Add the 4.x result adapter and update the subprocess parser

**Files:** `ChenWeb/python/pdf-parser/mineru_v4_adapter.py`, `ChenWeb/python/pdf-parser/parser_mineru.py`

- [x] Add a small adapter entrypoint that accepts a MiddleJson V2 file and an output JSON path. Load the document, call `mineru.render.render(middle_json, RenderFormat.CONTENT_LIST)`, and group flat entries by zero-based `page_idx` into the existing one-based `page_number` wrapper.
- [x] Map Content List V1 text, title, list, table, image, equation, header, footer, and page-number fields to ChenWeb's existing item fields. Keep optional values optional; preserve otherwise-unmapped textual content as a text item with its page and bbox when available.
- [x] Translate list item boxes from MiddleJson V2 page/block geometry when present; retain the containing list bbox when per-item boxes are unavailable. Preserve equation/image asset references from the materialized ZIP paths.
- [x] Change MineruParser to invoke `mineru-kit parse <pdf> --pages all --tier standard --format zip -o <zip>`. Extract `middle_json.json` and `images/` into a per-run temporary directory, call the adapter with the MinerU venv Python, then move normalized JSON and assets into the parser output layout.
- [x] Keep stale-output cleanup, callback heartbeat, failure-tail logging, and abort handling. Adjust phase parsing to the 4.x CLI's progress output while retaining periodic callbacks if MinerU does not expose page progress.
- [x] Rewrite image references in normalized JSON to the final parser `images/` paths after moving assets; downstream consumers must never receive ZIP-internal or temporary paths.
- [x] Update output discovery and returned result metadata so downstream code still sees ChenWeb's existing consolidated JSON and filenames.

### Task 3: Migrate the OCR fallback and update parser documentation

**Files:** `ChenWeb/python/pdf-parser/pdf_parser.py`, `ChenWeb/python/pdf-parser/parser_mineru.py`, `ChenWeb/python/pdf-parser/README.md`

- [x] Replace the fallback's legacy `pipeline`/`-m ocr`/`-l` options with `mineru-kit parse <pdf> --pages all --tier basic --ocr-mode ocr --format zip -o <zip>`.
- [x] Retain the current garbled-font detection and the successful fallback result metadata. Stop passing the legacy language option, which MinerU 4 no longer supports.
- [x] Update README install/configuration instructions, `MINERU_CLI` path, normal `standard` tier, fallback `basic`/OCR behavior, and the stable JSON handoff to the Go converter.
- [x] Run non-test checks only: verify MinerU reports exactly 4.0.9; inspect `mineru-kit parse --help`; run Python syntax compilation for the changed Python modules; inspect diffs and `jj log` for expected linear commits.
- [x] Do not run automated tests or parse a document unless the user explicitly requests testing/verification.
- [x] Commit the ChenWeb parser and documentation changes through `jj`.

## Completion checklist

- [x] The ThirdParty runtime uses official MinerU 4.0.9 and its 4.x configuration/tasks.
- [x] Normal parsing and forced OCR use documented 4.x flags and process every page.
- [x] The adapter preserves ChenWeb's established JSON contract, item data, and assets.
- [x] The README reflects actual install/runtime behavior.
- [x] No unrelated untracked PDFs were modified; repository commits are linear and separated by repository.

## References

- Design: [`../specs/2026-10-15-mineru-4-migration-design.md`](../specs/2026-10-15-mineru-4-migration-design.md)
- [MinerU migration guide](https://opendatalab.github.io/MinerU/reference/migration_4/)
- [MinerU output format contract](https://opendatalab.github.io/MinerU/reference/output_files/)
- [MinerU tiers and runtimes](https://opendatalab.github.io/MinerU/usage/tiers/)
