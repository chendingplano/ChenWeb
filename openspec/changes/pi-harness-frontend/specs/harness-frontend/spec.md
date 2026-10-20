## ADDED Requirements

### Requirement: Pi is a harness front end
ChenWeb SHALL serve Pi as harness `pi` through a gateway that speaks harness front-end protocol v1 and drives Pi through `pi --mode rpc`. Pi SHALL keep its own instructions, tools, extensions, skills, settings and session files; the gateway SHALL NOT add instructions, tools or extensions, and ChenWeb SHALL NOT store a copy of Pi's conversations.

#### Scenario: pi selected in the chat page
- **WHEN** an admin selects `pi` in the chat page's agent menu while its gateway is configured
- **THEN** the model menu lists the models Pi offers, and messages run as Pi turns in the workspace folder

#### Scenario: Continuing a Pi session
- **WHEN** a turn names an existing Pi session whose Pi process was stopped
- **THEN** the gateway starts Pi on that session's file and sends only the new message, and Pi supplies the earlier context

### Requirement: Pi uses its own login
The Pi gateway SHALL start Pi without the gateway's own variables and without variables whose names contain `SECRET`, `PASSWORD`, `API_KEY`, `TOKEN`, `ENCRYPTION`, `CREDENTIAL` or `DATABASE_URL` or end in `_KEY` or `_DSN`, except variables named in `PI_HARNESS_PASS_ENV`.

#### Scenario: Provider key in the gateway's environment
- **WHEN** the gateway runs with `ANTHROPIC_API_KEY` set and not named in `PI_HARNESS_PASS_ENV`
- **THEN** Pi does not receive it and uses the login stored in its own configuration folder

### Requirement: Pi sessions are confined to the workspace folder
The Pi gateway SHALL create sessions in `PI_HARNESS_WORKSPACE`, list only sessions in Pi's session folder for the workspace whose file records the workspace, and SHALL refuse to read or resume any other session with 404.

#### Scenario: Session from another folder
- **WHEN** a turn names a Pi session whose file records another folder
- **THEN** the gateway returns 404

### Requirement: Pi history comes from Pi's session file
The gateway SHALL read a session's history from Pi's own session file, showing the active branch: the user's messages, the assistant's text and tool calls, tool results and the user's `!` commands, and never thinking or extension-injected messages. A file in a format version the gateway does not read SHALL be reported as an error naming the version.

#### Scenario: Reopening a session started in a terminal
- **WHEN** an admin opens, from the Sessions menu, a session started in a terminal Pi in the workspace folder
- **THEN** the chat page shows its messages and tool calls

### Requirement: Pi extension dialogs reach the user
An extension's `confirm` dialog SHALL be emitted as a `permission_request` with the dialog's title and message, and answered `confirmed: true` on Allow and `confirmed: false` on Deny. A `select` dialog SHALL be emitted as an `AskUserQuestion` request and answered with the chosen option. Dialogs that need free text SHALL be cancelled.

#### Scenario: Approving a gated write
- **WHEN** a Pi extension asks `confirm` before a write
- **THEN** the page shows an approval card with the dialog's title, and Allow lets the write run

### Requirement: Unsupported permission modes are refused
Because Pi has no permission modes, a Pi turn with permission mode `acceptEdits` or `plan` SHALL end with an error and SHALL NOT start Pi.

#### Scenario: Plan mode
- **WHEN** a turn asks for permission mode `plan`
- **THEN** the gateway emits an error and starts no Pi process
