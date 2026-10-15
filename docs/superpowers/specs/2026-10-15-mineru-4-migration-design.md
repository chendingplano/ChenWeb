# MinerU 4.0.9 migration design

## Goal

Upgrade the MinerU installation used by `ChenWeb/python/pdf-parser` from the local
3.0.9 checkout to the latest stable 4.0.9 release, while keeping ChenWeb's stored
parser result and downstream Go converter contract stable.

## Current behavior

The Python parser invokes the legacy `mineru -p ... -o ...` command. It expects
MinerU 3.x `*_content_list.json`, uses `*_middle.json` to restore equation image
paths and per-item list bounding boxes, and copies result images into the parser's
output directory. ChenWeb's Go converter reads the consolidated JSON shape
`{"pages": [{"page_number", "items": [...] }]}`. Garbled text can trigger a second
pass using the legacy `pipeline` backend with OCR forced.

The local ThirdParty checkout is 3.0.9 and tracks a ChenWeb fork. MinerU 4 changes
the command entrypoint, configuration, tiers, and result schema. Its stateless
`mineru-kit parse` command can write a ZIP containing `middle_json.json` and image
assets. The migration guide warns that old JSON artifacts are not generally
compatible with the new result reader.

## Selected approach

Update the source checkout under `ThirdParty/mineru` to the official 4.0.9 release
and install it in the existing ThirdParty-managed virtual environment, retaining
the workspace path and setup entrypoint. Use the stateless
`mineru-kit parse <pdf> --tier standard --format zip -o <zip>` command for
complete-document conversion. For the existing forced-OCR fallback, use
`mineru-kit parse <pdf> --tier basic --ocr-mode ocr --format zip -o <zip>`.
`mineru-kit parse` processes all PDF pages by default; keep that behavior explicit
if the release CLI supports a page-range flag. The old `-l`/`--language` option is
not supported by the current 4.x CLI, so do not pass the configured legacy OCR
language argument. Parse the ZIP result and adapt its `middle_json.json` and
materialized `images/` assets into ChenWeb's existing consolidated JSON shape.
Keep the current image-copy behavior and progress/heartbeat reporting.

The adapter belongs in the Python parser. It should use MinerU 4's documented
`RenderFormat.CONTENT_LIST` renderer as the normalization step, then group its
flat content-list entries into ChenWeb's existing page wrapper. The mapping is:

| MinerU 4 Content List V1 entry | ChenWeb consolidated item |
| --- | --- |
| `page_idx` (zero-based) | enclosing `page_number` (`page_idx + 1`) |
| `type=text`, `text`, optional `text_level`, `bbox` | `type`, `text`, `text_level`, `bbox` |
| `type=list`, `list_items`, `bbox` | `type`, `list_items`, `bbox`; retain individual list-item boxes when derivable from MiddleJson V2 blocks |
| `type=table`, table fields, captions/footnotes, `img_path`, `bbox` | existing table fields (`table_body`, `table_caption`, `table_footnote`, `img_path`, `bbox`) |
| `type=image`, image captions/footnotes, `img_path`, `bbox` | existing image fields (`image_caption`, `image_footnote`, `img_path`, `bbox`) |
| `type=equation`, text and image reference, `bbox` | existing equation fields (`text`, `img_path`, `bbox`) |
| `type=header`, `footer`, `page_number` | same item types so the existing cover-page and running-furniture rules continue to apply |

The adapter must tolerate optional fields omitted by the V1 renderer. When an
item has no direct V1 equivalent, retain its textual content as a `text` item
with its page and geometry when available; do not silently discard text. Keep
the Go converter, stored JSON schema, server routing, and existing records on
their current contract. The fallback must not pass the removed `-l`/`--language`
argument.

## Alternatives considered

1. Update the Go converter and stored contract to consume MinerU 4 output
   directly. This creates a broader cross-service migration and is not required
   to upgrade the parser.
2. Stay on the newest compatible MinerU 3.x release. This avoids the breaking
   migration but does not achieve the requested 4.0.9 upgrade.

## Compatibility and failure handling

- Keep all-page parsing explicit and retain ChenWeb's current result file naming.
- Adapt 4.x page indices to the one-based `page_number` expected by the Go
  converter.
- Preserve the source-to-output field mapping above, including text/heading
  levels, tables, images, equations, captions, footnotes, and page furniture.
  Continue deriving per-item list boxes from the new MiddleJson V2 page/block
  structure; use the parent list box only when item boxes are unavailable.
- Resolve image references from the ZIP into the existing `images/` destination;
  fail parsing clearly if required structured output is missing or malformed.
- Preserve the OCR fallback trigger and replace its legacy backend/method flags
  with 4.x tier/OCR flags.
- Update ThirdParty setup/version/update tasks and the parser README so the
  documented install path and CLI match 4.x.

## Verification

Do not use the two unrelated untracked PDFs in `ThirdParty/mineru` as fixtures.
Verify the installed CLI reports 4.0.9 and exposes the new stateless parse
command. Review the adapter against MinerU 4's documented Content List V1 and
MiddleJson V2 contracts and ChenWeb's existing converter contract. Do not run
automated tests or parse a document as a smoke test unless the user explicitly
requests testing/verification.

## Knowledge and documentation impact

- Update `ChenWeb/python/pdf-parser/README.md` for the new install and CLI
  behavior.
- Update the ThirdParty setup instructions/tasks to use the official 4.0.9 package
  and configuration model.
- Keep the Go converter and existing persisted JSON contract unchanged; no
  database migration is expected.
- The legacy upstream source files remaining in the local ThirdParty checkout
  must not be presented as the installed runtime if the package installation no
  longer uses them.

## References

- [MinerU migration guide](https://opendatalab.github.io/MinerU/reference/migration_4/)
- [MinerU output format contract](https://opendatalab.github.io/MinerU/reference/output_files/)
- [MinerU tiers and runtimes](https://opendatalab.github.io/MinerU/usage/tiers/)
