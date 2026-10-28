# wpi-app-shell Specification

## Purpose
Opening a Web Page Interpreter (WPI) app by its URL and running its shell: layout templates
and panes, panels shown and hidden in a pane, menu-item bindings and shared panels, closing
and replacing panels, refresh on show, and the warning before leaving with unsaved changes.
## Requirements
### Requirement: Open an app by URL
The route `/wpi/<appId>` SHALL load the app's bundle and draw the app. The URL SHALL hold only
the app ID. If the bundle request fails, the page SHALL show an error box listing the
errors instead of the app. The app's root SHALL allow text selection.

#### Scenario: App opens
- **WHEN** a logged-in user opens `/wpi/customer-admin`
- **THEN** the app is drawn with the panes and starting pages its descriptor names

#### Scenario: Invalid app
- **WHEN** the bundle request returns 422
- **THEN** the page shows each validation error and draws no panes

#### Scenario: Not logged in
- **WHEN** a user without a session opens `/wpi/customer-admin`
- **THEN** the browser is redirected to `/login`

### Requirement: Customers demo in Development
The `/development` navigation SHALL include Development → Demos → Customers, with
English and Chinese menu labels. Selecting it SHALL display `/wpi/customers-demo`
inside the dashboard content area. This app SHALL open a copy of the page from
`interpreter-page.md` §5 under the independent ID `demo-customers` in its single
`main` pane, retaining text selection. It SHALL NOT depend on the `customers` page
used by customer-admin, whose actions target that app's other panes.

#### Scenario: Show demo customers
- **WHEN** a user with access to Development → Demos selects Customers
- **THEN** the WPI interpreter shows “Demo - Customers” above the customer list
- **AND** the columns are user name, phone number and email, in that order
- **AND** the list loads at most 200 records from `kb.wpi_demo_customers`, ordered by user name ascending
- **AND** leaving and reopening the demo loads the records again

### Requirement: Layout templates and panes
The app SHALL divide the viewport into panes with its layout template. `single-pane` SHALL
provide pane `main`. `top-left-middle-right-bottom` SHALL provide `top`, `middle` and
`bottom` rows with the middle row split into `left`, `middle` and `right`. A region the app
descriptor does not mention SHALL take no space. Each pane SHALL apply its size, min/max
size, resizability and scroll settings.

#### Scenario: Omitted region
- **WHEN** the app descriptor does not mention `bottom`
- **THEN** no bottom region is drawn and the middle row takes its height

#### Scenario: Resizable pane
- **WHEN** pane `left` has `resizable` true and `min_width` 160px
- **THEN** the user can drag its edge, and it does not become narrower than 160px

### Requirement: One visible panel per pane
A pane SHALL show at most one panel at a time; a pane SHALL be empty until a page is
opened in it. Opening a page in a pane SHALL create a panel with a new panel instance ID
and its own input doc.

#### Scenario: Same page twice
- **WHEN** customer 7 and customer 9 are opened from list rows, one after the other, in pane `right`
- **THEN** each is a separate panel with its own input doc

### Requirement: Menu bindings and shared panels
When a menu item's `open-page` runs, the resulting panel SHALL be bound to that item.
Clicking the item again while its panel exists SHALL show that panel without reloading the
page. Menu items whose `open-page` resolves to the same page with the same input SHALL be
bound to the same panel. The menu SHALL open its `initial_item` when the app starts.

#### Scenario: Switching back keeps the page
- **WHEN** the user types into the *New customer* form, clicks *Customers*, then clicks *New customer* again
- **THEN** the same form panel is shown with the typed values, without reloading

#### Scenario: Two items, one panel
- **WHEN** two menu items both open page `customers` in pane `middle` with input `{}`
- **THEN** clicking either shows the same panel

#### Scenario: Initial item
- **WHEN** the app starts and the menu's `initial_item` is `customers`
- **THEN** the `customers` page is opened in its target pane and bound to that item

### Requirement: Hidden versus closed panels
When a pane shows a different panel, the panel it was showing SHALL be hidden if a menu
item is bound to it, and closed otherwise. Closing SHALL go through the unsaved-changes
check; if the user chooses to keep editing, the new panel SHALL NOT be shown. A hidden
panel SHALL keep its input doc, including unsaved changes, and SHALL NOT trigger the
unsaved-changes check.

#### Scenario: Unbound panel with unsaved edits is replaced
- **WHEN** customer 7 (opened from a list row) has unsaved edits and the user clicks customer 9
- **THEN** the unsaved-changes prompt appears before customer 9 is shown

#### Scenario: Keep editing cancels the open
- **WHEN** the prompt appears and the user chooses "keep editing"
- **THEN** customer 7 stays visible with its edits and customer 9 is not opened

#### Scenario: Bound panel is hidden silently
- **WHEN** the *New customer* form has unsaved entries and the user clicks *Customers*
- **THEN** no prompt appears and the form is hidden with its entries kept

### Requirement: Refresh on show
A page whose descriptor sets `refresh.on_show` SHALL refresh each time its panel changes
from hidden to visible: its lists re-run their queries, and an input doc that came from a
load action is reloaded unless it has unsaved changes.

#### Scenario: List re-queries when shown again
- **WHEN** a customer is inserted from the *New customer* form and the user then clicks *Customers*
- **THEN** the customer list runs its query again and shows the new customer

#### Scenario: Unsaved edits survive a refresh
- **WHEN** a page with `refresh.on_show` and a dirty input doc becomes visible
- **THEN** its input doc is not reloaded

### Requirement: Leaving the app with unsaved changes
The app SHALL trigger the browser's leave confirmation when the user leaves or reloads the
page while any panel, visible or hidden, has unsaved changes.

#### Scenario: Hidden dirty panel
- **WHEN** the hidden *New customer* form has unsaved entries and the user reloads the page
- **THEN** the browser asks for confirmation before leaving

