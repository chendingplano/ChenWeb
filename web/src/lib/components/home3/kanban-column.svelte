<script lang="ts">
	import KanbanCard from './kanban-card.svelte';
	import type { Agent, Issue, IssueStatus } from './agentplatform-client';

	let {
		status,
		label,
		issues,
		agents,
		darkMode = true,
		selectedIssueNum,
		dragInFlight = false,
		onSelect,
		onDropCard,
		onDragStart,
		onDragEnd
	}: {
		status: IssueStatus;
		label: string;
		issues: Issue[]; // already filtered to this column
		agents: Agent[];
		darkMode?: boolean;
		selectedIssueNum: number | null;
		dragInFlight?: boolean;
		onSelect: (num: number) => void;
		onDropCard: (issueNumber: number, status: IssueStatus) => void;
		onDragStart: (num: number) => void;
		onDragEnd: () => void;
	} = $props();

	let hovering = $state(false);

	const surface = $derived(darkMode ? '#171B26' : '#F2F4F7');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const label1 = $derived(darkMode ? '#E2E8F0' : '#111827');
	const label2 = $derived(darkMode ? '#64748B' : '#9CA3AF');
	const dropTint = $derived(darkMode ? 'rgba(129,140,248,0.12)' : 'rgba(99,102,241,0.08)');

	function onDragOver(e: DragEvent) {
		if (!dragInFlight) return;
		e.preventDefault();
		if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
		hovering = true;
	}
	function onDragLeave() {
		hovering = false;
	}
	function onDrop(e: DragEvent) {
		e.preventDefault();
		hovering = false;
		const raw = e.dataTransfer?.getData('text/plain');
		if (!raw) return;
		const num = Number(raw);
		if (!Number.isFinite(num)) return;
		onDropCard(num, status);
	}
</script>

<section
	class="column"
	class:hovering
	style:--surface={surface}
	style:--border={border}
	style:--label1={label1}
	style:--label2={label2}
	style:--tint={dropTint}
	ondragover={onDragOver}
	ondragleave={onDragLeave}
	ondrop={onDrop}
	role="list"
	aria-label={label}
>
	<header class="head">
		<span class="title">{label}</span>
		<span class="count">{issues.length}</span>
	</header>
	<div class="cards">
		{#each issues as issue (issue.id)}
			<KanbanCard
				{issue}
				{agents}
				{darkMode}
				selected={selectedIssueNum === issue.issue_number}
				{onSelect}
				{onDragStart}
				{onDragEnd}
			/>
		{/each}
		{#if issues.length === 0}
			<div class="empty">Drop here</div>
		{/if}
	</div>
</section>

<style>
	.column {
		display: flex;
		flex-direction: column;
		gap: 10px;
		min-width: 260px;
		max-width: 320px;
		flex: 1 1 0;
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 12px;
		transition:
			background 200ms,
			border-color 200ms;
	}
	.column.hovering {
		background: var(--tint);
		border-color: #818cf8;
	}
	.head {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}
	.title {
		color: var(--label1);
		font-weight: 600;
		font-size: 13px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.count {
		color: var(--label2);
		font-size: 12px;
		font-variant-numeric: tabular-nums;
	}
	.cards {
		display: flex;
		flex-direction: column;
		gap: 8px;
		min-height: 40px;
	}
	.empty {
		color: var(--label2);
		font-size: 12px;
		text-align: center;
		padding: 12px 0;
		font-style: italic;
	}
</style>
