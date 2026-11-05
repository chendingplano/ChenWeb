## Decisions

- **D1 One page per app, by ID.** `initial_page` stays the only page reference; no page list.
  Pages are looked up by `descriptor_id` in `kb.frontend_pages`; nothing is copied into the app.
- **D2 Separate table, not a kind.** Pages already have their own table; apps follow the same
  pattern (store routes by kind: page, app, else `kb.wpi_descriptors`). The admin write keeps
  `app` as its kind, so the URL does not change.
- **D3 Page must exist at save.** A dangling reference is refused with 400 rather than stored;
  the bundle still reports a page removed later.
- **D4 Migration.** The slug stood in as `app_name` for migrated embedded apps because the real name
  was never stored. Page versions holding `app` are rewritten in place (as migration
  20261030000001 did); only the latest active version of a page yields an app row.
- **D5 Form fields.** The editor's app form drops `scope`, `access_controls`, `pane_layouter`,
  `app_image`, `app_icon`: the server never supported them, and an app does not deal with panes.

## Not built

Loading a stored app into the editor; a page picker for `initial_page`; app URL shape for deep links.
