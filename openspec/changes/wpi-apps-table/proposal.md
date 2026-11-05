## Why

A WPI app was stored in `kb.wpi_descriptors`, but the editor's Create App dialog actually put an
app's identity in a root attribute `app` of its first page, and saving an app descriptor was not
supported. That ties an app to one page, although several apps may want to use the same page.

## What Changes

- New table `kb.wpi_apps` (same shape as `kb.frontend_pages`) holds app descriptors, versioned.
  Existing app rows move out of `kb.wpi_descriptors`; its kind check drops `app`.
- An app names its page by descriptor ID (`initial_page`) and may carry `app_name`,
  `app_description`, `keywords`, `tags`. Saving an app is refused when `initial_page` has no
  stored page.
- **BREAKING:** the page root attribute `app` is removed. The migration turns each into an app row
  and moves its name, description, keywords, tags and size to the page root.
- Editor: Create App saves an app descriptor; Create Page's dialog names a page, not an app.

Design: devdoc 2026102601 §3.1.3, §4.1, §6.

## Impact

`server/api/wpi` (store, handler, validate), migration `20261105000001_create_kb_wpi_apps.sql`,
the WPI editor, messages, devdoc 2026102601.
