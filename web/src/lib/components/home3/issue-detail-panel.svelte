<script lang="ts">
	import { apClient, type Comment, type IssueStatus } from './agentplatform-client';
	import { apStore, STATUS_LABELS } from './agentplatform-store.svelte';

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

	// React to selection changes: when a different issue is selected, reload.
	$effect(() => {
		const num = apStore.selectedIssueNum;
		const slug = apStore.activeSlug;
		if (num === null || slug === null) {
			comments = [];
			lastLoadedNum = null;
			return;
		}
		if (num === lastLoadedNum) return;
		lastLoadedNum = num;
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
</style>
