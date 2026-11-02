## ADDED Requirements

### Requirement: Value-editor element classes
The system SHALL store one element class in `kb.wpi_descriptors` for each value editor in
devdoc 2026102701 §1.3.9, 28 in all, each naming code module `value-editor`. The following
23 classes SHALL be `active`:
- the built editors: `text-editor`, `multiline-text-editor`, `localized-text-editor`,
  `number-editor`, `switch`, `value-picker`, `ordered-multi-picker`, `string-list`,
  `color-picker`, `length-editor`, `data-source-editor`, `column-list-editor`,
  `validation-editor`, `read-only-value`, `json-editor`;
- the variants: `font-picker`, `alignment-picker`, `icon-picker`, `action-picker`,
  `layouter-picker`, `table-picker`, `field-picker`, `sort-order-editor`.

`date-picker`, `time-picker`, `button-type-picker`, `database-picker` and `schema-picker`
SHALL be `draft`. Each class SHALL fix its `editor` attribute (class scope) to the editor's
ID as used in property editor definitions, and SHALL have a description in English and
Chinese. Code module `value-editor` SHALL be registered in `kb.code_modules` with module type
`element`.

#### Scenario: Built editor is active
- **WHEN** the class list is requested
- **THEN** `color-picker` is listed as `active` with `code_module_id` `value-editor`, and its `editor` is `color` with class scope

#### Scenario: Unbuilt editor is a draft
- **WHEN** the class list is requested
- **THEN** `date-picker` is listed as `draft` with `code_module_id` `value-editor`

#### Scenario: Page naming an unbuilt editor
- **WHEN** a page lists an instance of `time-picker`
- **THEN** the page is rejected, as for any draft class

### Requirement: Value-editor code module attributes
Code module `value-editor` SHALL require `editor`, `label`, `data_binding` and `binding_mode`.
It SHALL accept `editor_options` and the shared size attributes. `editor` SHALL be one of the
28 value-editor IDs. In an active class or a page instance, it SHALL be one of the 23 that
are drawn (the built editors and the variants). `editor_options` SHALL be a JSON object.
Validation SHALL report any other value as an issue at its path.

#### Scenario: Unknown editor
- **WHEN** an admin saves an active class with module `value-editor` and `"editor": {"default_value": "slider", "scope": "class"}`
- **THEN** the save is rejected with an issue at `/editor`

#### Scenario: Active class with an unbuilt editor
- **WHEN** an admin saves an active class with module `value-editor` and editor `date-picker`
- **THEN** the save is rejected with an issue at `/editor`

#### Scenario: Options must be an object
- **WHEN** a page instance of `value-picker` sets `"editor_options": ["a", "b"]`
- **THEN** the page is rejected with an issue at that instance's `editor_options`

### Requirement: Value-editor elements on a page
The interpreter SHALL draw a `value-editor` instance as its `label` above the value editor
named by `editor`, given `editor_options` as its options. The editor SHALL show the value at
`data_binding`. With `binding_mode` `read-write`, each value the editor applies SHALL be
written to that path. A value the editor unsets SHALL be written as `null`. With
`read-only`, the value SHALL be shown with the read-only value editor. Without a binding, the
editor SHALL keep its value locally. Server field errors for the binding SHALL be shown under
the editor.
- `font-picker` SHALL draw the text editor.
- `alignment-picker`, `icon-picker`, `action-picker` and `layouter-picker` SHALL draw the
  value-picker. Without `editor_options.choices`, `action-picker` SHALL offer every active
  action class and `layouter-picker` every page layouter class.
- `table-picker`, `field-picker` and `sort-order-editor` SHALL draw the Pick Table, Pick
  Fields and sort-order controls. `field-picker` SHALL list the fields of
  `editor_options.table`.

#### Scenario: Bound colour picker
- **WHEN** a page has a `color-picker` instance bound read-write to `$input_doc.theme.accent` and the user picks `#ff0000`
- **THEN** the input doc becomes `{"theme": {"accent": "#ff0000"}}`

#### Scenario: Read-only binding
- **WHEN** a `length-editor` instance is bound read-only to a field holding `"12px"`
- **THEN** `12px` is shown and cannot be edited

#### Scenario: Value picker choices from the instance
- **WHEN** a `value-picker` instance sets `"editor_options": {"choices": ["draft", "final"]}`
- **THEN** the pulldown offers `draft` and `final`

#### Scenario: Action picker without choices
- **WHEN** an `action-picker` instance has no `editor_options.choices`
- **THEN** its pulldown offers the active action classes

#### Scenario: Unset value
- **WHEN** the user clears a bound `number-editor` read-write
- **THEN** `null` is written to the bound path

### Requirement: VALUE EDITORS palette sub-folder
The WPI Editor's palette SHALL show a VALUE EDITORS sub-folder inside the ELEMENTS folder. It
SHALL list every element class whose `code_module_id` is `value-editor`, with drafts marked
as drafts. ELEMENTS SHALL list all other element classes.
- The sub-folder SHALL start folded and fold or unfold when its heading is clicked.
- It SHALL be hidden while ELEMENTS is folded.
- The count beside ELEMENTS SHALL include the classes in VALUE EDITORS.
- A search matching a class in VALUE EDITORS SHALL show both folders open.
- Dragging a class from VALUE EDITORS onto a pane SHALL add an element of that class, as from
  ELEMENTS.
- The Flow page's palette SHALL be unchanged.

#### Scenario: Sub-folder contents
- **WHEN** the editor opens and the author unfolds ELEMENTS, then VALUE EDITORS
- **THEN** VALUE EDITORS lists the 28 value-editor classes, with the five unbuilt ones marked as drafts, and ELEMENTS lists the other classes beside the folded or open sub-folder

#### Scenario: Search opens both folders
- **WHEN** the author types `color` in the palette search
- **THEN** ELEMENTS and VALUE EDITORS are open and `color-picker` is shown

#### Scenario: Drop a value editor
- **WHEN** the author drags `length-editor` from VALUE EDITORS into a pane of a loaded page
- **THEN** an element of class `length-editor` is added to that pane and selected

### Requirement: Value-editor properties
The Properties panel SHALL show a `value-editor` instance in the groups placement; General
(`label`, `data_binding`, `binding_mode`); Editor (`editor`, read-only; `editor_options`, as
JSON); and size. Changes SHALL show on the canvas preview at once. The Class tab SHALL edit
these classes as any element class.

#### Scenario: Edit options
- **WHEN** the author selects a `value-picker` element and sets `editor_options` to `{"choices": ["a", "b"]}`
- **THEN** the preview's pulldown offers `a` and `b`
