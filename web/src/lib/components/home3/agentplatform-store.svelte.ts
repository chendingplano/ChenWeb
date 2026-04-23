// Svelte 5 runes store for the Agent Platform views. Holds workspace
// state, agents, and issues. Mutators wrap REST calls from apClient and
// apply optimistic updates where safe (kanban moves).

import {
	apClient,
	type Agent,
	type CreateAgentBody,
	type CreateIssueBody,
	type Issue,
	type IssueStatus,
	type UpdateIssueBody,
	type Workspace
} from './agentplatform-client';

// STATUS_ORDER is the left-to-right column order on the kanban board.
// "canceled" is intentionally excluded from the default board; it shows
// up only in filtered views later.
export const STATUS_ORDER: IssueStatus[] = ['backlog', 'todo', 'in_progress', 'in_review', 'done'];

export const STATUS_LABELS: Record<IssueStatus, string> = {
	backlog: 'Backlog',
	todo: 'Todo',
	in_progress: 'In Progress',
	in_review: 'In Review',
	done: 'Done',
	canceled: 'Canceled'
};

// spaceBetween returns a board_order value strictly between prev and next.
// Pass Number.NEGATIVE_INFINITY / POSITIVE_INFINITY for open-ended sides.
export function spaceBetween(prev: number, next: number): number {
	if (!isFinite(prev) && !isFinite(next)) return 1000;
	if (!isFinite(prev)) return next - 1000;
	if (!isFinite(next)) return prev + 1000;
	return (prev + next) / 2;
}

function createStore() {
	let workspaces = $state<Workspace[]>([]);
	let activeSlug = $state<string | null>(null);
	let agents = $state<Agent[]>([]);
	let issues = $state<Issue[]>([]);
	let selectedIssueNum = $state<number | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);

	const active = $derived(workspaces.find((w) => w.slug === activeSlug) ?? null);

	const selectedIssue = $derived(issues.find((i) => i.issue_number === selectedIssueNum) ?? null);

	async function loadWorkspaces() {
		loading = true;
		error = null;
		try {
			const res = await apClient.listWorkspaces();
			workspaces = res.workspaces;
			if (!activeSlug && workspaces.length > 0) {
				activeSlug = workspaces[0].slug;
				await Promise.all([loadAgents(), loadIssues()]);
			}
		} catch (e) {
			error = String((e as Error).message ?? e);
		} finally {
			loading = false;
		}
	}

	async function switchWorkspace(slug: string) {
		if (slug === activeSlug) return;
		activeSlug = slug;
		selectedIssueNum = null;
		await Promise.all([loadAgents(), loadIssues()]);
	}

	async function loadAgents() {
		if (!activeSlug) return;
		try {
			const res = await apClient.listAgents(activeSlug);
			agents = res.agents;
		} catch (e) {
			error = String((e as Error).message ?? e);
		}
	}

	async function createAgent(body: CreateAgentBody) {
		if (!activeSlug) return;
		const created = await apClient.createAgent(activeSlug, body);
		agents = [created, ...agents];
	}

	async function deleteAgent(id: string) {
		if (!activeSlug) return;
		await apClient.deleteAgent(activeSlug, id);
		agents = agents.filter((a) => a.id !== id);
	}

	async function loadIssues() {
		if (!activeSlug) return;
		try {
			const res = await apClient.listIssues(activeSlug);
			issues = res.issues;
		} catch (e) {
			error = String((e as Error).message ?? e);
		}
	}

	async function createIssue(body: CreateIssueBody) {
		if (!activeSlug) return;
		const created = await apClient.createIssue(activeSlug, body);
		issues = [...issues, created];
		selectedIssueNum = created.issue_number;
	}

	async function updateIssue(num: number, patch: UpdateIssueBody) {
		if (!activeSlug) return;
		const updated = await apClient.updateIssue(activeSlug, num, patch);
		issues = issues.map((i) => (i.issue_number === num ? updated : i));
	}

	// moveIssue handles a kanban drag-drop: optimistic local update, then a
	// bulk-move server call. On failure we revert by reloading.
	async function moveIssue(num: number, status: IssueStatus, boardOrder: number) {
		if (!activeSlug) return;
		const snapshot = issues;
		issues = issues.map((i) =>
			i.issue_number === num ? { ...i, status, board_order: boardOrder } : i
		);
		try {
			await apClient.bulkMoveIssues(activeSlug, [
				{ issue_number: num, status, board_order: boardOrder }
			]);
		} catch (e) {
			issues = snapshot;
			error = String((e as Error).message ?? e);
			throw e;
		}
	}

	function selectIssue(num: number | null) {
		selectedIssueNum = num;
	}

	return {
		get workspaces() {
			return workspaces;
		},
		get activeSlug() {
			return activeSlug;
		},
		get active() {
			return active;
		},
		get agents() {
			return agents;
		},
		get issues() {
			return issues;
		},
		get selectedIssueNum() {
			return selectedIssueNum;
		},
		get selectedIssue() {
			return selectedIssue;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		loadWorkspaces,
		switchWorkspace,
		loadAgents,
		createAgent,
		deleteAgent,
		loadIssues,
		createIssue,
		updateIssue,
		moveIssue,
		selectIssue
	};
}

export const apStore = createStore();
