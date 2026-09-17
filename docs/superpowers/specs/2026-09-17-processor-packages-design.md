# Processor Packages Design

## Goal

Keep document-processing settings local and let the Development manual-launch UI select named processor packages while preserving the complete checkbox list.

## Design

`config.toml` will contain no `[doc-processing]` section. The same processing settings and `[doc-processing-packages]` definitions will live in `config.local.toml`, which is already merged by the server config loader. The KB config endpoint will expose package names and processor IDs alongside the existing processor catalog.

The manual-launch UI will render a `Select Processors` pulldown from those packages. Selecting a package updates only checkbox state; all configured processor rows remain visible. The `Default` package is special: its launch publishes a line-file event without an `operation` field, allowing the backend to resolve `default_processors` through the same path used after PDF ingestion. Other packages publish their checked processor IDs as explicit operations. Users can still adjust checkboxes after choosing a package.

Mandatory stages remain checked and visible. Package names and processor IDs are normalized only for comparison; configured display/order is preserved. Missing or invalid package configuration falls back to an empty package list without breaking the existing processor UI.

## Verification

Add Go tests for local config/package parsing and TypeScript tests for package checkbox mapping and the special Default launch payload. Run the relevant Go tests, `go vet`, frontend type checking, and frontend unit tests/build.
