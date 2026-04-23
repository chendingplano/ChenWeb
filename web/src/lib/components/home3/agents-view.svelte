<script lang="ts">
	import { onMount } from 'svelte';
	import type { Agent, CreateAgentBody } from './agentplatform-client';
	import { apStore } from './agentplatform-store.svelte';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	const runtimeKinds: Array<{ value: Agent['runtime_kind']; label: string }> = [
		{ value: 'claude_code', label: 'Claude Code' },
		{ value: 'codex', label: 'Codex' },
		{ value: 'openclaw', label: 'OpenClaw' },
		{ value: 'opencode', label: 'OpenCode' }
	];

	function runtimeLabel(k: Agent['runtime_kind']): string {
		return runtimeKinds.find((r) => r.value === k)?.label ?? k;
	}

	let showCreate = $state(false);
	let submitting = $state(false);
	let localError = $state<string | null>(null);
	let draft = $state<CreateAgentBody>({
		name: '',
		runtime_kind: 'claude_code',
		avatar_emoji: '🤖',
		model: '',
		instructions: ''
	});

	onMount(() => {
		if (apStore.workspaces.length === 0) {
			apStore.loadWorkspaces();
		} else if (apStore.active) {
			apStore.loadAgents();
		}
	});

	async function submit() {
		localError = null;
		const name = draft.name.trim();
		if (!name) {
			localError = 'Name is required';
			return;
		}
		submitting = true;
		try {
			await apStore.createAgent({ ...draft, name });
			draft = {
				name: '',
				runtime_kind: 'claude_code',
				avatar_emoji: '🤖',
				model: '',
				instructions: ''
			};
			showCreate = false;
		} catch (e) {
			localError = String((e as Error).message ?? e);
		} finally {
			submitting = false;
		}
	}

	async function remove(a: Agent) {
		if (!confirm(`Archive agent "${a.name}"? Existing issues keep this reference.`)) return;
		try {
			await apStore.deleteAgent(a.id);
		} catch (e) {
			localError = String((e as Error).message ?? e);
		}
	}

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived(darkMode ? '#818CF8' : '#6366F1');
</script>

<div
	class="wrap"
	style:--page={pageBg}
	style:--card={card}
	style:--border={border}
	style:--heading={heading}
	style:--sub={sub}
	style:--btn={btn}
>
	<header class="toolbar">
		<div>
			<h2>Agents</h2>
			<p class="muted">
				{apStore.agents.length} agent{apStore.agents.length === 1 ? '' : 's'}
				{apStore.active ? ` in ${apStore.active.name}` : ''}
			</p>
		</div>
		<button
			class="primary"
			onclick={() => (showCreate = !showCreate)}
			disabled={!apStore.active}
		>
			{showCreate ? 'Cancel' : '+ New Agent'}
		</button>
	</header>

	{#if showCreate}
		<form
			class="create-form"
			onsubmit={(e) => {
				e.preventDefault();
				submit();
			}}
		>
			<div class="row">
				<label>
					<span>Emoji</span>
					<input class="emoji" bind:value={draft.avatar_emoji} maxlength="4" />
				</label>
				<label class="grow">
					<span>Name</span>
					<input bind:value={draft.name} required placeholder="e.g. Claude-Backend" />
				</label>
				<label>
					<span>Runtime</span>
					<select bind:value={draft.runtime_kind}>
						{#each runtimeKinds as rk (rk.value)}
							<option value={rk.value}>{rk.label}</option>
						{/each}
					</select>
				</label>
			</div>
			<label>
				<span>Model (optional)</span>
				<input bind:value={draft.model} placeholder="e.g. claude-opus-4-7" />
			</label>
			<label>
				<span>Instructions (optional system prompt)</span>
				<textarea rows="4" bind:value={draft.instructions}></textarea>
			</label>
			<div class="form-foot">
				<button class="primary" type="submit" disabled={submitting || !draft.name.trim()}>
					{submitting ? 'Creating…' : 'Create agent'}
				</button>
			</div>
		</form>
	{/if}

	{#if localError || apStore.error}
		<div class="error" role="alert">{localError ?? apStore.error}</div>
	{/if}

	<div class="grid">
		{#each apStore.agents as a (a.id)}
			<article class="agent-card">
				<header>
					<span class="emoji">{a.avatar_emoji}</span>
					<div class="meta">
						<span class="name">{a.name}</span>
						<span class="runtime">{runtimeLabel(a.runtime_kind)}</span>
					</div>
					<button class="danger-link" onclick={() => remove(a)}>Archive</button>
				</header>
				{#if a.model}
					<p class="muted small">Model: {a.model}</p>
				{/if}
				{#if a.instructions}
					<p class="instructions">{a.instructions}</p>
				{/if}
			</article>
		{/each}
		{#if apStore.agents.length === 0}
			<div class="empty">No agents yet. Create one to assign issues.</div>
		{/if}
	</div>
</div>

<style>
	.wrap {
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
	}
	h2 {
		margin: 0;
		font-size: 20px;
		color: var(--heading);
	}
	.muted,
	.small {
		color: var(--sub);
		font-size: 12px;
		margin: 4px 0 0;
	}
	.primary {
		background: var(--btn);
		color: white;
		border: none;
		padding: 8px 14px;
		border-radius: 8px;
		font-weight: 500;
		cursor: pointer;
		font-size: 13px;
	}
	.primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.danger-link {
		background: none;
		border: none;
		color: #f87171;
		font-size: 12px;
		cursor: pointer;
	}
	.create-form {
		display: flex;
		flex-direction: column;
		gap: 10px;
		background: var(--card);
		border: 1px solid var(--border);
		padding: 16px;
		border-radius: 10px;
	}
	.row {
		display: flex;
		gap: 10px;
	}
	.row .grow {
		flex: 1;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 12px;
		color: var(--sub);
	}
	.emoji {
		width: 52px;
		text-align: center;
	}
	input,
	select,
	textarea {
		background: var(--page);
		color: var(--heading);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		font-size: 13px;
		font-family: inherit;
	}
	.form-foot {
		display: flex;
		justify-content: flex-end;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
		gap: 12px;
	}
	.agent-card {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 14px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.agent-card header {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.agent-card .emoji {
		font-size: 24px;
	}
	.meta {
		flex: 1;
		display: flex;
		flex-direction: column;
	}
	.name {
		color: var(--heading);
		font-weight: 500;
	}
	.runtime {
		color: var(--sub);
		font-size: 11px;
	}
	.instructions {
		color: var(--heading);
		font-size: 12px;
		margin: 4px 0 0;
		white-space: pre-wrap;
	}
	.empty {
		color: var(--sub);
		font-style: italic;
		text-align: center;
		padding: 24px;
		grid-column: 1 / -1;
	}
	.error {
		background: rgba(248, 113, 113, 0.12);
		color: #f87171;
		padding: 10px 12px;
		border-radius: 8px;
		font-size: 13px;
	}
</style>
