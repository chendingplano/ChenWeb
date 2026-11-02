## ADDED Requirements

### Requirement: Property editors are drawn from definitions

The WPI Editor SHALL draw the property editor of every page, pane, panel and element with one
shared renderer, from a definition kept as data in code for that kind of entity (one per element
code module, and one each for the page, panes and the flow-layouter panel). No kind of entity SHALL
have its own editor component. A definition SHALL list the entity's groups in order and, for each
property, its label, its value editor and the data that editor needs. Pulldown choices SHALL come
from the entity's class or code module where one exists. A property the definition does not
describe SHALL get a common-sense value editor chosen from its name and default value, and
attributes of the class that no group lists SHALL appear in an *Other* group.

#### Scenario: Element module editor from its definition
- **WHEN** the author selects an `input` element
- **THEN** the panel shows the groups General, Caption layout, Caption appearance, Input and help, Validation and Size in that order, and `input_type` is a pulldown of text, password, email, number, tel, url and search

#### Scenario: Common-sense editor for an undescribed attribute
- **WHEN** an element's class has an instance attribute `border_color` that its module's definition does not list
- **THEN** it appears in the Other group with the color picker

#### Scenario: Choices from action classes
- **WHEN** the author edits a button's `on_click`
- **THEN** the pulldown lists the active trigger action classes, and choosing *open page* stores `{"action": "open-page"}`

### Requirement: Same layout for every property editor

Every property editor SHALL show a title naming the entity, then its groups. Clicking a group's
heading SHALL fold or unfold it. Each property SHALL show a label, its value editor and its help
tip; a property every instance must set SHALL be marked with `*`; a property with its own value
SHALL offer *Reset*, which removes the value so the default applies. A property fixed by the class
SHALL be shown read-only and marked as fixed. Hidden properties SHALL keep their values. Titles,
labels, group names, choices and messages SHALL be available in English and Chinese.

#### Scenario: Fold a group
- **WHEN** the author clicks the heading of the page editor's Geometry group
- **THEN** its properties are hidden, and clicking it again shows them

#### Scenario: Reset an override
- **WHEN** a text element sets `font_size` to `18px` and the author clicks Reset beside it
- **THEN** `font_size` is removed from the instance and the editor shows the class or module default

### Requirement: Help tip icon

Every property SHALL have a help tip, which MAY be empty. Element help tips SHALL come from the
class's `help_tip` for the attribute, in the editor's language; page, pane and panel help tips
SHALL come from the definition. When a property's help tip is not empty, an info icon SHALL be
shown at the upper right of its value editor, and hovering over it or focusing it with the
keyboard SHALL show the tip. When the help tip is empty, no icon SHALL be shown.

#### Scenario: Element help tip
- **WHEN** the class of the selected input gives `placeholder` the help tip `{"en": "Shown when empty", "zh-cn": "为空时显示"}` and the editor is in Chinese
- **THEN** an info icon is at the upper right of the placeholder editor, and hovering over it shows 为空时显示

#### Scenario: No tip, no icon
- **WHEN** a property's help tip is empty
- **THEN** its value editor has no info icon

### Requirement: Value editors

Each property SHALL be edited with a reusable value editor: text, multi-line text, localized text
(one string, or separate English and Chinese versions), number, switch, value picker, ordered
multi-picker, color picker (none, one color, or separate light and dark colors), length (`auto` and
`elastic` where the property allows them, or a non-negative number in px, rem, em or % as the
property allows; a bare number means px), data source, sort order, column list, validation rules,
read-only value or JSON. A property with a fixed set of values SHALL use a pulldown, never a text
field. A value editor SHALL apply only values that are valid for the property. Invalid typed
text SHALL be kept, unapplied and unflagged, while the field has focus, so a half-typed value is
not reported; on leaving the field the editor SHALL restore the stored value and say why the text
was not applied. A warning that does not make the value invalid (for
example a width above `max_width`) SHALL be shown while the value is applied.

#### Scenario: Length typed without a unit
- **WHEN** the author types `240` into a pane's `width`
- **THEN** the pane's width becomes `240px` and the preview shows it at once

#### Scenario: Invalid length not applied
- **WHEN** the author types `12xy` into a page's `min_width` and leaves the field
- **THEN** the page keeps its previous `min_width`, and the editor says the value is not a valid length and was not applied

#### Scenario: Pulldown for a fixed set of values
- **WHEN** the author edits a pane's `hori_align`
- **THEN** the value editor is a pulldown with left, center, right and an unset choice

### Requirement: The panel shows the selected entity

The Properties panel SHALL show the property editor of the one selected entity: an element node,
or the page, a pane or a panel chosen with the *Page*, *Panes* and *Panel* controls or by clicking
the preview. *Panes* with no pane selected SHALL select the root pane. *Panel* SHALL show the
panel of the selected leaf pane. When nothing is selected, several nodes are selected, or the
selected pane is not a leaf in *Panel* mode, the panel SHALL show nothing. Nodes that are not WPI
entities (flow nodes, page and action boxes) SHALL keep their existing name and settings view.

#### Scenario: Several nodes selected
- **WHEN** the author ⌘-clicks two element nodes
- **THEN** the Properties panel is empty

#### Scenario: Panes with nothing selected
- **WHEN** the author clicks *Panes* and no pane is selected
- **THEN** the root pane is selected and its editor is shown

### Requirement: Changes apply at once and canvas changes show at once

A valid value SHALL take effect on the canvas as soon as it is entered, without leaving the field.
A change made on the canvas SHALL show in the property editor at once, including the page's width
and height after resizing the page, a pane's size after dragging its divider, and positions.

#### Scenario: Page resized on the canvas
- **WHEN** the page editor is open and the author drags the page's right edge to 900 px
- **THEN** the editor's `width` shows `900px` without reselecting the page

#### Scenario: Color applied while choosing
- **WHEN** the author changes a pane's `background_color` in the color picker
- **THEN** the preview shows the new color while the picker is still open

### Requirement: Positions are shown read-only

The editor of a selected element or pane SHALL show its position, x and y in page pixels from the
page's top-left corner, read-only, and SHALL update it when the entity moves on the canvas. The
page editor SHALL show no position.

#### Scenario: Element moves when a sibling grows
- **WHEN** an element's position is shown and the author makes the element before it taller
- **THEN** the shown y increases by the same amount

### Requirement: One undo step per focus

Everything the author changes in one property between giving it focus and leaving it SHALL be one
undo step, recorded before the first change. A change made without focus SHALL be one step. A
canvas action, undo or redo during a focus SHALL end that step, so a later change in the same
field records a new one.

#### Scenario: Typing a word
- **WHEN** the author focuses an element's `placeholder`, types `Name`, and leaves the field
- **THEN** one undo restores the previous placeholder

#### Scenario: Two fields
- **WHEN** the author changes `width` and then `height`, leaving each field
- **THEN** the first undo restores `height` and the second restores `width`

### Requirement: Value editors are documented

Each element code module document, and the page, page layouter and flow layouter documents, SHALL
list for every attribute its group, its value editor and where that editor's data comes from. An
attribute the document does not describe SHALL use the common-sense value editor. Every value
editor that is not very simple SHALL be a reusable module in the module catalogue.

#### Scenario: Module document lists value editors
- **WHEN** a developer opens `interpreter-list.md`
- **THEN** its Property editor table gives `source` the data-source editor and `columns` the column-list editor
