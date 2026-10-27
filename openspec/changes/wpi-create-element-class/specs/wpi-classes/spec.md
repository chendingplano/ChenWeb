## MODIFIED Requirements

### Requirement: Classes are named after their code
Element classes SHALL be stored as `kind = 'element'` rows and action classes as
`kind = 'action'` rows. A class's `descriptor_id` SHALL be the name under which the
interpreter registers the code that implements it. The element classes SHALL be `text`,
`input`, `select`, `button`, `list` and `menu`; the action classes SHALL be `load`, `save`,
`open-page`, `close-page` and `save-page`. An `active` class row whose ID has no code in the
interpreter, and an instance naming such a class, SHALL be validation errors. An element
class with no code MAY be stored as a `draft` through the create endpoint (capability
`wpi-element-class-authoring`); a draft is never used by the interpreter.

#### Scenario: Class without code
- **WHEN** an admin PUTs an element class row `slider`
- **THEN** the response is 400 saying element class `slider` has no code in this interpreter

#### Scenario: Instance of an unknown class
- **WHEN** a page lists `{"element": "slider"}`
- **THEN** writing the page fails with an error at `/elements/<i>/element`

#### Scenario: Draft class without code
- **WHEN** an admin creates element class `slider` through `POST /api/v1/wpi/classes/element`
- **THEN** it is stored as a `draft`, and the interpreter does not use it
