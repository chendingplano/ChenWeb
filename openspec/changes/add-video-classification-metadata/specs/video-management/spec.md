## ADDED Requirements

### Requirement: Upload captures classification metadata

The video upload flow SHALL accept seven optional classification fields and persist them on `kb.videos`: `keywords`, `category`, `subcategory`, `container`, `status`, `notes`, and `video_type`. Omitted fields SHALL be stored as NULL and MUST NOT block the upload. List and detail responses SHALL expose all seven fields, falling back to safe defaults (empty string for text fields, `draft` for `status`) for legacy rows.

#### Scenario: Upload with all classification fields

- **WHEN** an operator uploads a video with `keywords`, `category`, `subcategory`, `container`, `status`, `notes`, and `video_type` supplied
- **THEN** the video is stored with those values and the response echoes each of them

#### Scenario: Upload omitting the classification fields

- **WHEN** an operator uploads a video providing only the file, name, and description
- **THEN** the upload succeeds and the new fields are stored as NULL, with responses returning empty strings for the text fields and `draft` for `status`

### Requirement: Status is constrained to a fixed set

The `status` field SHALL be one of `draft`, `published`, or `archived`, defaulting to `draft` when omitted. The server SHALL reject any other value with a `400` error and SHALL NOT persist the video.

#### Scenario: Status defaults to draft

- **WHEN** an operator uploads a video without specifying `status`
- **THEN** the stored and returned `status` is `draft`

#### Scenario: Invalid status is rejected

- **WHEN** an upload supplies a `status` outside `{draft, published, archived}`
- **THEN** the server responds with a validation error and no video row is created

### Requirement: Video type is auto-detected with an editable fallback

The upload dialog SHALL pre-fill `video_type` from the chosen file's extension (e.g. `mp4`) and allow the operator to override it. When the client sends no `video_type`, the server SHALL derive it from the stored filename's extension (lowercased, without the leading dot).

#### Scenario: Type inferred from the chosen file

- **WHEN** the operator selects `training.mp4` in the upload dialog
- **THEN** the `video_type` input is pre-filled with `mp4` before submission

#### Scenario: Server backfills a missing type

- **WHEN** an upload arrives with an empty `video_type` for a file named `clip.webm`
- **THEN** the stored `video_type` is `webm`

### Requirement: Category and subcategory suggest existing values

The upload dialog SHALL render `category` and `subcategory` as editable comboboxes whose suggestions are the distinct, non-empty values already present among the loaded videos, while still allowing the operator to enter a new value.

#### Scenario: Existing values are offered as suggestions

- **WHEN** the operator opens the upload dialog and other videos already use the category `Training`
- **THEN** `Training` appears as a selectable suggestion in the `category` combobox

#### Scenario: A new value can be entered freely

- **WHEN** the operator types a category not used by any existing video
- **THEN** the typed value is accepted and stored without error
