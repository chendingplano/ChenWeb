## ADDED Requirements

### Requirement: Adapters are registered by harness name
The system SHALL hold harness adapters in a registry keyed by harness name. Each adapter SHALL describe its harness name, adapter version, declared capabilities, and maximum run-request size. Registration SHALL fail for an empty or duplicate name, or for an adapter that does not declare `streaming`. Pi SHALL be registered as `pi`.

#### Scenario: Duplicate registration
- **WHEN** two adapters named `pi` are registered
- **THEN** the second registration returns an error and the first stays registered

### Requirement: Service profiles name their harness
Every agent service profile SHALL name its harness. A stored profile version without a harness SHALL be read as `pi`. The harness SHALL be set by the server and SHALL NOT be accepted from the Pi admin candidate.

#### Scenario: Legacy stored version
- **WHEN** a conversation is pinned to a stored profile version whose JSON has no `harness`
- **THEN** the run uses the `pi` adapter

### Requirement: Required capabilities are checked before use
A profile SHALL require `streaming` and `cancellation`. It SHALL also require `interactive_tool_approval` when its default permission is `ask`, and `history_replay` when it saves and resumes conversations. The system SHALL check a profile against the registry at startup (logging each incompatible service), when a profile version is activated (rejecting it), when a conversation is created, and when a run starts. An unregistered harness or a missing required capability SHALL make the service unavailable with a reason naming the harness and the missing capabilities.

#### Scenario: Harness not registered
- **WHEN** the active profile names harness `codex` and no `codex` adapter is registered
- **THEN** creating a conversation returns 503 and no conversation is created

#### Scenario: Activation rejected
- **WHEN** an administrator activates a profile with default permission `ask` and its harness lacks `interactive_tool_approval`
- **THEN** activation returns 400 and the active version is unchanged

### Requirement: Optional operations run only when declared
The system SHALL query context capacity only from adapters that declare `context_capacity_reporting`; otherwise it SHALL use the fallback history budget. It SHALL fold history only with adapters that declare `history_summary`; otherwise older turns SHALL be trimmed. It SHALL refuse a run requested in `ask` mode, and any permission decision, when the run's adapter does not declare `interactive_tool_approval`.

#### Scenario: No context capacity
- **WHEN** a run starts on an adapter without `context_capacity_reporting`
- **THEN** the adapter is not asked for a context window and the fallback budget applies

#### Scenario: Ask mode without approval support
- **WHEN** a run is requested with `permission_mode` `ask` on an adapter without `interactive_tool_approval`
- **THEN** the request is rejected with 409 before any run attempt is created

### Requirement: Effective harness is recorded per run
Each run attempt SHALL record the harness name, adapter version and model name that executed it. Existing attempts SHALL read as harness `pi`. Cancel and permission decisions SHALL go to the adapter recorded on the attempt.

#### Scenario: Run record
- **WHEN** a run starts on the Pi adapter
- **THEN** its attempt row has `harness = 'pi'`, the Pi adapter version, and the profile's model
