## ADDED Requirements

### Requirement: Product drawing on the results page

The Results tab of the Product Review results page SHALL display a generated reference drawing of the
reviewed product above the scope-tree/results/metric-details panes, reusing the existing product-drawings
generation flow (`/api/v1/product-drawings/generate`, `/pending/:token/keep`, `/pending/:token`). When the
profile has no associated drawing, the page SHALL show an inline generator (a prompt pre-filled from the
profile's name, description, keywords, and notes, editable before submission) instead of an image.

#### Scenario: Profile has no drawing yet

- **WHEN** the Results tab loads for a profile with no associated drawing
- **THEN** the drawing area shows the inline generator with a prompt pre-filled from the profile's name,
  description, keywords, and notes

#### Scenario: Generating a drawing shows a preview before persisting

- **WHEN** a user submits the inline generator
- **THEN** the pending generated image is shown with options to keep it or discard it, and no
  `kb.product_drawings` row is created until the user keeps it

#### Scenario: Kept drawing persists across visits

- **WHEN** a user keeps a generated drawing for a profile
- **THEN** the profile is associated with the kept drawing, and reopening the Results tab for that profile
  later shows the kept drawing directly without prompting to generate again

#### Scenario: Discarding a pending drawing allows retry

- **WHEN** a user discards a pending generated drawing
- **THEN** no `kb.product_drawings` row is created and the inline generator is shown again

#### Scenario: A profile with a kept drawing can be regenerated

- **WHEN** a user chooses to regenerate a drawing for a profile that already has one
- **THEN** the existing kept drawing keeps displaying until a new one is generated and kept, at which point
  the profile is associated with the new drawing

### Requirement: Resizable scope-tree, results, and metric-details panes

On the Results tab, the scope-tree pane, the results pane, and the metric-details pane SHALL be
independently resizable by dragging a vertical splitter between each adjacent pair of panes, and SHALL
expand together to fill the full remaining width of the window (or of the embedding dashboard content
area). Pane sizes SHALL persist per browser so they are restored on the next visit.

#### Scenario: Dragging a splitter resizes adjacent panes

- **WHEN** a user drags the splitter between the scope-tree pane and the results pane
- **THEN** the scope-tree pane's width changes accordingly and the results pane fills the remaining space

#### Scenario: Pane sizes persist across visits

- **WHEN** a user resizes a pane and later reopens the Results tab in the same browser
- **THEN** the panes render at the previously chosen sizes

#### Scenario: Panes fill the available width

- **WHEN** the Results tab is rendered at a wide viewport
- **THEN** the three panes together occupy the full remaining width, with no unused gap where a
  fixed-width cap previously stopped them

#### Scenario: Narrow viewports keep the existing stacked layout

- **WHEN** the Results tab is rendered below the existing single-column responsive breakpoint
- **THEN** the panes stack as they do today and the drag splitters are not shown

### Requirement: Drawing area is resizable in height

A horizontal splitter SHALL sit between the product drawing area and the row of three panes, letting a user
drag to change the drawing area's height. The chosen height SHALL persist per browser.

#### Scenario: Dragging the horizontal splitter resizes the drawing area

- **WHEN** a user drags the horizontal splitter below the drawing area
- **THEN** the drawing area's height changes accordingly and the pane row below it fills the remaining
  vertical space

#### Scenario: Drawing area height persists across visits

- **WHEN** a user resizes the drawing area and later reopens the Results tab in the same browser
- **THEN** the drawing area renders at the previously chosen height

### Requirement: App-shell context panel is hidden for Product Review

The app shell's generic context panel ("APP STATUS") and its open/close toggle SHALL NOT be shown while Product Review is the active dashboard content (nav child id `apps-product-review`, covering both the intake and results views), and no other dashboard page's context panel behavior SHALL change.

#### Scenario: Context panel is absent while viewing Product Review

- **WHEN** a user navigates to Product Review inside the dashboard shell
- **THEN** the context panel and its toggle button are not rendered, and the content area uses the space
  that would otherwise be reserved for it

#### Scenario: Context panel returns on other pages

- **WHEN** a user navigates away from Product Review to another dashboard page
- **THEN** the context panel and its toggle are shown again, respecting whatever open/closed state the user
  last had on that other page
