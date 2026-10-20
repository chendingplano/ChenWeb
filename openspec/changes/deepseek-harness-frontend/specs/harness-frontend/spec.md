## ADDED Requirements

### Requirement: DeepSeek Harness is a harness front end
ChenWeb SHALL serve DeepSeek Harness as harness `deepseek-harness` through a gateway that speaks harness front-end protocol v1 and drives dsh through `dsh --profile acp`. dsh SHALL keep its own instructions, tools, sandbox and approval rules, skills and session history; the gateway SHALL NOT add instructions, MCP servers or a permission policy.

#### Scenario: DeepSeek Harness selected in the chat page
- **WHEN** an admin selects DeepSeek Harness in the chat page's agent menu while its gateway is configured
- **THEN** the model menu lists the models dsh offers, and messages run as dsh turns in the workspace folder

#### Scenario: Continuing a dsh session
- **WHEN** a turn names an existing dsh session
- **THEN** the gateway sends only the new message, and dsh supplies the earlier context

#### Scenario: Saved agent list without DeepSeek Harness
- **WHEN** a user's saved chat settings list agents that do not include DeepSeek Harness
- **THEN** the agent menu still offers DeepSeek Harness

### Requirement: DeepSeek Harness uses its own login
The DeepSeek Harness gateway SHALL start dsh without the gateway's own variables and without variables whose names contain `SECRET`, `PASSWORD`, `API_KEY`, `TOKEN`, `ENCRYPTION`, `CREDENTIAL` or `DATABASE_URL` or end in `_KEY` or `_DSN`.

#### Scenario: DeepSeek key in the gateway's environment
- **WHEN** the gateway runs with `DEEPSEEK_API_KEY` set
- **THEN** dsh does not receive it and uses the login stored under its own home folder

### Requirement: DeepSeek Harness sessions are confined to the workspace folder
The DeepSeek Harness gateway SHALL create sessions in `DEEPSEEK_HARNESS_WORKSPACE`, list only root sessions whose recorded folder is the workspace, and SHALL refuse to read or resume any other session with 404.

#### Scenario: Session from another folder
- **WHEN** a turn names a dsh session whose log records another folder
- **THEN** the gateway returns 404

### Requirement: DeepSeek Harness history comes from dsh's session log
The gateway SHALL read a session's history from dsh's own session log, showing the user's messages, the assistant's text and tool calls, and tool results, and never reasoning or dsh's injected context. A log in a format version the gateway does not read SHALL be reported as an error naming the version.

#### Scenario: Reopening a session started in dsh's Web UI
- **WHEN** an admin opens, from the Sessions menu, a session started in dsh's own Web UI in the workspace folder
- **THEN** the chat page shows its messages and tool calls

### Requirement: DeepSeek Harness approvals reach the user
dsh's `session/request_permission` requests SHALL be emitted as `permission_request` events showing the tool and input of the matching tool call. Allow SHALL select dsh's `allow_once` option and Deny its `reject_once` option; the gateway SHALL NOT select an "always" option.

#### Scenario: Approving an escalated write
- **WHEN** dsh asks to escalate a write outside the workspace
- **THEN** the page shows an approval card for the `write` tool, and Allow selects `allow_once`

### Requirement: Unsupported permission modes are refused
Because dsh's ACP server has no permission modes, a DeepSeek Harness turn with permission mode `acceptEdits` or `plan` SHALL end with an error and SHALL NOT run.

#### Scenario: Plan mode
- **WHEN** a turn asks for permission mode `plan`
- **THEN** the gateway emits an error and sends nothing to dsh
