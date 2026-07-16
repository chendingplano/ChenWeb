# Product

## Register

product

Note: routes under `/semos` (home, about, marketing pages) are **brand** surfaces; routes under `/home3` and `/semos/workspace` (knowledge base, document review, dashboards) are **product** surfaces. Pick per task.

## Users

Enterprise knowledge workers in Chinese organizations: compliance reviewers, standards engineers, procurement and bid teams, and the CIOs who evaluate SemOS as a vendor. Bilingual UI (zh-CN primary, en secondary). They arrive with large heterogeneous document sets (PDF, Word, PPT, standards, contracts) and need trustworthy, citable answers, not chat toys.

## Product Purpose

SemOS (知识通, branded Bzton/标准通 for some tenants) turns documents into searchable, reviewable, actionable, interlinked knowledge: parsing and "smelting" documents into artifacts (summaries, metrics, provisions, entities, graphs), hybrid search (BM25 + vector + graph + SQL), multi-dimensional document review, and a low-code AI application platform. Success: a visitor trusts the platform enough to upload their organization's documents; a reviewer trusts the citations enough to sign off.

## Brand Personality

Scholarly, precise, quietly confident. "Paper and ink": warm paper grounds, ink text, a single bronze accent used like a bookbinder's foil stamp. Feels like a well-made reference volume, not a SaaS dashboard. Emotional goals: trust, calm, craftsmanship.

## Anti-references

- Generic AI-SaaS landing pages: purple gradients, glassmorphism, floating 3D blobs, gradient text.
- Crypto/startup neon-on-black maximalism.
- Identical icon-card grids and hero-metric stat bands with fake numbers.
- Consumer-app playfulness; this audience signs procurement contracts.

## Design Principles

1. **Evidence over adjectives** — show structure, citations, and provenance; the design itself should feel audited and traceable.
2. **One accent, spent deliberately** — bronze (#b08d57) marks passage and emphasis (ornaments, numerals, kickers); it never floods.
3. **Editorial rhythm** — alternating typographic densities, ornament dividers instead of rules, numbered sections like a bound volume.
4. **Config is copy** — all page copy lives in site-config TOML (per-tenant); components render whatever the tenant ships. i18n messages cover only chrome (nav, buttons).
5. **Both themes, always** — every surface ships light and dark from the first commit.

## Accessibility & Inclusion

- Honor `prefers-reduced-motion` (the reveal pattern already does; keep it).
- Visible focus rings (`focus-visible`) on all interactive elements.
- CJK typography: no tight tracking on Chinese body text; keep line-height ≥ 1.7 for zh body copy.
- Contrast: ink on paper and paper on ink both ≥ WCAG AA.
