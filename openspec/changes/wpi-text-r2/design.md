## Context

The human approved KnowledgeStore `interpreter-text.md` R2. Text class version 4 is draft; version 3 remains active. Existing instances carry independent class copies.

## Goals / Non-Goals

Implement the approved attribute/default contract, caption/icon/tooltip, localized content and editor. Preserve string conversion and old copies. Rich text, actions, formatting and Page Styles remain excluded.

## Decisions

- Extend `TextElement.svelte` and module-specific validators; do not change generic merging or input behavior.
- Keep flat attributes and use the established Properties panel. Add a focused text property component to avoid expanding generic input logic.
- Share the finite color/icon vocabulary in JSON between Go and TypeScript; validate ranges and grammar on both sides.
- Use a viewport-constrained fixed tooltip in the top layer, with hover/focus/touch access, cancellable delayed closure and cleanup. Observe visibility to close it when cached ancestors hide.
- Apply border-box outer sizing and detect overflow for keyboard scrollability. Keep content-sized flow items measurable; avoid inline-size containment collapsing intrinsic width.
- Store a new active descriptor version via goose only after tests. Preserve prior versions and page copies.

## Risks / Trade-offs

Explicit sizes can be smaller than text → local scrolling and browser tests. Caption rearrangement can confuse reading order → keep caption first in DOM. Tooltip state can survive cached hiding → observe visibility and close. Literal colors can fail contrast → inherited defaults and theme checks.

## Migration Plan

Validate the approved descriptor with updated code; add the next active version and correct the code-module document path. Update existing frontend reusable-module row. Rollback deletes only the migration-owned active version and restores the prior module path; draft version 4 remains intact.
