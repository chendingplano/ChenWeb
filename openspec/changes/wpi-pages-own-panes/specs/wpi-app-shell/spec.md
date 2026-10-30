## MODIFIED Requirements

### Requirement: Open an app by URL
The route `/wpi/<appId>` SHALL load the app's bundle and draw the app. The URL SHALL hold only
the app ID. The app SHALL open its `initial_page` in the app's page area. If the bundle request
fails, the page SHALL show an error box listing the errors instead of the app. The app's root
SHALL allow text selection.

#### Scenario: App opens
- **WHEN** a logged-in user opens `/wpi/customer-admin`
- **THEN** the app draws its initial page `customer-admin-home` with that page's panes

#### Scenario: Invalid app
- **WHEN** the bundle request returns 422
- **THEN** the page shows each validation error and draws no page

#### Scenario: Client cancels a bundle request
- **WHEN** bundle loading fails after the HTTP request context is canceled
- **THEN** the server records an informational cancellation and does not write a server-error response
- **AND** bundle-loading failures on requests that have not been canceled remain errors

#### Scenario: Not logged in
- **WHEN** a user without a session opens `/wpi/customer-admin`
- **THEN** the browser is redirected to `/login`

### Requirement: Customers demo in Development
The `/development` navigation SHALL include Development → Demos → Customers, with
English and Chinese menu labels. Selecting it SHALL display `/wpi/customers-demo`
inside the dashboard content area. This app SHALL open the single-pane page
`demo-customers` as its initial page, retaining text selection. It SHALL NOT depend on the
`customers` page used by customer-admin, whose actions target that app's other panes.

#### Scenario: Show demo customers
- **WHEN** a user with access to Development → Demos selects Customers
- **THEN** the WPI interpreter shows “Demo - Customers” above the customer list
- **AND** the columns are user name, phone number and email, in that order
- **AND** the list loads at most 200 records from `kb.wpi_demo_customers`, ordered by user name ascending
- **AND** leaving and reopening the demo loads the records again

#### Scenario: Embedded page follows the site theme
- **WHEN** the Customers demo is open and the user switches between light and dark mode
- **THEN** the dashboard and embedded WPI page update their backgrounds and text colours together
- **AND** the running page and loaded records are preserved without reloading the iframe
- **AND** other open same-origin tabs follow the selected mode

### Requirement: Refresh on show
A page whose descriptor sets `refresh.on_show` SHALL refresh each time its page instance
changes from hidden to shown: its lists re-run their queries, and an input doc that came from
an action is reloaded unless it has unsaved changes.

#### Scenario: List re-queries when shown again
- **WHEN** a customer is inserted from the *New customer* form and the user then clicks *Customers*
- **THEN** the customer list runs its query again and shows the new customer

#### Scenario: Unsaved edits survive a refresh
- **WHEN** a page with `refresh.on_show` and a dirty input doc is shown again
- **THEN** its input doc is not reloaded

### Requirement: Leaving the app with unsaved changes
The app SHALL trigger the browser's leave confirmation when the user leaves or reloads the
page while any page instance, shown or hidden, has unsaved changes.

#### Scenario: Hidden dirty page
- **WHEN** the hidden *New customer* form has unsaved entries and the user reloads the page
- **THEN** the browser asks for confirmation before leaving

## ADDED Requirements

### Requirement: Menus open pages in panes
A menu item SHALL open its page through its `on_click` `open-page`, in the pane that
`open-page` resolves to (capability `wpi-panes`). A menu SHALL open its `initial_item` when the
page holding the menu is rendered. A menu item SHALL be shown as current while its target pane
shows a page instance of the item's page with the item's input.

#### Scenario: Initial item
- **WHEN** `customer-admin-home` is opened and its menu's `initial_item` is `customers`
- **THEN** page `customers` is opened in the home page's `middle` pane and the *Customers* item is current

#### Scenario: Two items, one page instance
- **WHEN** two menu items both open page `customers` in pane `middle` with input `{}`
- **THEN** clicking either shows the same page instance

## REMOVED Requirements

### Requirement: Layout templates and panes
**Reason**: Apps no longer divide the screen; each page divides its own space with a page layouter.
**Migration**: Use a page layouter in the page (capability `wpi-panes`); the migration turns the `customer-admin` app's panes into page `customer-admin-home`.

### Requirement: One visible panel per pane
**Reason**: Replaced by the Panel Cache, which keeps page instances opened in panes.
**Migration**: See requirement "Panel Cache" in capability `wpi-panes`.

### Requirement: Menu bindings and shared panels
**Reason**: The Panel Cache finds a page instance by page and input, so menus need no bindings.
**Migration**: See requirements "Menus open pages in panes" and, in capability `wpi-panes`, "Panel Cache".

### Requirement: Hidden versus closed panels
**Reason**: A pane that switches pages now hides the page it showed and never closes it.
**Migration**: See requirement "Panel Cache" in capability `wpi-panes`.
