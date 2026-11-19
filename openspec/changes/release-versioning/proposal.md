## Why

The Releases page records a hand-typed major/minor version with no link to the builds that
actually ship, and the Deployment page can only install the newest archive on the relay. We
need real release numbers that the build machine assigns, a running list of what changed
since the last build (bug fixes, improvements, features), and release notes that tell the
person deploying to production exactly what a chosen release adds over what is running now.

## What Changes

- **BREAKING** Replace the free-text `public.releases` / `public.release_items` tables (empty
  on both machines) with versioned release tables in `kb`. Versions are
  `<major>.<minor>.<revision>`, each part shown as two digits (`01.02.03`), kept per project.
  Only the project `ChenWeb` is enabled now; `shared` changes ship inside ChenWeb releases.
- The build machine (`[deployment] role = "build"`) owns versions and items. On every other
  machine the Releases page is read-only reference.
- Admins create a new **major** (`M+1.00.00`) or **minor** (`M.m+1.00`) with a description
  on the Releases page. `00` means "none created yet".
- **Release items** (type, component ChenWeb/shared, description, optional GitHub issue,
  pull request, notes) are added while work is done, from the page or from a command-line
  tool agents can use. Items wait as *pending* until a build takes them.
- Every **successful build** becomes a new revision (`M.m.r+1`): the build takes all pending
  items, embeds the version in the binaries and MANIFEST, writes release notes (plain text
  and PDF) into the archive, and uploads a small release file next to the archive on the
  relay. A failed build leaves the items pending and the version unused.
- The production machine records the release it is **currently running** and mirrors the
  release history it receives from archives.
- **BREAKING** The Deploy button lets the admin **pick any archive** on the relay instead of
  always the newest. For the chosen release the page lists every item since the running
  release (several releases may be skipped), warns when the choice is older (a rollback),
  and offers the same notes as PDF.
- Release notes can be viewed as text or PDF for one release or a range, on both machines.

## Capabilities

### New Capabilities
- `release-versions`: per-project version numbers, creating majors and minors, release
  items and their pending/released life cycle, role-gated editing, the item CLI.
- `release-build-integration`: how a build reserves, embeds, notes and commits a revision,
  and how a failed or interrupted build gives it back.
- `release-notes`: text and PDF notes for one release or a range of releases.
- `release-deployment`: the production machine's current release, the release history
  mirror, choosing a release to deploy, and notes since the running release.

### Modified Capabilities
None in `openspec/specs/`. The deploy behaviour this changes ("Deploy shows the archive
before it starts", which always offers the newest archive) is in the unarchived
`deployment-page` change; `release-deployment` replaces that requirement. Archive
`deployment-page` first or note the supersession when archiving it.

## Impact

- **Backend:** `server/api/releaseshandler/` rewritten; `server/api/deployment/` (archive
  listing, plan steps, release version on build and deployment rows); new
  `server/cmd/release-tool/` (next version, notes, commit, abort, add-item); `server/buildinfo`
  (release version in embedded metadata); server start-up import on the deploy machine.
- **Scripts:** `shell_server_build_rcp.sh` (reserve/notes/commit/abort steps, sidecar
  upload), `mise.toml` `build-server-linux` (embed version, MANIFEST lines, shared git rev),
  `shell_server_deploy.sh` (`--list`, fetch a release file, install `release/`).
- **Database:** new goose migration — drop the empty old tables (guarded), create
  `kb.releases`, `kb.release_items`, `kb.release_current`; add release version columns to
  `kb.deployment_builds` and `kb.deployments`. No cross-table or cross-machine IDs are
  introduced (releases are keyed by project + version, build numbers already exist), so the
  ID manager is not needed.
- **Frontend:** `releases-view.svelte` / `releases-client.ts` rebuilt; `deployment-view.svelte`
  (release picker, notes since running release, version in history); `build-info-view.svelte`
  (version); Paraglide messages (en, zh-cn).
- **Production box:** PDF rendering needs `pandoc`, `typst` and a CJK-capable font installed
  (root step for the user); the `chenweb` service imports release files at start.
- **Docs:** devdoc `2026111901-devdoc-release-management.md` is superseded; deployment devdoc
  `2026111602` and the runbook gain the release steps; `ChenWeb/CLAUDE.md` gains the rule to
  add a release item for each fix.
- **Reusable modules searched** (`find-reusable-module`: pdf render/generate, pdf viewer,
  version/release/markdown/typst): reuse **20261109-02 Document Builder (backend)** —
  `docbuilder.PandocConvert` for Markdown → PDF with the `DOCUMENT_PDF_ENGINE` /
  `DOCUMENT_PDF_FONT` settings; reuse **20261109-01 Markdown Viewer** to show notes on the
  page. **20261114-06 PDF View Window** was considered and not used (built for highlighted
  evidence; PDFs open in the browser's own viewer). Nothing in the catalogue handles version
  numbers or release items; that code is new and ChenWeb-specific for now (it moves to
  `shared/` when tax adopts it).
