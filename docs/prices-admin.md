# Price management

The **Development → System Admin → System → Price Management** page creates, views, modifies and deletes price definitions. The upper panel creates or edits a definition; the lower panel lists all definitions, filterable by type, and expands each one to show its price items. The design note is `KnowledgeStore/doc-repo/devdocs/202609/2026092801-devdoc-pricing-page.md`.

A price definition has a unique `price_def_name`, a `price_type` (`service` for prices charged to customers, `llm` for LLM provider prices) and an optional description. It holds one or more price items. Each item has an `item_name` (unique within its definition), `item_type` (`input` or `output`), `cache` (`hit`, `miss`, or empty for not applicable), `time_span` (`peak`, `off-peak`, or empty), a free-text `unit` (e.g. `million-tokens`), a `currency` code stored upper case (e.g. `CN`, `US`), and a non-negative decimal `value`. Values travel as decimal strings and are stored as `NUMERIC`, so `0.04` stays exact. Deleting a definition also deletes its items.

The admin-only JSON API is `GET/POST /api/v1/prices` and `PUT/DELETE /api/v1/prices/:id`. POST and PUT accept the definition fields and an `items` array and save both in one transaction; PUT replaces the previous item list. The project migration `20260928000004_create_prices.sql` creates `public.price_defs` and `public.price_items`.

Nothing reads these prices yet. LLM cost reporting (`server/api/llmreporthandler`) still uses its own built-in rates.
