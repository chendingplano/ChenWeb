<script lang="ts">
	import { onMount } from 'svelte';
	import type { Project } from './agentplatform-client';
	import { apStore } from './agentplatform-store.svelte';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let showCreate = $state(false);
	let submitting = $state(false);
	let localError = $state<string | null>(null);
	let newName = $state('');
	let newDesc = $state('');

	// Inline-edit state
	let editingId = $state<string | null>(null);
	let editName = $state('');
	let editDesc = $state('');
	let editSaving = $state(false);

	onMount(() => {
		if (apStore.workspaces.length === 0) {
			apStore.loadWorkspaces();
		} else if (apStore.active) {
			apStore.loadProjects();
		}
	});

	async function submitCreate() {
		localError = null;
		const name = newName.trim();
		if (!name) { localError = 'Name is required'; return; }
		submitting = true;
		try {
			await apStore.createProject({ name, description: newDesc.trim() || undefined });
			newName = '';
			newDesc = '';
			showCreate = false;
		} catch (e) {
			localError = String((e as Error).message ?? e);
		} finally {
			submitting = false;
		}
	}

	function startEdit(p: Project) {
		editingId = p.id;
		editName = p.name;
		editDesc = p.description;
	}

	function cancelEdit() {
		editingId = null;
	}

	async function saveEdit(id: string) {
		localError = null;
		const name = editName.trim();
		if (!name) { localError = 'Name is required'; return; }
		editSaving = true;
		try {
			await apStore.updateProject(id, { name, description: editDesc.trim() || undefined });
			editingId = null;
		} catch (e) {
			localError = String((e as Error).message ?? e);
		} finally {
			editSaving = false;
		}
	}

	async function remove(p: Project) {
		if (!confirm(`Delete project "${p.name}"? Issues in this project will lose their project association.`)) return;
		try {
			await apStore.deleteProject(p.id);
		} catch (e) {
			localError = String((e as Error).message ?? e);
		}
	}

	const pageBg  = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card    = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border  = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub     = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn     = $derived(darkMode ? '#818CF8' : '#6366F1');
	const inputBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
</script>

<div
	class="wrap"
	style:--page={pageBg}
	style:--card={card}
	style:--border={border}
	style:--heading={heading}
	style:--sub={sub}
	style:--btn={btn}
	style:--input-bg={inputBg}
>
	<header class="toolbar">
		<div>
			<h2>Projects</h2>
			<p class="muted">
				{apStore.projects.length} project{apStore.projects.length === 1 ? '' : 's'}
				{apStore.active ? ` in ${apStore.active.name}` : ''}
			</p>
		</div>
		<button
			class="primary"
			onclick={() => (showCreate = !showCreate)}
			disabled={!apStore.active}
		>
			{showCreate ? 'Cancel' : '+ New Project'}
		</button>
	</header>

	{#if showCreate}
		<form
			class="create-form"
			onsubmit={(e) => { e.preventDefault(); submitCreate(); }}
		>
			<label>
				<span>Name</span>
				<input
					bind:value={newName}
					required
					placeholder="e.g. Backend Refactor"
				/>
			</label>
			<label>
				<span>Description (optional)</span>
				<textarea rows="2" bind:value={newDesc} placeholder="Short description…"></textarea>
			</label>
			<div class="form-foot">
				<button class="primary" type="submit" disabled={submitting || !newName.trim()}>
					{submitting ? 'Creating…' : 'Create project'}
				</button>
			</div>
		</form>
	{/if}

	{#if localError || apStore.error}
		<div class="error" role="alert">{localError ?? apStore.error}</div>
	{/if}

	{#if apStore.projects.length === 0}
		<div class="empty">No projects yet. Create one to group your issues.</div>
	{:else}
		<ul class="list">
			{#each apStore.projects as p (p.id)}
				<li class="project-row">
					{#if editingId === p.id}
						<form
							class="edit-form"
							onsubmit={(e) => { e.preventDefault(); saveEdit(p.id); }}
						>
							<div class="edit-fields">
								<input class="edit-name" bind:value={editName} required placeholder="Project name" />
								<input class="edit-desc" bind:value={editDesc} placeholder="Description" />
							</div>
							<div class="edit-actions">
								<button class="primary small" type="submit" disabled={editSaving || !editName.trim()}>
									{editSaving ? 'Saving…' : 'Save'}
								</button>
								<button class="ghost small" type="button" onclick={cancelEdit}>Cancel</button>
							</div>
						</form>
					{:else}
						<div class="project-info">
							<span class="project-name">{p.name}</span>
							{#if p.description}
								<span class="project-desc">{p.description}</span>
							{/if}
						</div>
						<div class="project-actions">
							<span class="date">{new Date(p.created_at).toLocaleDateString()}</span>
							<button class="action-link" onclick={() => startEdit(p)}>Edit</button>
							<button class="danger-link" onclick={() => remove(p)}>Delete</button>
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
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
	h2 { margin: 0; font-size: 20px; color: var(--heading); }
	.muted { color: var(--sub); font-size: 12px; margin: 4px 0 0; }
	.primary {
		background: var(--btn);
		color: white;
		border: none;
		padding: 8px 14px;
		border-radius: 8px;
		font-weight: 500;
		cursor: pointer;
		font-size: 13px;
		transition: filter 150ms;
	}
	.primary:hover:not(:disabled) { filter: brightness(1.1); }
	.primary:disabled { opacity: 0.5; cursor: not-allowed; }
	.primary.small { padding: 5px 10px; font-size: 12px; }
	.ghost {
		background: transparent;
		color: var(--sub);
		border: 1px solid var(--border);
		padding: 8px 14px;
		border-radius: 8px;
		cursor: pointer;
		font-size: 13px;
	}
	.ghost.small { padding: 5px 10px; font-size: 12px; }
	.create-form {
		display: flex;
		flex-direction: column;
		gap: 10px;
		background: var(--card);
		border: 1px solid var(--border);
		padding: 16px;
		border-radius: 10px;
	}
	label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--sub); }
	input, textarea {
		background: var(--input-bg);
		color: var(--heading);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		font-size: 13px;
		font-family: inherit;
	}
	textarea { resize: vertical; }
	.form-foot { display: flex; justify-content: flex-end; }
	.list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
	.project-row {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 12px 16px;
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 12px;
	}
	.project-info { display: flex; flex-direction: column; gap: 2px; flex: 1; min-width: 0; }
	.project-name { color: var(--heading); font-weight: 500; font-size: 14px; }
	.project-desc { color: var(--sub); font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.project-actions { display: flex; align-items: center; gap: 10px; flex-shrink: 0; }
	.date { font-size: 11px; color: var(--sub); }
	.action-link { background: none; border: none; color: var(--btn); font-size: 12px; cursor: pointer; }
	.danger-link { background: none; border: none; color: #f87171; font-size: 12px; cursor: pointer; }
	.edit-form { flex: 1; display: flex; align-items: center; gap: 10px; }
	.edit-fields { flex: 1; display: flex; gap: 8px; min-width: 0; }
	.edit-name { flex: 0 0 200px; }
	.edit-desc { flex: 1; min-width: 0; }
	.edit-actions { display: flex; gap: 6px; flex-shrink: 0; }
	.empty { color: var(--sub); font-style: italic; text-align: center; padding: 40px 24px; }
	.error { background: rgba(248,113,113,0.12); color: #f87171; padding: 10px 12px; border-radius: 8px; font-size: 13px; }
</style>
