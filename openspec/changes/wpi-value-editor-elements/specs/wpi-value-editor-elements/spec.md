## ADDED Requirements

### Requirement: Value-editor element classes
The system SHALL store one element class in `kb.wpi_descriptors` for each value editor in
devdoc 2026102701 §1.3.9, 29 in all, each naming code module `value-editor`. The following
24 classes SHALL be `active`:
- the built editors: `text-editor`, `multiline-text-editor`, `localized-text-editor`,
  `number-editor`, `switch`, `value-picker`, `ordered-multi-picker`, `string-list`,
  `color-picker`, `length-editor`, `data-source-editor`, `column-list-editor`,
  `validation-editor`, `read-only-value`, `json-editor`;
- the variants: `font-picker`, `alignment-picker`, `icon-picker`, `emoji-picker`, `action-picker`,
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
Code module `value-editor` SHALL require `editor`, `label` and `binding_mode`. It SHALL
accept `data_binding`, `editor_options` and the shared size attributes. Each class SHALL give
`label` a default (the editor's name) and leave `data_binding` without one, so a dropped
instance is drawn at once and keeps its value locally until it is bound. `editor` SHALL be one of the
29 value-editor IDs. In an active class or a page instance, it SHALL be one of the 24 that
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
- `font-picker` SHALL draw the text editor and SHALL apply only `inherit` or a CSS
  font-family list.
- `alignment-picker`, `action-picker` and `layouter-picker` SHALL draw the value-picker.
  Without `editor_options.choices`, `action-picker` SHALL offer every active action class
  and `layouter-picker` every page layouter class.
- `icon-picker` SHALL draw the icon library's search picker, in the page's language. It
  SHALL store the chosen icon's name and SHALL show the stored icon.
- `emoji-picker` SHALL draw the shared emoji search picker in the page's language and SHALL
  store and display the complete Unicode emoji string.
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

#### Scenario: Pick an icon by meaning
- **WHEN** an `icon-picker` instance is bound read-write to `$input_doc.icon`, and the user types `reload` (or `刷新`) and picks the refresh icon
- **THEN** the input doc becomes `{"icon": "refresh-cw"}`, and the picker shows that icon

#### Scenario: Pick an emoji by meaning
- **WHEN** an `emoji-picker` instance is bound read-write to `$input_doc.emoji` and a user finds `🎉` using `庆祝` and selects it
- **THEN** the input doc becomes `{"emoji": "🎉"}` and the picker shows that emoji

#### Scenario: Font family rule
- **WHEN** the user types `Arial; color: red` into a bound `font-picker`
- **THEN** the value is not applied, and the editor says why

### Requirement: Icon picker editor ID
The value editor ID `icon-picker` SHALL draw the icon library's search picker wherever it is
used. This includes an attribute that a class configures with `icon-picker` in the
Properties panel. Its search box, empty result and error SHALL be shown in English and
Chinese.

#### Scenario: Class attribute configured with icon-picker
- **WHEN** a class sets an attribute's value editor to `icon-picker` and the author selects an instance of that class
- **THEN** the Properties panel shows the icon search picker for that attribute, not a pulldown

### Requirement: VALUE EDITORS palette folder
The WPI Editor's palette SHALL show a VALUE EDITORS folder directly after the ELEMENTS
folder. It SHALL list every element class whose `code_module_id` is `value-editor`, with
drafts marked as drafts. ELEMENTS SHALL list all other element classes.
- VALUE EDITORS SHALL start folded and SHALL fold, count and open on search like the other
  folders.
- Classes in VALUE EDITORS SHALL be treated as element classes. Dragging one onto a pane
  SHALL add an element of that class. Selecting it SHALL show its properties and its Class
  tab.
- The Flow page's palette SHALL be unchanged.

#### Scenario: Folder contents
- **WHEN** the editor opens and the author unfolds VALUE EDITORS
- **THEN** it lists the 29 value-editor classes, with the five unbuilt ones marked as drafts, and ELEMENTS does not list them

#### Scenario: Folder order
- **WHEN** the editor opens
- **THEN** the palette's folders end with PAGES, ELEMENTS, VALUE EDITORS, in that order

#### Scenario: Search
- **WHEN** the author types `color` in the palette search
- **THEN** VALUE EDITORS is open and shows `color-picker`

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
