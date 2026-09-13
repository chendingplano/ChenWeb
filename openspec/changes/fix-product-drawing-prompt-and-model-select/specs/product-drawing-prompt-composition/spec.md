## ADDED Requirements

### Requirement: Exploded-view prompt composition
The system SHALL compose a 3D exploded-view drawing prompt from a fixed
template file, a product name, and an optional list of product-specific
component names, rather than appending a static, product-agnostic
instruction string to arbitrary prompts.

#### Scenario: Product with known components
- **WHEN** the template is composed for product name "血压计" with components
  ["控制按钮", "电路板", "袖带"]
- **THEN** the resulting prompt starts with "Draw a 3D exploded technical
  illustration of 血压计." and includes a sentence naming those components
  as the parts the illustration should include

#### Scenario: Product with no known components
- **WHEN** the template is composed for a product name with an empty
  components list
- **THEN** the resulting prompt omits the "For instance..." component-listing
  sentence entirely, without leaving a malformed or empty sentence in its
  place

### Requirement: Prompt composition preview endpoint
The system SHALL expose a stateless endpoint that renders the composed
prompt for a given product name and component list without generating an
image or writing to the database, so callers can preview and edit the
default prompt before requesting generation.

#### Scenario: Preview request
- **WHEN** a client sends `POST /api/v1/product-drawings/compose-prompt`
  with `{"product_name": "血压计", "components": ["控制按钮", "电路板"]}`
- **THEN** the response is `200 OK` with a JSON body containing the composed
  `prompt` string, and no row is written to `kb.product_drawings`

### Requirement: Drawing generation uses the given prompt verbatim
The system SHALL send the prompt provided to the drawing-generation endpoint
to the image provider without appending any additional static instruction
text.

#### Scenario: Generation request with a fully composed prompt
- **WHEN** `POST /api/v1/product-drawings/generate` is called with a
  non-empty `prompt` field
- **THEN** the image provider receives exactly that prompt text, with no
  additional text appended by the server
