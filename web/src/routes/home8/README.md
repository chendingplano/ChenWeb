# home8 — Jenny Gu handmade crochet storefront

An Etsy-style storefront for Jenny Gu (hobby: crochet), built as a single
client-rendered SvelteKit route (`ssr=false`). No backend required: catalog is
in-memory; reviews, favorites, basket, and patterns persist to `localStorage`.

## Views (in-app, state-driven, no reload)

- **Home** — maker story, featured pieces, categories, about, how-it's-made, newsletter.
- **Shop** — Jenny's items for sale, with category filter, sort, and search.
- **Gallery** — every piece including guest makers, in a masonry grid with a maker filter.
- **Product** — Etsy-like detail: image gallery, price, ratings, specs, "meet the maker",
  rating breakdown, and a working customer review form.
- **Pattern Studio (admin)** — list and manage crochet patterns (add / edit / delete,
  difficulty + status, search, status summary).

## Design notes (impeccable, brand register)

- **Palette**: committed warm "riso craft-fair" direction — marigold + raspberry on warm
  paper, sage used only as a small accent to avoid the beige-and-sage craft cliché. OKLCH throughout.
- **Type**: Bricolage Grotesque (display) + Hanken Grotesk (body) + Caveat (handmade accents).
  All chosen off the reflex-reject list.
- **Imagery**: every product has a distinct generative crochet ripple-stitch swatch (SVG),
  so the storefront is fully offline-reliable and on-brand. A real Unsplash maker photo loads
  on the About section with a graceful `onerror` fallback to the swatch.

## localStorage keys

`jg_favorites`, `jg_cart`, `jg_reviews`, `jg_patterns`. Clear these to reset to seed data.

Checkout is intentionally not wired up (demo storefront).
