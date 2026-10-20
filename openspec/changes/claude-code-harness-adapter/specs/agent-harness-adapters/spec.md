## ADDED Requirements

### Requirement: Claude Code adapter confines runs to SemOS tools
The `claude-code` harness SHALL run each turn with no Claude Code built-in tools, no filesystem settings, skills, plugins, subagents or other MCP servers, and no persisted session. The only tools SHALL be the run's allowed SemOS tools, served in-process. Every tool call SHALL pass the gateway's permission callback, which SHALL deny any tool that is not one of the run's tools on that server. A tool call that did not pass the callback SHALL NOT reach ChenWeb.

#### Scenario: Built-in tool requested
- **WHEN** the model asks to use `Bash` or `Read`
- **THEN** the tool is not available to the session, and the permission callback denies it if asked

#### Scenario: Tool call identity
- **WHEN** an allowed knowledge tool call runs
- **THEN** ChenWeb receives it with `x-chenweb-tool-call-id` equal to the SDK tool-use ID, and the gateway's activity and sources events carry the same ID

### Requirement: Claude Code adapter runs only Anthropic profiles
A profile whose harness is `claude-code` SHALL have provider `anthropic`; otherwise the service SHALL be unavailable and its activation rejected.

#### Scenario: Wrong provider
- **WHEN** an administrator activates a `claude-code` profile with provider `openai`
- **THEN** activation returns 400

### Requirement: Administrators choose a service's harness
The profile admin API SHALL accept an optional `harness` (lowercase letters, digits and hyphens, at most 32 characters; empty means `pi`). It SHALL list the registered harnesses, and activation SHALL require the chosen harness to be registered and compatible.

#### Scenario: Gateway not configured
- **WHEN** `claude-code` is chosen but `CLAUDE_CODE_GATEWAY_URL` is unset
- **THEN** activation returns 400 and the active version is unchanged
