// REST client for the Agent Platform API (/api/v1/ap/*).
// Server handlers: ChenWeb/server/api/agentplatformhandler/*.go
// All requests use same-origin cookies for Kratos session auth.

export type Workspace = {
	id: string;
	slug: string;
	name: string;
	owner_user_id: string;
	my_role: 'owner' | 'member' | 'viewer';
	created_at: string;
	updated_at: string;
};

export type Agent = {
	id: string;
	workspace_id: string;
	name: string;
	avatar_emoji: string;
	runtime_kind: 'claude_code' | 'codex' | 'openclaw' | 'opencode';
	model: string;
	instructions: string;
	enabled: boolean;
	archived_at?: string | null;
	created_at: string;
	updated_at: string;
};

export type Project = {
	id: string;
	workspace_id: string;
	name: string;
	description: string;
	created_at: string;
	updated_at: string;
};

export type IssueStatus = 'backlog' | 'todo' | 'in_progress' | 'in_review' | 'done' | 'canceled';

export type Issue = {
	id: string;
	workspace_id: string;
	project_id?: string | null;
	issue_number: number;
	title: string;
	description: string;
	status: IssueStatus;
	priority: number; // 0-4
	board_order: number;
	assignee_user_id?: string | null;
	assignee_agent_id?: string | null;
	created_by: string;
	created_at: string;
	updated_at: string;
};

export type Comment = {
	id: string;
	issue_id: string;
	author_user_id?: string | null;
	author_agent_id?: string | null;
	body: string;
	created_at: string;
};

export type BulkMoveItem = {
	issue_number: number;
	status: IssueStatus;
	board_order: number;
};

export type CreateAgentBody = {
	name: string;
	avatar_emoji?: string;
	runtime_kind: Agent['runtime_kind'];
	model?: string;
	instructions?: string;
	enabled?: boolean;
};

export type UpdateAgentBody = Partial<{
	name: string;
	avatar_emoji: string;
	model: string;
	instructions: string;
	enabled: boolean;
}>;

export type CreateIssueBody = {
	title: string;
	description?: string;
	project_id?: string;
	status?: IssueStatus;
	priority?: number;
	assignee_user_id?: string;
	assignee_agent_id?: string;
};

export type UpdateIssueBody = Partial<{
	title: string;
	description: string;
	project_id: string;
	status: IssueStatus;
	priority: number;
	board_order: number;
	assignee_user_id: string;
	assignee_agent_id: string;
}>;

// -----------------------------------------------------------------------------
// Transport
// -----------------------------------------------------------------------------

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
	const res = await fetch(path, {
		method,
		credentials: 'same-origin',
		headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
		body: body !== undefined ? JSON.stringify(body) : undefined
	});
	if (res.status === 204) return undefined as T;
	const text = await res.text();
	let parsed: unknown = null;
	if (text) {
		try {
			parsed = JSON.parse(text);
		} catch {
			/* leave parsed null */
		}
	}
	if (!res.ok) {
		const msg =
			(parsed && typeof parsed === 'object' && 'error_msg' in parsed
				? String((parsed as { error_msg: unknown }).error_msg)
				: '') || `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return parsed as T;
}

// -----------------------------------------------------------------------------
// API surface
// -----------------------------------------------------------------------------

export const apClient = {
	// Workspaces
	listWorkspaces: () => req<{ workspaces: Workspace[] }>('GET', '/api/v1/ap/workspaces'),
	createWorkspace: (body: { slug: string; name: string }) =>
		req<Workspace>('POST', '/api/v1/ap/workspaces', body),

	// Agents
	listAgents: (slug: string) =>
		req<{ agents: Agent[] }>('GET', `/api/v1/ap/w/${encodeURIComponent(slug)}/agents`),
	createAgent: (slug: string, body: CreateAgentBody) =>
		req<Agent>('POST', `/api/v1/ap/w/${encodeURIComponent(slug)}/agents`, body),
	updateAgent: (slug: string, id: string, body: UpdateAgentBody) =>
		req<Agent>('PATCH', `/api/v1/ap/w/${encodeURIComponent(slug)}/agents/${id}`, body),
	deleteAgent: (slug: string, id: string) =>
		req<void>('DELETE', `/api/v1/ap/w/${encodeURIComponent(slug)}/agents/${id}`),

	// Projects
	listProjects: (slug: string) =>
		req<{ projects: Project[] }>('GET', `/api/v1/ap/w/${encodeURIComponent(slug)}/projects`),
	createProject: (slug: string, body: { name: string; description?: string }) =>
		req<Project>('POST', `/api/v1/ap/w/${encodeURIComponent(slug)}/projects`, body),

	// Issues
	listIssues: (slug: string) =>
		req<{ issues: Issue[] }>('GET', `/api/v1/ap/w/${encodeURIComponent(slug)}/issues`),
	getIssue: (slug: string, num: number) =>
		req<Issue>('GET', `/api/v1/ap/w/${encodeURIComponent(slug)}/issues/${num}`),
	createIssue: (slug: string, body: CreateIssueBody) =>
		req<Issue>('POST', `/api/v1/ap/w/${encodeURIComponent(slug)}/issues`, body),
	updateIssue: (slug: string, num: number, body: UpdateIssueBody) =>
		req<Issue>('PATCH', `/api/v1/ap/w/${encodeURIComponent(slug)}/issues/${num}`, body),
	bulkMoveIssues: (slug: string, moves: BulkMoveItem[]) =>
		req<{ updated: number }>('POST', `/api/v1/ap/w/${encodeURIComponent(slug)}/issues/bulk-move`, {
			moves
		}),

	// Comments
	listComments: (slug: string, num: number) =>
		req<{ comments: Comment[] }>(
			'GET',
			`/api/v1/ap/w/${encodeURIComponent(slug)}/issues/${num}/comments`
		),
	createComment: (slug: string, num: number, body: { body: string }) =>
		req<Comment>('POST', `/api/v1/ap/w/${encodeURIComponent(slug)}/issues/${num}/comments`, body)
};
