## ADDED Requirements

### Requirement: Training viewer gallery
The Resources page's Videos → Training (培训) entry SHALL render a read-only card gallery of all videos. Each card SHALL display the video's cover image, name, description, size, and upload time. The page SHALL NOT expose upload/edit/delete controls.

#### Scenario: Cards render video metadata
- **WHEN** an authenticated user opens Resources → Videos → Training
- **THEN** each video appears as a card showing its cover image, name, description, human-readable size, and upload time

#### Scenario: Missing cover image
- **WHEN** a video has no associated cover image
- **THEN** the card shows a sensible placeholder rather than a broken image

#### Scenario: Empty state
- **WHEN** there are no videos
- **THEN** the gallery shows an empty state rather than an error

### Requirement: Play on click
Clicking a card SHALL play the associated video.

#### Scenario: Playback
- **WHEN** the user clicks a video card
- **THEN** the video plays (uploaded videos stream from the server with seeking support)
