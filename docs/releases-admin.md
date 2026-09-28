# Releases administration

The **Development → System Admin → System → Releases** page manages releases and their released items. The upper panel creates or edits a release; the lower panel lists all releases and expands each one to show its notes and items.

Each release has a major version, minor version, release notes, and a release date. A `(major_version, minor_version)` pair must be unique. Each item has a type (`bug fix`, `improvement`, or `new feature`), description, notes, pull request, and ticket number. A release can have no items. Deleting a release also deletes its items.

The list sorts major version first, then minor version, both descending with digit runs compared numerically (`12` before `2`). Other characters compare alphabetically without regard to case. Releases with equal natural versions use their ID as a stable tie breaker.

The admin-only JSON API is `GET/POST /api/v1/releases` and `PUT/DELETE /api/v1/releases/:id`. POST and PUT accept the release fields and an `items` array. They save the release and its complete item list in one database transaction; PUT replaces the previous item list. The project migration `20260928000003_create_releases.sql` creates `public.releases` and `public.release_items`. The existing startup project migrator applies it.
