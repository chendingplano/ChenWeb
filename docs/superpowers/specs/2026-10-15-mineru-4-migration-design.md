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

Install official MinerU 4.0.9 in the existing ThirdParty-managed virtual
environment, retaining its workspace path and setup entrypoint. Use the stateless
`mineru-kit parse` command for complete-document conversion. Request the `standard`
tier for normal parsing and use the 4.x OCR mode with the `basic` tier for the
existing forced-OCR fallback. Parse the ZIP result and adapt its `middle_json`
pages and blocks into ChenWeb's existing consolidated JSON shape. Keep the current
image-copy behavior and progress/heartbeat reporting.

The adapter belongs in the Python parser. It is the boundary between MinerU 4's
MiddleJson v2 result and ChenWeb's established parser output. The Go converter,
stored JSON schema, server routing, and existing records remain on their current
contract. The fallback's configured language parameter must be reviewed against
the 4.x CLI; current 4.x documentation removes the old language option, so the
migration must not pass an obsolete `-l` argument.

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
- Preserve text/heading levels, list items and their boxes, tables, images,
  equations, captions, footnotes, and page furniture handling where the new
  output contract provides corresponding data.
- Resolve image references from the ZIP into the existing `images/` destination;
  fail parsing clearly if required structured output is missing or malformed.
- Preserve the OCR fallback trigger and replace its legacy backend/method flags
  with 4.x tier/OCR flags.
- Update ThirdParty setup/version/update tasks and the parser README so the
  documented install path and CLI match 4.x.

## Verification

Do not use the two unrelated untracked PDFs in `ThirdParty/mineru` as fixtures.
Verify the installed CLI reports 4.0.9 and exposes the new stateless parse
command. Review output adaptation against MinerU 4's documented MiddleJson v2
shape and ChenWeb's existing converter contract. Automated tests are outside this
change unless explicitly requested.

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
