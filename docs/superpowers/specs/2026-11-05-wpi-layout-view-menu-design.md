# WPI Layouter View Menu

## Context

The WPI Editor's floating toolbar appears when one page element is selected in
the rendered page preview. The Properties panel currently shows that element's
instance properties. Its Class tab can edit the selected element's class, and
the same class editor already supports page-layouter and element-layouter
targets in other editor contexts.

## Design

Add a view icon to the floating toolbar. Its dropdown contains **Page Layouter**
and **Element Layouter**. The App option is out of scope.

Choosing either item keeps the selected element selected, switches the
Properties panel to its Class tab, and points the existing class editor at the
corresponding descriptor:

- **Page Layouter** uses the active page's `page_layouter.layouter` descriptor.
- **Element Layouter** uses `panels[selectedElement.pane].elem_layouter.layouter`.

The toolbar reports the chosen view to the editor. The editor resolves the
descriptor target and passes it to the existing `WpiClassPanel` and
`EntityClassEditor`; it does not create a second layouter editor or change the
page data model. The selected element remains available as the context for the
element's pane. If the selected element changes while a layouter view is active,
the selected layouter view continues to follow the selection. If the new
selection has no usable target for that view, the editor returns to the selected
element's default Class target.

The chosen view persists while the same page remains open, including across
element selection changes. It resets to the default element view when selection
is cleared or the editor switches to another page or tab.

The menu labels, accessible name, and tooltip use Paraglide messages in English
and Chinese. Existing editor behavior remains the default before a layouter view
is chosen.

## Missing targets

If the page or pane has no non-empty layouter descriptor ID, the corresponding
menu item is disabled. The menu remains available for the other target when
possible. A non-empty ID that cannot be loaded is handled by the existing class
editor's unavailable-class state.

## Acceptance criteria

1. Selecting a page element shows a view icon in its floating toolbar.
2. The menu offers Page Layouter and Element Layouter in both supported locales.
3. Page Layouter opens the active page's layouter descriptor in the Properties
   panel's Class tab without clearing element selection.
4. Element Layouter opens the selected element's pane layouter descriptor in the
   same panel and keeps the selection.
5. Switching selected elements while a layouter view is active resolves the
   layouter for the newly selected element.
6. Missing or empty layouter IDs disable only their corresponding menu item; if
   such a target becomes unavailable after selection changes, the panel falls
   back to the selected element's default Class target.
7. The selected layouter view persists across selections on the same page and
   resets when selection is cleared or the page or tab changes.
