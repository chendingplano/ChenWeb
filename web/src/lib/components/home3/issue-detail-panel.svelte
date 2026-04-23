<script lang="ts">
	import { onMount } from 'svelte';
	import {
		apClient,
		isTerminalRun,
		type Comment,
		type IssueStatus,
		type TaskEvent,
		type TaskRun
	} from './agentplatform-client';
	import { apStore, STATUS_LABELS } from './agentplatform-store.svelte';
	import type { WSFrame } from './ws-client';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let comments = $state<Comment[]>([]);
	let loadingComments = $state(false);
	let commentsError = $state<string | null>(null);
	let newBody = $state('');
	let posting = $state(false);
	let lastLoadedNum = $state<number | null>(null);

	// --- Task runs state ---
	let runs = $state<TaskRun[]>([]);
	let runsError = $state<string | null>(null);
	let launchingRun = $state(false);
	let expandedRunId = $state<string | null>(null);
	// Map run id → events observed so far (for the expanded row).
	let runEvents = $state<Record<string, TaskEvent[]>>({});
	// Active polling identifier — used to cancel stale loops on re-entry.
	let pollToken = 0;

	async function loadRuns(slug: string, num: number) {
		try {
			const res = await apClient.listIssueRuns(slug, num);
			runs = res.runs;
			// Auto-expand the latest run if it's still live and nothing else is open.
			if (!expandedRunId && runs[0] && !isTerminalRun(runs[0].status)) {
				setExpandedRun(runs[0].id);
			}
		} catch (e) {
			runsError = String((e as Error).message ?? e);
		}
	}

	async function launchRun() {
		const slug = apStore.activeSlug;
		const issue = apStore.selectedIssue;
		if (!slug || !issue) return;
		if (!issue.assignee_agent_id) {
			runsError = 'assign an agent to this issue before running';
			return;
		}
		runsError = null;
		launchingRun = true;
		try {
			const run = await apClient.runIssue(slug, issue.issue_number);
			runs = [run, ...runs];
			setExpandedRun(run.id);
		} catch (e) {
			runsError = String((e as Error).message ?? e);
		} finally {
			launchingRun = false;
		}
	}

	async function cancelRun(runId: string) {
		const slug = apStore.activeSlug;
		if (!slug) return;
		try {
			await apClient.cancelRun(slug, runId);
			// Refresh the run row; polling (if any) will also catch it up.
			const fresh = await apClient.getRun(slug, runId);
			runs = runs.map((r) => (r.id === runId ? fresh : r));
		} catch (e) {
			runsError = String((e as Error).message ?? e);
		}
	}

	function setExpandedRun(runId: string | null) {
		expandedRunId = runId;
		// Invalidate the previous polling loop; a new one starts if needed.
		pollToken++;
		if (!runId) return;
		runEvents[runId] ??= [];
		startPolling(runId, pollToken);
	}

	async function startPolling(runId: string, token: number) {
		const slug = apStore.activeSlug;
		if (!slug) return;
		// Initial fetch, then 2s interval until terminal or cancelled.
		let since = 0;
		const existing = runEvents[runId] ?? [];
		if (existing.length) since = existing[existing.length - 1].id;

		while (pollToken === token && apStore.activeSlug === slug) {
			// When realtime is healthy the WS delivers events/status; skip
			// the REST round-trip to stay quiet. The loop keeps running so
			// we reclaim the responsibility if the WS drops.
			if (apStore.realtimeStatus !== 'open') {
				try {
					const [{ events }, run] = await Promise.all([
						apClient.listRunEvents(slug, runId, since),
						apClient.getRun(slug, runId)
					]);
					if (events.length) {
						runEvents[runId] = [...(runEvents[runId] ?? []), ...events];
						since = events[events.length - 1].id;
					}
					runs = runs.map((r) => (r.id === runId ? run : r));
					if (isTerminalRun(run.status)) return;
				} catch {
					/* swallow transient errors; keep polling */
				}
			}
			await new Promise((r) => setTimeout(r, 2000));
		}
	}

	function runStatusColor(s: TaskRun['status']): string {
		switch (s) {
			case 'succeeded':
				return '#34D399';
			case 'failed':
				return '#F87171';
			case 'canceled':
				return '#94A3B8';
			case 'running':
				return '#818CF8';
			case 'claimed':
				return '#FBBF24';
			default:
				return '#64748B';
		}
	}

	// Subscribe to realtime frames for the currently-selected issue / run.
	// The store's dispatch applies issue.updated to issues[] itself; we
	// consume the comment / run / task event frames here.
	onMount(() => {
		return apStore.subscribeFrames(handleFrame);
	});

	function handleFrame(f: WSFrame) {
		const iss = apStore.selectedIssue;
		switch (f.type) {
			case 'comment.created': {
				const c = f.payload as Comment;
				if (iss && c.issue_id === iss.id) {
					// Append if not already present (REST race).
					if (!comments.some((x) => x.id === c.id)) {
						comments = [...comments, c];
					}
				}
				return;
			}
			case 'run.created': {
				const r = f.payload as TaskRun;
				if (iss && r.issue_id === iss.id) {
					if (!runs.some((x) => x.id === r.id)) {
						runs = [r, ...runs];
					}
					if (!expandedRunId) setExpandedRun(r.id);
				}
				return;
			}
			case 'task.status': {
				const r = f.payload as TaskRun;
				if (runs.some((x) => x.id === r.id)) {
					runs = runs.map((x) => (x.id === r.id ? r : x));
				}
				return;
			}
			case 'task.event': {
				const ev = f.payload as TaskEvent;
				const existing = runEvents[ev.task_run_id] ?? [];
				// Skip dupes if polling and WS briefly overlap.
				if (existing.some((e) => e.id === ev.id)) return;
				runEvents = {
					...runEvents,
					[ev.task_run_id]: [...existing, ev]
				};
				return;
			}
		}
	}

	function eventColor(k: TaskEvent['kind']): string {
		switch (k) {
			case 'stderr':
			case 'error':
				return '#F87171';
			case 'status':
				return '#818CF8';
			case 'artifact':
				return '#34D399';
			default:
				return '';
		}
	}

	// React to selection changes: when a different issue is selected, reload.
	$effect(() => {
		const num = apStore.selectedIssueNum;
		const slug = apStore.activeSlug;
		if (num === null || slug === null) {
			comments = [];
			runs = [];
			runEvents = {};
			expandedRunId = null;
			pollToken++;
			lastLoadedNum = null;
			return;
		}
		if (num === lastLoadedNum) return;
		lastLoadedNum = num;
		// Reset run UI for the newly-selected issue.
		runs = [];
		runEvents = {};
		expandedRunId = null;
		pollToken++;
		runsError = null;
		loadRuns(slug, num);
		loadingComments = true;
		commentsError = null;
		apClient
			.listComments(slug, num)
			.then((r) => {
				comments = r.comments;
			})
			.catch((e) => {
				commentsError = String((e as Error).message ?? e);
			})
			.finally(() => {
				loadingComments = false;
			});
	});

	async function assignAgent(agentId: string) {
		const issue = apStore.selectedIssue;
		if (!issue) return;
		try {
			await apStore.updateIssue(issue.issue_number, {
				assignee_agent_id: agentId,
				assignee_user_id: ''
			});
		} catch {
			/* surfaced via store.error */
		}
	}

	async function changeStatus(status: IssueStatus) {
		const issue = apStore.selectedIssue;
		if (!issue) return;
		try {
			await apStore.updateIssue(issue.issue_number, { status });
		} catch {
			/* store.error */
		}
	}

	async function postComment() {
		const slug = apStore.activeSlug;
		const issue = apStore.selectedIssue;
		if (!slug || !issue) return;
		const body = newBody.trim();
		if (!body) return;
		posting = true;
		try {
			const created = await apClient.createComment(slug, issue.issue_number, { body });
			comments = [...comments, created];
			newBody = '';
		} catch (e) {
			commentsError = String((e as Error).message ?? e);
		} finally {
			posting = false;
		}
	}

	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
</script>

<div
	class="panel"
	style:--card={card}
	style:--border={border}
	style:--heading={heading}
	style:--sub={sub}
>
	{#if !apStore.selectedIssue}
		<p class="muted">Select an issue to see details.</p>
	{:else}
		{@const iss = apStore.selectedIssue}
		<header class="head">
			<span class="number">#{iss.issue_number}</span>
			<h3>{iss.title}</h3>
		</header>

		<section>
			<label>Status</label>
			<select
				value={iss.status}
				onchange={(e) =>
					changeStatus((e.currentTarget as HTMLSelectElement).value as IssueStatus)}
			>
				{#each Object.entries(STATUS_LABELS) as [v, l] (v)}
					<option value={v}>{l}</option>
				{/each}
			</select>
		</section>

		<section>
			<label>Assignee</label>
			<select
				value={iss.assignee_agent_id ?? ''}
				onchange={(e) => assignAgent((e.currentTarget as HTMLSelectElement).value)}
			>
				<option value="">— unassigned —</option>
				{#each apStore.agents as a (a.id)}
					<option value={a.id}>{a.avatar_emoji} {a.name}</option>
				{/each}
			</select>
		</section>

		{#if iss.description}
			<section>
				<label>Description</label>
				<p class="desc">{iss.description}</p>
			</section>
		{/if}

		<section class="runs">
			<div class="runs-head">
				<label>Agent Runs</label>
				<button
					class="run-btn"
					onclick={launchRun}
					disabled={launchingRun || !iss.assignee_agent_id}
					title={iss.assignee_agent_id ? 'Enqueue a new run' : 'Assign an agent first'}
				>
					{launchingRun ? 'Queuing…' : '▶ Run'}
				</button>
			</div>
			{#if runsError}
				<p class="error-text">{runsError}</p>
			{/if}
			{#if runs.length === 0}
				<p class="muted">No runs yet.</p>
			{:else}
				<ul class="runs-list">
					{#each runs as run (run.id)}
						<li class="run-row" class:expanded={expandedRunId === run.id}>
							<button
								class="run-head"
								onclick={() =>
									setExpandedRun(expandedRunId === run.id ? null : run.id)}
							>
								<span class="run-dot" style:background={runStatusColor(run.status)}></span>
								<span class="run-status">{run.status}</span>
								<time>{new Date(run.queued_at).toLocaleTimeString()}</time>
								{#if run.exit_code !== null && run.exit_code !== undefined}
									<span class="muted small">exit {run.exit_code}</span>
								{/if}
							</button>
							{#if expandedRunId === run.id}
								<div class="run-body">
									{#if run.error_message}
										<p class="error-text small">{run.error_message}</p>
									{/if}
									<pre class="log">{#each runEvents[run.id] ?? [] as ev (ev.id)}<span
												style:color={eventColor(ev.kind)}
												>{ev.kind === 'stderr' || ev.kind === 'error'
													? 'err'
													: ev.kind === 'status'
														? 'sta'
														: ev.kind === 'artifact'
															? 'art'
															: 'out'} · {ev.payload}</span
											>{'\n'}{/each}</pre>
									{#if !isTerminalRun(run.status)}
										<button class="cancel-link" onclick={() => cancelRun(run.id)}>
											Cancel run
										</button>
									{/if}
								</div>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<section class="comments">
			<label>Comments</label>
			{#if loadingComments}
				<p class="muted">Loading…</p>
			{:else if commentsError}
				<p class="error-text">{commentsError}</p>
			{:else if comments.length === 0}
				<p class="muted">No comments yet.</p>
			{:else}
				<ul>
					{#each comments as c (c.id)}
						<li>
							<div class="cmt-head">
								<span class="who">
									{c.author_agent_id ? '🤖 Agent' : '👤 User'}
								</span>
								<time>{new Date(c.created_at).toLocaleString()}</time>
							</div>
							<div class="cmt-body">{c.body}</div>
						</li>
					{/each}
				</ul>
			{/if}
			<form
				class="reply"
				onsubmit={(e) => {
					e.preventDefault();
					postComment();
				}}
			>
				<textarea rows="2" bind:value={newBody} placeholder="Write a comment…"></textarea>
				<button type="submit" disabled={posting || !newBody.trim()}>
					{posting ? 'Posting…' : 'Post'}
				</button>
			</form>
		</section>
	{/if}
</div>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 14px;
	}
	.head {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.number {
		color: var(--sub);
		font-size: 11px;
		font-variant-numeric: tabular-nums;
	}
	h3 {
		color: var(--heading);
		font-size: 15px;
		font-weight: 600;
		margin: 0;
		line-height: 1.35;
	}
	section {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	label {
		color: var(--sub);
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	select,
	textarea {
		background: var(--card);
		color: var(--heading);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 6px 8px;
		font-size: 12px;
		font-family: inherit;
	}
	.desc {
		margin: 0;
		color: var(--heading);
		font-size: 12px;
		white-space: pre-wrap;
	}
	.muted {
		color: var(--sub);
		font-size: 12px;
		margin: 0;
	}
	.error-text {
		color: #f87171;
		font-size: 12px;
		margin: 0;
	}
	.comments ul {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.comments li {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
	}
	.cmt-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		font-size: 11px;
		color: var(--sub);
	}
	.cmt-body {
		color: var(--heading);
		font-size: 12px;
		margin-top: 4px;
		white-space: pre-wrap;
	}
	.reply {
		display: flex;
		flex-direction: column;
		gap: 6px;
		margin-top: 8px;
	}
	.reply button {
		align-self: flex-end;
		background: #818cf8;
		color: white;
		border: none;
		padding: 6px 12px;
		border-radius: 8px;
		cursor: pointer;
		font-size: 12px;
	}
	.reply button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.runs {
		gap: 6px;
	}
	.runs-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}
	.run-btn {
		background: #818cf8;
		color: white;
		border: none;
		padding: 4px 10px;
		border-radius: 6px;
		font-size: 12px;
		cursor: pointer;
	}
	.run-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	.runs-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.run-row {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 8px;
		overflow: hidden;
	}
	.run-row.expanded {
		background: rgba(129, 140, 248, 0.06);
	}
	.run-head {
		width: 100%;
		background: none;
		border: none;
		padding: 6px 10px;
		display: flex;
		align-items: center;
		gap: 8px;
		cursor: pointer;
		color: var(--heading);
		font-size: 12px;
		text-align: left;
	}
	.run-head:hover {
		background: rgba(255, 255, 255, 0.02);
	}
	.run-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}
	.run-status {
		flex: 1;
		font-variant-numeric: tabular-nums;
	}
	.run-head time,
	.run-head .small {
		color: var(--sub);
		font-size: 11px;
	}
	.run-body {
		padding: 8px 10px 10px;
		border-top: 1px solid var(--border);
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.log {
		margin: 0;
		font-family: 'Fira Code', 'Cascadia Code', monospace;
		font-size: 11px;
		background: rgba(0, 0, 0, 0.25);
		color: var(--heading);
		padding: 8px;
		border-radius: 6px;
		max-height: 220px;
		overflow-y: auto;
		white-space: pre-wrap;
		word-break: break-all;
	}
	.cancel-link {
		align-self: flex-start;
		background: none;
		border: 1px solid var(--border);
		color: #f87171;
		padding: 3px 8px;
		border-radius: 6px;
		font-size: 11px;
		cursor: pointer;
	}
	.cancel-link:hover {
		background: rgba(248, 113, 113, 0.08);
	}
</style>
