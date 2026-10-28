## MODIFIED Requirements

### Requirement: Validation rules
The validator SHALL check each descriptor in its kind's format: `schema_version` `"0.4"`
for element class rows, `"0.2"` for apps, pages and action class rows:

- structure: required attributes present, no unknown attributes, values of the allowed
  kinds and enumerations, `descriptor_id` matching `^[a-z0-9][a-z0-9-]{0,127}$`;
- capabilities: every layout template and layouter is one the interpreter supports, every
  active action class has code, and every active element class names a code module that has
  code and is an active element module in `kb.code_modules` (an element class draft is
  checked for structure and against `kb.code_modules`, capability
  `wpi-element-class-authoring`);
- class rows: as in capabilities `wpi-classes` and `wpi-code-modules`; an attribute that a
  class's code module does not define produces a warning, not an error;
- instances (page write and bundle, checked against the active class rows):
  - every instance names a class that has an active row;
  - an element instance sets only attributes its class names, plus sizes, and none its
    class fixes;
  - every attribute whose default in its class row is `null` is set;
  - the merged instance has every attribute the code requires and passes the
    cross-attribute rules: a `text` element has exactly one of `text` and `data_binding`, and
    a menu's `initial_item` is an item that opens a page;
- references: every referenced page and class exists (bundle only);
- panes: every pane named by the app or an `open-page` instance exists in the app's layout
  template (bundle only);
- tables: every table and column named by a `load` or `save` instance exists in the
  database (bundle only);
- bindings and references: well-formed paths, no `__proto__`, `constructor` or
  `prototype` segment, and each reference root used only where allowed;
- consistency: an attribute of a page's `save_action` that no `read-write` element on that
  page binds produces a warning, not an error.

#### Scenario: Unknown pane
- **WHEN** an `open-page` instance targets pane `side` in an app using `top-left-middle-right-bottom`
- **THEN** the bundle request returns 422 naming the page, the instance's path and the pane

#### Scenario: Missing column
- **WHEN** a `save` instance maps an attribute to a column that does not exist in its table
- **THEN** the bundle request returns 422 naming the page, the table and the column

#### Scenario: Forbidden path segment
- **WHEN** an element instance's `data_binding` is `$input_doc.__proto__.x`
- **THEN** validation reports an error for that instance

#### Scenario: Reference root outside its place
- **WHEN** a `load` instance's `where` uses `$input_doc.id` instead of `$args.id`
- **THEN** validation reports an error for that instance

#### Scenario: Unsupported schema version
- **WHEN** a descriptor has `schema_version` `"0.1"`
- **THEN** validation reports that this interpreter does not support that version

#### Scenario: Unbound save attribute
- **WHEN** a page's `save_action` maps attribute `email` but no `read-write` element on the page binds `$input_doc.email`
- **THEN** the bundle is returned with a warning naming the page and the attribute

#### Scenario: Element class row in an old format
- **WHEN** an element class row has `schema_version` `"0.3"`
- **THEN** validation reports that this interpreter supports `"0.4"` for element descriptors

#### Scenario: Page write checks instances against classes
- **WHEN** an admin PUTs a page whose `input` instance does not set `data_binding`, which the `input` class requires
- **THEN** the response is 400 with an error at that instance's path
