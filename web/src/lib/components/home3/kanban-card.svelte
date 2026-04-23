<script lang="ts">
	import type { Agent, Issue } from './agentplatform-client';

	let {
		issue,
		agents,
		darkMode = true,
		selected = false,
		onSelect,
		onDragStart,
		onDragEnd
	}: {
		issue: Issue;
		agents: Agent[];
		darkMode?: boolean;
		selected?: boolean;
		onSelect: (num: number) => void;
		onDragStart: (num: number) => void;
		onDragEnd: () => void;
	} = $props();

	const priorityLabels = ['—', 'Low', 'Medium', 'High', 'Urgent'];
	const priorityColor = (p: number) =>
		p >= 4 ? '#F87171' : p === 3 ? '#FBBF24' : p === 2 ? '#60A5FA' : '#94A3B8';

	const agent = $derived(
		issue.assignee_agent_id
			? agents.find((a) => a.id === issue.assignee_agent_id) ?? null
			: null
	);

	// Design tokens
	const cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const borderCol = $derived(selected ? '#818CF8' : darkMode ? '#2D3348' : '#E4E6EB');
	const title = $derived(darkMode ? '#E2E8F0' : '#111827');
	const muted = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const numberTint = $derived(darkMode ? '#64748B' : '#9CA3AF');
</script>

<div
	class="kanban-card"
	style:--bg={cardBg}
	style:--border={borderCol}
	style:--title={title}
	style:--muted={muted}
	style:--number={numberTint}
	draggable="true"
	role="button"
	tabindex="0"
	aria-pressed={selected}
	ondragstart={(e) => {
		e.dataTransfer?.setData('text/plain', String(issue.issue_number));
		if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
		onDragStart(issue.issue_number);
	}}
	ondragend={onDragEnd}
	onclick={() => onSelect(issue.issue_number)}
	onkeydown={(e) => {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			onSelect(issue.issue_number);
		}
	}}
>
	<div class="head">
		<span class="number">#{issue.issue_number}</span>
		{#if issue.priority > 0}
			<span class="priority" style:color={priorityColor(issue.priority)}>
				● {priorityLabels[issue.priority]}
			</span>
		{/if}
	</div>
	<div class="title">{issue.title}</div>
	{#if agent}
		<div class="foot">
			<span class="avatar">{agent.avatar_emoji}</span>
			<span class="assignee">{agent.name}</span>
		</div>
	{:else if issue.assignee_user_id}
		<div class="foot">
			<span class="assignee">👤 Assigned</span>
		</div>
	{/if}
</div>

<style>
	.kanban-card {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 10px 12px;
		cursor: grab;
		transition:
			border-color 150ms,
			transform 150ms,
			box-shadow 150ms;
		user-select: none;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.kanban-card:active {
		cursor: grabbing;
	}
	.kanban-card:hover {
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
	}
	.head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		font-size: 11px;
	}
	.number {
		color: var(--number);
		font-variant-numeric: tabular-nums;
	}
	.priority {
		font-weight: 500;
		font-size: 10px;
	}
	.title {
		color: var(--title);
		font-size: 13px;
		line-height: 1.35;
		word-break: break-word;
	}
	.foot {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 11px;
		color: var(--muted);
	}
	.avatar {
		font-size: 14px;
	}
</style>
