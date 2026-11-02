## Requirements

### Class selection

The WPI Editor SHALL offer Properties and Class tabs. Class SHALL read the latest
draft or active definition from `kb.wpi_descriptors`, independently of an instance's
class copy. Elements select their element class; actions select their action class;
panes select their page layouter class; panels select their element layouter class.
Pages, flow-only nodes and multiple selections SHALL show no class editor. Action
boxes SHALL offer a selector populated from all active stored action classes.
The node toolbar's Edit class SHALL open the same Class tab.
The Class header SHALL display the stored record version and descriptor schema
version separately from the class's draft/implemented status.

### Class body JSON

With Class selected, Code → View JSON SHALL show the selected class body, including
unsaved edits in its attribute forms, without adding instance IDs or other instance
values. Applying edited JSON SHALL update that same class draft; Save class SHALL
persist it through the existing class-update endpoint. Descriptor identity and schema
version SHALL retain their read-only constraints. With Properties selected, View JSON
SHALL retain its existing instance/page behavior. The dialog's title and note SHALL
identify the edited class and explain that Apply changes the draft rather than the
stored body.

### Attribute tree

Every descriptor attribute SHALL be represented by a button labeled with its name.
Selecting the button SHALL fold or unfold an inline form and its nested attributes.
The form SHALL expose name, localized description, an Attribute scope dropdown
with Class attribute and Instance attribute choices, default
value, a value-editor selector, and an inline configuration form for that editor.
Element `default_value` wrappers SHALL be hidden from the logical tree. Nested
objects and arrays SHALL be traversable. Identity fields SHALL remain read-only;
array indices and built-in class attribute names SHALL not be renamed.

Defaults SHALL preserve JSON types. Empty input removes the default, `null` means
a required element instance attribute, and `""` is an empty string. Renames SHALL
reject reserved or duplicate names and move metadata for the renamed subtree.

### Definition metadata and value editors

Optional `attribute_definitions` SHALL map logical JSON pointers to objects with
optional `description`, `scope` (`class` or `instance`), and `value_editor`
(`{id, options?}`). It SHALL be validated independently and SHALL NOT become an
instance attribute or runtime default. Metadata SHALL survive class copying.
Known editor IDs and configuration option types SHALL be validated by the server.
Every attribute displayed in Class SHALL have explicit scope and value-editor ID
and options in `body.attribute_definitions`, so the form and class JSON share the
same values. Legacy descriptors missing definitions SHALL receive them in the
local draft and offer Save class to persist them; reading SHALL NOT silently
write the database. Existing explicit editor choices and options SHALL be kept.
Top-level element scope SHALL agree with the descriptor's fixed flag, correcting
stale metadata in the draft. Description edits SHALL update the help tip and
metadata together; clearing a plain description SHALL remove both fields.
Completeness and dirty checks SHALL ignore JSON object key order, including order
changes when PostgreSQL reads a JSONB body.

For top-level element attributes, Class SHALL set `instance_modifiable: false`,
require a non-null default and thereby retain the existing fixed-default behavior;
Instance SHALL remove that restriction. Built-in class attributes SHALL remain
Class attributes. For nested attributes and action/layouter descriptors, scope
SHALL be authoring classification only; the UI SHALL explain that distinction.

The selector SHALL contain the devdoc's complete value-editor list. Configuration
SHALL include relevant fields for number bounds, whole numbers, choices, length
units/keywords, rows, theme colours, validation type and date/time range, plus a
JSON object form for full options. Editors documented as unbuilt SHALL be marked.
The JSON options disclosure SHALL identify its content as the selected value editor's
options, explain that an empty object uses default options, and distinguish it from
the class-body JSON and attribute default value.
Standalone table/field/sort controls SHALL explain their JSON fallback.
Stored editor choices SHALL override code defaults for element property editors
and panel layouter configuration when the instance copy includes that metadata.
Changing to a different editor SHALL remove incompatible value conversions;
equivalent picker aliases SHALL retain existing conversions and dynamic choices.

### Saving and draft lifetime

Authenticated class reads SHALL support element, action, page-layouter and
element-layouter classes. Updates SHALL require an administrator, an existing row,
matching identity and a valid descriptor, retaining version, status and creation
audit fields. Validation errors SHALL leave both the stored row and local edits
intact. Existing element endpoints SHALL retain their behavior.

Save class SHALL remain disabled while unchanged with complete definitions, loading
or saving. Missing explicit definitions SHALL enable Save class with an explanation;
discarding user edits SHALL retain those definitions in the local draft. A successful
save SHALL clear only the submitted class's dirty marker and refresh the editor's
class lists without rewriting existing instance copies. Unsaved drafts SHALL be
retained while changing selection, editor/canvas tabs, sidebar views and preview.
Browser unload and route navigation SHALL warn before abandoning class drafts;
explicit discard SHALL ask for confirmation. UI text SHALL support English and
Chinese, and text selection SHALL remain enabled.
