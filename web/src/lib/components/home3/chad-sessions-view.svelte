<script lang="ts">
	import { onMount } from 'svelte';
	import Clock3Icon from '@lucide/svelte/icons/clock-3';
	import FolderOpenIcon from '@lucide/svelte/icons/folder-open';
	import MessageSquareIcon from '@lucide/svelte/icons/message-square';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import {
		getHarnessSession,
		listHarnessSessions,
		type ChadSessionDetail,
		type ChadSessionSummary
	} from './chad-sessions-client';

	let { darkMode = true, harness = 'chad' }: { darkMode?: boolean; harness?: 'chad' | 'pi' } = $props();
	let harnessLabel = $derived(harness === 'pi' ? 'Pi' : 'Chad');
	let sessions = $state<ChadSessionSummary[]>([]);
	let selectedID = $state<string | null>(null);
	let detail = $state<ChadSessionDetail | null>(null);
	let loading = $state(true);
	let detailLoading = $state(false);
	let error = $state<string | null>(null);
	let detailError = $state<string | null>(null);
	const pageSize = 20;
	let page = $state(0);
	let pageCount = $derived(Math.max(1, Math.ceil(sessions.length / pageSize)));
	let pageSessions = $derived(sessions.slice(page * pageSize, (page + 1) * pageSize));

	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let text = $derived(darkMode ? '#E2E8F0' : '#111827');
	let secondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let muted = $derived(darkMode ? '#64748B' : '#9CA3AF');

	onMount(() => {
		loadSessions();
	});

	async function loadSessions() {
		loading = true;
		error = null;
		try {
			const response = await listHarnessSessions(harness);
			sessions = response.sessions ?? [];
			const selectedIndex = selectedID
				? sessions.findIndex((session) => session.id === selectedID)
				: -1;
			page = selectedIndex >= 0 ? Math.floor(selectedIndex / pageSize) : 0;
			const nextID =
				selectedID && sessions.some((session) => session.id === selectedID)
					? selectedID
					: (sessions[0]?.id ?? null);
			selectedID = nextID;
			if (nextID) await loadDetail(nextID);
			else detail = null;
		} catch (err) {
			error = errorMessage(err);
		} finally {
			loading = false;
		}
	}

	async function loadDetail(id: string) {
		selectedID = id;
		detailLoading = true;
		detailError = null;
		try {
			detail = await getHarnessSession(harness, id);
		} catch (err) {
			detail = null;
			detailError = errorMessage(err);
		} finally {
			detailLoading = false;
		}
	}

	function errorMessage(err: unknown): string {
		return err instanceof Error ? err.message : String(err);
	}

	function formatDate(seconds: number | undefined): string {
		if (!seconds) return 'Unknown time';
		return new Intl.DateTimeFormat(undefined, {
			dateStyle: 'medium',
			timeStyle: 'short'
		}).format(new Date(seconds * 1000));
	}

	function formatContent(content: unknown): string {
		if (content === undefined || content === null) return '—';
		if (typeof content === 'string') return content;
		try {
			return JSON.stringify(content, null, 2);
		} catch {
			return String(content);
		}
	}

	function tokenCount(content: unknown): number {
		const bytes = new TextEncoder().encode(formatContent(content)).length;
		return Math.max(1, Math.ceil(bytes / 4));
	}

	function roleLabel(role: string | undefined): string {
		return role ? role.toUpperCase() : 'MESSAGE';
	}

	function setPage(nextPage: number): void {
		page = Math.max(0, Math.min(nextPage, pageCount - 1));
	}
</script>

<div
	class="session-page"
	style="--page-bg:{pageBg}; --card-bg:{cardBg}; --surface:{surface}; --border:{border}; --accent:{accent}; --accent-tint:{accentTint}; --text:{text}; --secondary:{secondary}; --muted:{muted};"
>
	<header class="page-header">
		<div>
			<p class="eyebrow">SYSTEM ADMIN / LLM / {harnessLabel.toUpperCase()} SESSIONS</p>
			<h1>{harnessLabel} Sessions</h1>
			<p class="intro">Read-only request and response logs from the local {harnessLabel} session store.</p>
		</div>
		<div class="header-actions">
			<span class="count">{sessions.length} {sessions.length === 1 ? 'session' : 'sessions'}</span>
			<button
				class="refresh"
				onclick={loadSessions}
				disabled={loading}
				aria-label="Refresh sessions"
			>
				<RefreshCwIcon size={15} class={loading ? 'spin' : ''} />
				Refresh
			</button>
		</div>
	</header>

	{#if error}
		<div class="notice error">Could not load sessions: {error}</div>
	{/if}

	<div class="workspace">
		<aside class="session-list" aria-label="{harnessLabel} sessions">
			<div class="list-heading"><span>SESSION DIRECTORY</span><span>{sessions.length}</span></div>
			{#if loading && !sessions.length}
				<div class="state">Loading sessions…</div>
			{:else if !sessions.length}
				<div class="state">No {harnessLabel} sessions found.</div>
			{:else}
				<div class="rows">
					{#each pageSessions as session (session.id)}
						<button
							class:selected={selectedID === session.id}
							class="session-row"
							onclick={() => loadDetail(session.id)}
						>
							<div class="row-top">
								<span class="session-id">{session.id}</span><span class="message-count"
									>{session.messageCount} msg</span
								>
							</div>
							<div class="session-title">{session.title || 'Untitled session'}</div>
							<div class="row-meta"><Clock3Icon size={12} />{formatDate(session.updated)}</div>
							{#if session.cwd}<div class="row-meta cwd">
									<FolderOpenIcon size={12} />{session.cwd}
								</div>{/if}
						</button>
					{/each}
				</div>
			{/if}
			{#if sessions.length > pageSize}
				<div class="pagination">
					<button onclick={() => setPage(page - 1)} disabled={page === 0}>Previous</button>
					<span>Page {page + 1} of {pageCount}</span>
					<button onclick={() => setPage(page + 1)} disabled={page >= pageCount - 1}>Next</button>
				</div>
			{/if}
		</aside>

		<section class="detail" aria-live="polite">
			{#if detailLoading}
				<div class="detail-state">Loading session…</div>
			{:else if detailError}
				<div class="detail-state error-text">Could not load this session: {detailError}</div>
			{:else if !detail}
				<div class="detail-state">
					<MessageSquareIcon size={22} />Select a session to inspect its log.
				</div>
			{:else}
				<div class="detail-head">
					<div>
						<p class="eyebrow">SESSION LOG</p>
						<h2>{detail.title || detail.id}</h2>
						<code>{detail.id}</code>
					</div>
					<div class="detail-facts">
						<span><Clock3Icon size={13} />{formatDate(detail.updated)}</span><span
							><MessageSquareIcon size={13} />{detail.messages.length} messages</span
						>
					</div>
				</div>
				{#if detail.cwd}<div class="cwd-banner"><FolderOpenIcon size={14} />{detail.cwd}</div>{/if}
				{#if !detail.messages.length}
					<div class="detail-state">This session has no messages.</div>
				{:else}
					<div class="messages">
						{#each detail.messages as message, index}
							<article class="message-card role-{message.role || 'unknown'}">
								<div class="message-label">
									<span>{roleLabel(message.role)}</span>
									<span class="message-stats">
										<span class="message-tokens">tokens={tokenCount(message.content)}</span>
										<span class="message-index">#{index + 1}</span>
									</span>
								</div>
								{#if message.toolCallCommand}
									<div class="tool-call">
										<span>TOOL CALL: {message.toolCallCommand}</span>
										{#if message.toolCallParameters !== undefined}
											<pre>{formatContent(message.toolCallParameters)}</pre>
										{/if}
									</div>
								{/if}
								<pre>{formatContent(message.content)}</pre>
							</article>
						{/each}
					</div>
				{/if}
			{/if}
		</section>
	</div>
</div>

<style>
	.session-page {
		height: 100%;
		min-height: 0;
		box-sizing: border-box;
		display: flex;
		flex-direction: column;
		overflow: hidden;
		background: var(--page-bg);
		color: var(--text);
		padding: 28px;
		font-family: 'DM Sans', 'Avenir Next', sans-serif;
	}
	.page-header,
	.detail-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 20px;
	}
	.page-header {
		flex-shrink: 0;
	}
	.eyebrow {
		margin: 0 0 8px;
		color: var(--accent);
		font-size: 10px;
		font-weight: 700;
		letter-spacing: 0.16em;
	}
	h1,
	h2 {
		margin: 0;
		letter-spacing: -0.03em;
	}
	h1 {
		font-size: 28px;
	}
	h2 {
		font-size: 22px;
	}
	.intro {
		margin: 8px 0 0;
		color: var(--secondary);
		font-size: 13px;
	}
	.header-actions {
		display: flex;
		align-items: center;
		gap: 12px;
	}
	.count,
	.message-count {
		color: var(--muted);
		font-size: 12px;
	}
	.refresh {
		display: inline-flex;
		align-items: center;
		gap: 7px;
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 11px;
		background: var(--card-bg);
		color: var(--text);
		cursor: pointer;
		font: inherit;
		font-size: 12px;
	}
	.refresh:disabled {
		opacity: 0.55;
		cursor: default;
	}
	.spin {
		animation: spin 0.8s linear infinite;
	}
	.notice {
		flex-shrink: 0;
		margin-top: 18px;
		border: 1px solid #b45353;
		border-radius: 8px;
		padding: 10px 12px;
		font-size: 12px;
	}
	.workspace {
		flex: 1;
		display: grid;
		grid-template-columns: minmax(280px, 340px) minmax(0, 1fr);
		min-height: 0;
		margin-top: 24px;
		overflow: hidden;
		border: 1px solid var(--border);
		border-radius: 12px;
		background: var(--card-bg);
	}
	.session-list {
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		border-right: 1px solid var(--border);
		background: color-mix(in srgb, var(--surface) 42%, var(--card-bg));
	}
	.list-heading {
		display: flex;
		justify-content: space-between;
		padding: 14px 16px;
		border-bottom: 1px solid var(--border);
		color: var(--muted);
		font-size: 10px;
		font-weight: 700;
		letter-spacing: 0.13em;
	}
	.rows {
		flex: 1;
		min-height: 0;
		overflow-x: hidden;
		overflow-y: auto;
	}
	.pagination {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-shrink: 0;
		gap: 8px;
		border-top: 1px solid var(--border);
		padding: 10px 12px;
		color: var(--muted);
		font-size: 11px;
	}
	.pagination button {
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 5px 8px;
		background: var(--card-bg);
		color: var(--text);
		font: inherit;
		cursor: pointer;
	}
	.pagination button:disabled {
		cursor: default;
		opacity: 0.45;
	}
	.session-row {
		display: block;
		width: 100%;
		border: 0;
		border-bottom: 1px solid var(--border);
		padding: 14px 16px;
		background: transparent;
		color: var(--text);
		text-align: left;
		cursor: pointer;
		user-select: text;
		-webkit-user-select: text;
	}
	.session-row:hover,
	.session-row.selected {
		background: var(--accent-tint);
	}
	.row-top,
	.row-meta,
	.detail-facts span,
	.cwd-banner {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.row-top {
		justify-content: space-between;
		gap: 10px;
	}
	.session-id {
		overflow: hidden;
		color: var(--accent);
		font:
			12px 'Fira Code',
			monospace;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.session-title {
		overflow: hidden;
		margin-top: 7px;
		color: var(--text);
		font-size: 13px;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.row-meta {
		margin-top: 8px;
		color: var(--muted);
		font-size: 11px;
	}
	.cwd {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.state,
	.detail-state {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 9px;
		min-height: 150px;
		padding: 24px;
		color: var(--muted);
		font-size: 13px;
		text-align: center;
	}
	.detail {
		min-width: 0;
		min-height: 0;
		padding: 24px;
		overflow-x: hidden;
		overflow-y: auto;
		user-select: text;
		-webkit-user-select: text;
	}
	.detail-head code {
		display: inline-block;
		margin-top: 9px;
		color: var(--muted);
		font:
			11px 'Fira Code',
			monospace;
	}
	.detail-facts {
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		gap: 12px;
		color: var(--secondary);
		font-size: 11px;
	}
	.cwd-banner {
		margin: 22px 0 18px;
		overflow: hidden;
		border: 1px solid var(--border);
		border-radius: 7px;
		padding: 9px 11px;
		color: var(--secondary);
		font:
			11px 'Fira Code',
			monospace;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.messages {
		display: grid;
		gap: 14px;
	}
	.message-card {
		border: 1px solid var(--border);
		border-left: 3px solid var(--muted);
		border-radius: 8px;
		background: var(--surface);
	}
	.message-card.role-user {
		border-left-color: #38bdf8;
	}
	.message-card.role-assistant {
		border-left-color: var(--accent);
	}
	.message-card.role-system {
		border-left-color: var(--muted);
		opacity: 0.82;
	}
	.message-label {
		display: flex;
		justify-content: space-between;
		padding: 10px 13px 8px;
		color: var(--secondary);
		font-size: 10px;
		font-weight: 700;
		letter-spacing: 0.12em;
	}
	.message-index {
		color: var(--muted);
		font-weight: 400;
		letter-spacing: 0;
	}
	.message-stats {
		display: inline-flex;
		align-items: center;
		gap: 10px;
	}
	.message-tokens {
		color: var(--accent);
		font-weight: 600;
		letter-spacing: 0;
	}
	.tool-call {
		border-top: 1px solid var(--border);
		padding: 10px 13px 0;
		color: var(--accent);
		font-size: 10px;
		font-weight: 700;
		letter-spacing: 0.1em;
	}
	.tool-call pre {
		margin: 8px -13px 0;
		border-top: 1px solid var(--border);
		color: var(--secondary);
		font-weight: 400;
		letter-spacing: 0;
	}
	pre {
		margin: 0;
		overflow: auto;
		border-top: 1px solid var(--border);
		padding: 13px;
		color: var(--text);
		font:
			12px/1.6 'Fira Code',
			'SFMono-Regular',
			monospace;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.error-text {
		color: #fca5a5;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	@media (max-width: 760px) {
		.session-page {
			padding: 18px;
		}
		.page-header,
		.detail-head {
			flex-direction: column;
		}
		.header-actions {
			width: 100%;
			justify-content: space-between;
		}
		.workspace {
			grid-template-columns: 1fr;
			grid-template-rows: minmax(0, 1fr) minmax(0, 1fr);
		}
		.session-list {
			border-right: 0;
			border-bottom: 1px solid var(--border);
		}
		.rows {
			max-height: none;
		}
		.detail {
			min-height: 0;
			padding: 18px;
		}
		.detail-facts {
			justify-content: flex-start;
		}
	}
</style>
