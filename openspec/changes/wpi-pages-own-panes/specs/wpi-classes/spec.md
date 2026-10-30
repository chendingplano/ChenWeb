## MODIFIED Requirements

### Requirement: Instances live in pages and are merged with their class
An element instance SHALL be written in a panel's `elem_list` as
`{"element": "<class>", "class": <whole class row>, ...}`; an action instance SHALL be written
where it is used as `{"action": "<class>", "class": <whole class row>, ...}`. `element` or
`action` SHALL equal the copy's `descriptor_id`, and the copy SHALL be a valid class row of its
kind. The places that hold action instances are:

- a button's or menu item's `on_click`;
- a list's `on_row_click` or `source`;
- a page's `input` or `save_action`;
- an `open-page`'s `input`.

An instance SHALL NOT set class attributes. An action instance SHALL set only instance
attributes its class's code defines, plus `args`. An element instance SHALL set only instance
attributes its class copy names, plus `width`, `height`, `min_width` and `max_width`, and none
that its copy fixes. Every attribute whose default in the copy is `null` SHALL be set. The
interpreter SHALL use the merged instance: the copy's non-null defaults overlaid with the
instance's own attributes, then the copy's fixed values. For an element, a page-wide
`binding_mode` fills in before the merge when the copy names `binding_mode` and does not fix
it (capability `wpi-pages`). The stored class SHALL NOT be read for an instance: a change to
the class SHALL reach only instances written afterwards.

Each position SHALL accept only these classes:

- triggers: `save`, `open-page`, `close-page`, `save-page`;
- `source`: `load` or `query`;
- `input`: static data (an object without `action`), `load`, `query` or `load-file`;
- `save_action`: `save`.

#### Scenario: Two instances of one class
- **WHEN** a panel lists two `input` instances with their class copies, one with `"label": "User name"` and one with `"label": "Phone"`
- **THEN** both are drawn by the `input` code module, each with its own label and binding and the copy's default sizes

#### Scenario: Class changed after the page was written
- **WHEN** the `input` class's default `width` changes from `elastic` to `240px` after a page was written
- **THEN** that page's `input` instances keep the width of their copy

#### Scenario: Instance sets a class attribute
- **WHEN** an instance sets `"description": "x"`
- **THEN** writing the page fails with an error at that attribute

#### Scenario: Instance sets an attribute its class does not name
- **WHEN** a `button` instance's copy does not name `input_type` and the instance sets `"input_type": "email"`
- **THEN** writing the page fails with an error at `/panels/<pane>/elem_list/<i>/input_type`

#### Scenario: Name differs from the copy
- **WHEN** an instance has `"element": "button"` and a copy whose `descriptor_id` is `input`
- **THEN** writing the page fails with an error at that instance's `element`

#### Scenario: Wrong class in a position
- **WHEN** a list's `source` is `{"action": "save", ...}`
- **THEN** writing the page fails with an error naming the allowed classes `load, query`

#### Scenario: Static data at a trigger
- **WHEN** a button's `on_click` is `{"page": "customers"}`, an object without `action`
- **THEN** writing the page fails with a missing `action` error; only `input` positions take static data

## ADDED Requirements

### Requirement: The server copies classes into new instances
When a page is written, the server SHALL copy the latest active class row of the right kind
into every element instance, action instance, page layouter copy and element layouter copy
that has no `class`, before validating the page. Copies already present SHALL be left
unchanged. A class with no active row SHALL be an error at the instance's path. The write
response SHALL return the stored page.

#### Scenario: Hand-written page
- **WHEN** an admin PUTs a page whose instances and layouter copies have no `class`
- **THEN** the stored page holds a copy of each active class, and the response returns it

#### Scenario: Existing copy kept
- **WHEN** an admin PUTs a page whose `input` instance holds an older copy of the `input` class
- **THEN** the stored instance keeps that older copy

#### Scenario: Draft class
- **WHEN** a page names element class `slider`, which has only a draft row
- **THEN** writing the page fails with an error at that instance's `element`

### Requirement: Class attribute names
`class` SHALL NOT be the name of an attribute in a class row, since instances and layouter
copies use it for their class copy.

#### Scenario: Attribute named class
- **WHEN** an element class row names an attribute `class`
- **THEN** writing the class fails with an error at `/class`
