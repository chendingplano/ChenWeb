## ADDED Requirements

### Requirement: Codex is a harness front end
ChenWeb SHALL serve Codex as harness `codex` through a gateway that speaks harness front-end protocol v1 and drives Codex through `codex app-server`. Codex SHALL keep its own instructions, tools, sandbox and approval rules, memories and thread history; the gateway SHALL NOT add instructions or override the sandbox or approval policy, except as a permission mode requests.

#### Scenario: Codex selected in the chat page
- **WHEN** an admin selects Codex in the chat page's agent menu while the Codex gateway is configured
- **THEN** the model menu lists the models Codex offers the account, and messages run as Codex turns in the workspace folder

#### Scenario: Continuing a Codex session
- **WHEN** a turn names an existing Codex thread
- **THEN** the gateway sends only the new message, and Codex supplies the earlier context

### Requirement: Codex uses the machine's Codex login
The Codex gateway SHALL start Codex without the gateway secret and without variables whose names contain `SECRET`, `PASSWORD` or `API_KEY`.

#### Scenario: OpenAI key in the gateway's environment
- **WHEN** the gateway runs with `OPENAI_API_KEY` set by the ChenWeb environment
- **THEN** Codex does not receive it and uses its own sign-in

### Requirement: Codex sessions are confined to the workspace folder
The Codex gateway SHALL start threads in `CODEX_WORKSPACE`, list only threads whose folder is the workspace, and SHALL refuse to read or resume any other thread with 404.

#### Scenario: Thread from another folder
- **WHEN** a turn names a Codex thread whose recorded folder is not the workspace
- **THEN** the gateway returns 404

### Requirement: Codex approvals and questions reach the user
Codex's command, file-change and permission approval requests SHALL be emitted as `permission_request` events and answered with the user's allow or deny. Codex's `request_user_input` questions SHALL be emitted as `AskUserQuestion` requests, and an allow decision's answers SHALL be passed to Codex keyed by question ID. Requests the page cannot show (MCP elicitations) SHALL be declined.

#### Scenario: Approving a command
- **WHEN** Codex asks to run a command outside its sandbox
- **THEN** the page shows an approval card, and Allow sends `accept` to Codex

#### Scenario: Answering a Codex question
- **WHEN** Codex asks "Which DB?" with options Postgres and MySQL and the user picks Postgres
- **THEN** Codex receives `{answers: {<question id>: {answers: ["Postgres"]}}}`
