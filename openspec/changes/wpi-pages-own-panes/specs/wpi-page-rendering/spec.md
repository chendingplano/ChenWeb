## MODIFIED Requirements

### Requirement: Flow layouter and sizes
The `flow-layouter` SHALL place a panel's elements left to right and wrap to a new row when
the next element does not fit. A fixed width SHALL be used as given; `elastic` widths SHALL
share the remaining row width within each element's `min_width` and `max_width`; `auto`
SHALL size the element to its content. Widths SHALL follow pane resizes and SHALL be measured
inside the pane's left and right margins.

#### Scenario: Elastic fills remaining space
- **WHEN** a row holds a 160px element and an elastic element with `max_width` 400px in a 500px pane without margins
- **THEN** the elastic element takes the rest of the row: 500px minus 160px and the gap between the two elements

#### Scenario: Wrapping
- **WHEN** the next element's minimum width does not fit in what is left of the row
- **THEN** it starts a new row

### Requirement: Page box
The interpreter SHALL draw a page's box with `width`, `height`, `min_width` and `min_height`
(each `50px` by default), `max_width` and `max_height`, measured inside its page area: the
app's page area, or the pane the page was opened in (capability `wpi-panes`). Without `width`
the page SHALL be as wide as its page area; without `height` it SHALL be as tall as its page
area when its root pane is split into columns or into rows with an `elastic` row, and
otherwise as tall as its content. In the app's page area, when the page is smaller than the
area, `hori_align` (default `center`) and `vert_align` (default `top`) SHALL place it, and
when it is bigger, `hori_scrollbar` and `vert_scrollbar` (`auto` by default, or `hide`) SHALL
decide whether it scrolls or is clipped; in a pane, that pane's attributes SHALL do this
instead. `bkgd_color` and `frgd_color` set to `auto` SHALL inherit the application theme, so
the page follows light and dark mode.

#### Scenario: Centred form in the app's page area
- **WHEN** a page sets `"max_width": "640px"` and is the initial page of an app 1200px wide
- **THEN** the page is 640px wide, centred in the page area

#### Scenario: Form in a pane
- **WHEN** the same page is opened in a pane 1000px wide with `"hori_align": "left"`
- **THEN** the page is 640px wide at the left of the pane

### Requirement: Flow spacing and alignment
A panel's `elem_layouter` for `flow-layouter` SHALL take its parameters from its own `config`,
or else from its class copy's `config`. The parameters SHALL be only `column_gap` and
`row_gap` (nonnegative decimal lengths in `px` or `rem`, default `0.75rem`),
`horizontal_alignment` (`start`, `center`, `end`, `space-between`, default `start`),
`vertical_alignment` (`top`, `center`, `bottom`, `stretch`, default `top`), and
`rows_alignment` (`top`, `center`, `bottom`, `space-between`, default `top`). Unknown keys,
non-object config and unsupported values SHALL be rejected at their descriptor paths. A
parameter missing from both SHALL take its default.

Horizontal alignment SHALL distribute leftover space independently in each row after
elastic growth and width limits. Row breaks and source order SHALL be preserved.
Vertical alignment SHALL position unequal-height items within each row; stretch SHALL
fill auto-height wrappers and their rendered element roots while retaining explicit
heights. A panel has no height of its own, so rows alignment has no extra height to
distribute; the pane's `vert_align` places the panel. Space-between SHALL retain configured
minimum gaps; a single item or row SHALL start at the start/top edge. Fixed oversized items
SHALL retain normal overflow.

#### Scenario: Exact fit and resized pane
- **WHEN** seven 240px elements have 16px gaps in a 1008px panel
- **THEN** four elements fit in the first row and three in the second
- **WHEN** the usable width becomes 1007px
- **THEN** only three elements fit in the first row, preserving source order

#### Scenario: Centered last row
- **WHEN** three 240px elements form the last row in 1008px with 16px gaps and horizontal center
- **THEN** the row has 128px of space on each side

#### Scenario: Panel config replaces the copy's
- **WHEN** a panel's `elem_layouter` has its own `config` with `"column_gap": "1rem"` and the other parameters
- **THEN** the panel uses 1rem column gaps, whatever its class copy's `config` says
