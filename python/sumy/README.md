# Sumy worker environment

The Go `extract_summaries` processor embeds `server/api/doc-processing/sumy_worker.py`.
Python only ranks passages; Go loads chunks, verifies evidence and saves
`kb.ex_summaries`. There is one Python invocation per document and one ranking
operation per chunk. No LLM, API credential or generative model is required.

From the ChenWeb root, provision once on each machine:

```sh
uv sync --locked --directory python/sumy
python/sumy/.venv/bin/python -m nltk.downloader -d python/sumy/nltk_data punkt_tab
export SUMY_PYTHON="$PWD/python/sumy/.venv/bin/python"
```

For Windows use `.venv/Scripts/python.exe`. The worker finds `nltk_data` next to
the virtual environment; `NLTK_DATA` can also point to a provisioned directory.
Provision this environment on the deployment host as well: the Go executable
embeds the worker code but does not bundle Python, packages or tokenizer data.
The process does not install or download anything while processing documents.

Set `EXTRACT_SUMMARIES_ALGORITHM` to `lexrank` (default), `textrank`, `lsa`, `luhn`
or `sumbasic`, and `EXTRACT_SUMMARIES_SENTENCES` to 1–100 (default 3).
`EXTRACT_SUMMARIES_TIMEOUT` defaults to `2m` per document. This is a sentence budget,
not the frozen benchmark's character budget; ranking is not constrained by that
benchmark unless a separate evaluation adapter applies its output budget.

Add `extract_summaries` to `[doc-processing].required_processors` in the active
config/config.local.toml. Include it in `default_processors` only if it should run
on normal automatic events. Existing nonempty defaults are retained locally.
The dashboard discovers stages from this configuration, so no new UI is needed.

Dev Mode request on `kb.pdf.start-doc-processing`:

```json
{"record_ids":[416],"doc-processors":["extract_summaries"],"failed-proc-only":false}
```

The controller applies mandatory prerequisites and pipeline policy as usual.
Explicit processor selection does not bypass a governing enforced policy.
This processor also supports a direct `HandleEvent` call with already-produced
chunks; missing/empty chunk artifacts are errors, not instructions to rechunk.

Run the tests from ChenWeb:

```sh
python/sumy/.venv/bin/python -m unittest discover -s python/sumy -p 'test_worker.py' -v
SUMY_TEST_PYTHON="$PWD/python/sumy/.venv/bin/python" \
  go test ./server/api/doc-processing -run 'TestExtractSummar|TestPythonSumy' -count=1
EX_SUMMARY_DB_TEST=1 \
  go test ./server/api/doc-processing -run TestExtractSummariesMigrationIntegration -count=1
```

The optional database test uses the existing project credentials from environment
or `.env`, creates an isolated schema and drops it afterward. It checks migration
up/down/up, idempotent up, uniqueness, the actual installed summary-search trigger
function and input-deletion cascade. It does not apply the project migration to
the live `kb` schema or modify migration history. Normal service startup applies
the committed goose migration `20261021000002_create_kb_ex_summaries.sql`.

Output is an extract in the source language. Translation, keywords, embeddings,
summary trees and global hybrid-search indexing are outside this processor.
English and Chinese are supported; empty/mixed language metadata triggers content
detection. Unsupported explicit language metadata fails visibly. Text-only tables
are retained verbatim; ranking is not a table-understanding or requirement-coverage
guarantee. Chunks with no usable ranking vocabulary or no ranked result fall back to
source-order selection; each such result records its reason in extraction_config.
