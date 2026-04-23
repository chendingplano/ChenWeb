<script lang="ts">
	import { onMount } from 'svelte';
	import KanbanColumn from './kanban-column.svelte';
	import IssueDetailPanel from './issue-detail-panel.svelte';
	import type { Issue, IssueStatus } from './agentplatform-client';
	import {
		apStore,
		STATUS_ORDER,
		STATUS_LABELS,
		spaceBetween
	} from './agentplatform-store.svelte';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let draggingNum = $state<number | null>(null);
	let showCreate = $state(false);
	let newTitle = $state('');
	let newPriority = $state(0);
	let submitting = $state(false);

	onMount(() => {
		if (apStore.workspaces.length === 0) {
			apStore.loadWorkspaces();
		}
	});

	// Issues grouped by status, sorted by board_order ASC then issue_number ASC.
	// STATUS_ORDER provides the five visible columns in left-to-right order.
	const grouped = $derived(() => {
		const out: Record<IssueStatus, Issue[]> = {
			backlog: [],
			todo: [],
			in_progress: [],
			in_review: [],
			done: [],
			canceled: []
		};
		for (const i of apStore.issues) {
			if (out[i.status]) out[i.status].push(i);
		}
		for (const s of STATUS_ORDER) {
			out[s].sort((a, b) =>
				a.board_order !== b.board_order
					? a.board_order - b.board_order
					: a.issue_number - b.issue_number
			);
		}
		return out;
	});

	async function handleDrop(issueNumber: number, newStatus: IssueStatus) {
		draggingNum = null;
		const existing = grouped()[newStatus];
		const last = existing.filter((i) => i.issue_number !== issueNumber).at(-1);
		const prev = last ? last.board_order : 0;
		const nextOrder = spaceBetween(prev, Number.POSITIVE_INFINITY);
		try {
			await apStore.moveIssue(issueNumber, newStatus, nextOrder);
		} catch {
			/* error surfaced via apStore.error */
		}
	}

	async function submitNew() {
		const title = newTitle.trim();
		if (!title) return;
		submitting = true;
		try {
			await apStore.createIssue({ title, priority: newPriority });
			newTitle = '';
			newPriority = 0;
			showCreate = false;
		} catch {
			/* shown via store.error */
		} finally {
			submitting = false;
		}
	}

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const headingColor = $derived(darkMode ? '#E2E8F0' : '#111827');
	const subtle = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btnBg = $derived(darkMode ? '#818CF8' : '#6366F1');
	const inputBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const inputBorder = $derived(darkMode ? '#2D3348' : '#E4E6EB');
</script>

<div
	class="board"
	style:--page={pageBg}
	style:--heading={headingColor}
	style:--subtle={subtle}
	style:--btn={btnBg}
	style:--input-bg={inputBg}
	style:--input-border={inputBorder}
>
	<header class="toolbar">
		<div>
			<h2>{apStore.active?.name ?? 'Agent Platform'}</h2>
			{#if apStore.active}
				<p class="sub">{apStore.issues.length} issue{apStore.issues.length === 1 ? '' : 's'}</p>
			{/if}
		</div>
		<div class="toolbar-right">
			{#if apStore.workspaces.length > 1}
				<select
					class="ws-select"
					value={apStore.activeSlug ?? ''}
					onchange={(e) =>
						apStore.switchWorkspace((e.currentTarget as HTMLSelectElement).value)}
				>
					{#each apStore.workspaces as w (w.id)}
						<option value={w.slug}>{w.name}</option>
					{/each}
				</select>
			{/if}
			<button
				class="primary"
				onclick={() => {
					showCreate = !showCreate;
					if (showCreate) {
						setTimeout(() => document.getElementById('ap-new-title')?.focus(), 30);
					}
				}}
				disabled={!apStore.active}
			>
				{showCreate ? 'Cancel' : '+ New Issue'}
			</button>
		</div>
	</header>

	{#if showCreate}
		<form
			class="create-form"
			onsubmit={(e) => {
				e.preventDefault();
				submitNew();
			}}
		>
			<input
				id="ap-new-title"
				bind:value={newTitle}
				placeholder="What needs to be done?"
				required
			/>
			<select bind:value={newPriority}>
				<option value={0}>No priority</option>
				<option value={1}>Low</option>
				<option value={2}>Medium</option>
				<option value={3}>High</option>
				<option value={4}>Urgent</option>
			</select>
			<button class="primary" type="submit" disabled={submitting || !newTitle.trim()}>
				{submitting ? 'Creating…' : 'Create'}
			</button>
		</form>
	{/if}

	{#if apStore.error}
		<div class="error" role="alert">{apStore.error}</div>
	{/if}

	<div class="main">
		<div class="columns">
			{#each STATUS_ORDER as s (s)}
				<KanbanColumn
					status={s}
					label={STATUS_LABELS[s]}
					issues={grouped()[s]}
					agents={apStore.agents}
					{darkMode}
					selectedIssueNum={apStore.selectedIssueNum}
					dragInFlight={draggingNum !== null}
					onSelect={(n) => apStore.selectIssue(n)}
					onDropCard={handleDrop}
					onDragStart={(n) => (draggingNum = n)}
					onDragEnd={() => (draggingNum = null)}
				/>
			{/each}
		</div>

		{#if apStore.selectedIssue}
			<aside class="detail-drawer">
				<button
					class="close"
					aria-label="Close details"
					onclick={() => apStore.selectIssue(null)}
				>✕</button>
				<IssueDetailPanel {darkMode} />
			</aside>
		{/if}
	</div>
</div>

<style>
	.board {
		display: flex;
		flex-direction: column;
		gap: 16px;
		background: var(--page);
		min-height: 100%;
		padding: 16px 20px 32px;
	}
	.toolbar {
		display: flex;
		justify-content: space-between;
		align-items: flex-end;
		gap: 16px;
	}
	.toolbar h2 {
		margin: 0;
		font-size: 20px;
		font-weight: 600;
		color: var(--heading);
	}
	.sub {
		margin: 4px 0 0;
		font-size: 12px;
		color: var(--subtle);
	}
	.toolbar-right {
		display: flex;
		gap: 8px;
		align-items: center;
	}
	.primary {
		background: var(--btn);
		color: white;
		border: none;
		padding: 8px 14px;
		border-radius: 8px;
		font-weight: 500;
		font-size: 13px;
		cursor: pointer;
		transition: filter 150ms;
	}
	.primary:hover:not(:disabled) {
		filter: brightness(1.1);
	}
	.primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.ws-select,
	.create-form input,
	.create-form select {
		background: var(--input-bg);
		color: var(--heading);
		border: 1px solid var(--input-border);
		border-radius: 8px;
		padding: 8px 10px;
		font-size: 13px;
	}
	.create-form {
		display: flex;
		gap: 8px;
		align-items: center;
		background: var(--input-bg);
		border: 1px solid var(--input-border);
		padding: 12px;
		border-radius: 10px;
	}
	.create-form input {
		flex: 1;
	}
	.main {
		display: flex;
		gap: 12px;
		align-items: flex-start;
		min-height: 0;
		flex: 1;
	}
	.columns {
		display: flex;
		gap: 12px;
		align-items: flex-start;
		overflow-x: auto;
		padding-bottom: 8px;
		flex: 1 1 0;
	}
	.detail-drawer {
		flex: 0 0 340px;
		background: var(--input-bg);
		border: 1px solid var(--input-border);
		border-radius: 10px;
		position: relative;
		max-height: 80vh;
		overflow-y: auto;
	}
	.close {
		position: absolute;
		top: 6px;
		right: 8px;
		background: none;
		border: none;
		color: var(--subtle);
		font-size: 14px;
		cursor: pointer;
		line-height: 1;
		padding: 4px 6px;
		border-radius: 6px;
	}
	.close:hover {
		background: rgba(0, 0, 0, 0.1);
	}
	.error {
		background: rgba(248, 113, 113, 0.12);
		color: #f87171;
		padding: 10px 12px;
		border-radius: 8px;
		font-size: 13px;
	}
</style>
