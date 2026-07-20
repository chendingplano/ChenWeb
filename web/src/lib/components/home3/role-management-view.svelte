<script lang="ts">
	type RoleRow = {
		key: string;
		label: string;
		description: string;
		status: 'active' | 'reserved' | 'disabled';
	};

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let roles = $state<RoleRow[]>([
		{ key: 'admin', label: 'Administrator', description: 'Full administrative access.', status: 'active' },
		{ key: 'dev', label: 'Developer', description: 'Engineering and development workflows.', status: 'active' },
		{ key: 'k_engineer', label: 'Knowledge Engineer', description: 'Knowledge-system and taxonomy operations.', status: 'active' },
		{ key: 'root', label: 'Root', description: 'Reserved role; not active yet.', status: 'reserved' }
	]);

	let showCreate = $state(false);
	let editingKey = $state<string | null>(null);
	let error = $state<string | null>(null);

	let createDraft = $state({
		key: '',
		label: '',
		description: '',
		status: 'active' as RoleRow['status']
	});

	let editDraft = $state({
		key: '',
		label: '',
		description: '',
		status: 'active' as RoleRow['status']
	});

	function normalizeKey(key: string) {
		return key.trim().toLowerCase().replace(/\s+/g, '_');
	}

	function validateDraft(key: string, label: string) {
		if (!key.trim()) return 'Role key is required.';
		if (!label.trim()) return 'Role label is required.';
		return null;
	}

	function submitCreate() {
		const normalizedKey = normalizeKey(createDraft.key);
		error = validateDraft(normalizedKey, createDraft.label);
		if (error) return;
		if (roles.some((role) => role.key === normalizedKey)) {
			error = `Role "${normalizedKey}" already exists.`;
			return;
		}
		roles = [
			{
				key: normalizedKey,
				label: createDraft.label.trim(),
				description: createDraft.description.trim(),
				status: createDraft.status
			},
			...roles
		];
		createDraft = { key: '', label: '', description: '', status: 'active' };
		showCreate = false;
		error = null;
	}

	function startEdit(role: RoleRow) {
		editingKey = role.key;
		editDraft = { ...role };
		error = null;
	}

	function cancelEdit() {
		editingKey = null;
		error = null;
	}

	function saveEdit(key: string) {
		const normalizedKey = normalizeKey(editDraft.key);
		error = validateDraft(normalizedKey, editDraft.label);
		if (error) return;
		if (normalizedKey !== key && roles.some((role) => role.key === normalizedKey)) {
			error = `Role "${normalizedKey}" already exists.`;
			return;
		}
		roles = roles.map((role) =>
			role.key === key
				? {
						key: normalizedKey,
						label: editDraft.label.trim(),
						description: editDraft.description.trim(),
						status: editDraft.status
					}
				: role
		);
		editingKey = null;
		error = null;
	}

	function removeRole(role: RoleRow) {
		if (!confirm(`Delete role "${role.key}"?`)) return;
		roles = roles.filter((row) => row.key !== role.key);
	}

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const panel = $derived(darkMode ? '#151A29' : '#FDFDFD');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived(darkMode ? '#0F766E' : '#0F766E');
	const inputBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const danger = '#dc2626';
</script>

<div
	class="wrap"
	style:--page={pageBg}
	style:--card={card}
	style:--panel={panel}
	style:--border={border}
	style:--heading={heading}
	style:--sub={sub}
	style:--btn={btn}
	style:--input-bg={inputBg}
	style:--danger={danger}
>
	<header class="toolbar">
		<div>
			<h2>Role Management</h2>
			<p class="muted">Manage the shared role catalog used by Kratos-backed identities.</p>
		</div>
		<button class="primary" onclick={() => (showCreate = !showCreate)}>
			{showCreate ? 'Cancel' : '+ Add Role'}
		</button>
	</header>

	{#if showCreate}
		<form class="editor" onsubmit={(event) => { event.preventDefault(); submitCreate(); }}>
			<div class="row three">
				<label><span>Role Key</span><input bind:value={createDraft.key} placeholder="k_engineer" /></label>
				<label><span>Label</span><input bind:value={createDraft.label} placeholder="Knowledge Engineer" /></label>
				<label>
					<span>Status</span>
					<select bind:value={createDraft.status}>
						<option value="active">active</option>
						<option value="reserved">reserved</option>
						<option value="disabled">disabled</option>
					</select>
				</label>
			</div>
			<label><span>Description</span><input bind:value={createDraft.description} placeholder="What this role is intended for." /></label>
			<div class="actions"><button class="primary" type="submit">Create Role</button></div>
		</form>
	{/if}

	{#if error}
		<div class="error" role="alert">{error}</div>
	{/if}

	<div class="table-card">
		<table>
			<thead>
				<tr>
					<th>Role</th>
					<th>Status</th>
					<th>Description</th>
					<th>Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each roles as role (role.key)}
					<tr>
						{#if editingKey === role.key}
							<td colspan="4">
								<form class="editor compact" onsubmit={(event) => { event.preventDefault(); saveEdit(role.key); }}>
									<div class="row three">
										<label><span>Role Key</span><input bind:value={editDraft.key} /></label>
										<label><span>Label</span><input bind:value={editDraft.label} /></label>
										<label>
											<span>Status</span>
											<select bind:value={editDraft.status}>
												<option value="active">active</option>
												<option value="reserved">reserved</option>
												<option value="disabled">disabled</option>
											</select>
										</label>
									</div>
									<label><span>Description</span><input bind:value={editDraft.description} /></label>
									<div class="actions">
										<button class="primary" type="submit">Save</button>
										<button class="ghost" type="button" onclick={cancelEdit}>Cancel</button>
									</div>
								</form>
							</td>
						{:else}
							<td>
								<div class="role-cell">
									<strong>{role.label}</strong>
									<span>{role.key}</span>
								</div>
							</td>
							<td><span class="status">{role.status}</span></td>
							<td class="description">{role.description}</td>
							<td>
								<div class="row-actions">
									<button class="link" onclick={() => startEdit(role)}>Edit</button>
									<button class="danger-link" onclick={() => removeRole(role)}>Delete</button>
								</div>
							</td>
						{/if}
					</tr>
				{/each}
			</tbody>
		</table>
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
	.toolbar, .row, .actions, .row-actions, .role-cell {
		display: flex;
	}
	.toolbar {
		justify-content: space-between;
		align-items: flex-end;
		gap: 16px;
	}
	h2 { margin: 0; font-size: 20px; color: var(--heading); }
	.muted { margin: 4px 0 0; color: var(--sub); font-size: 12px; }
	.primary, .ghost, .link, .danger-link {
		border-radius: 8px;
		cursor: pointer;
		font-size: 13px;
	}
	.primary {
		background: var(--btn);
		border: none;
		color: white;
		font-weight: 600;
		padding: 8px 14px;
	}
	.ghost {
		background: transparent;
		border: 1px solid var(--border);
		color: var(--heading);
		padding: 8px 14px;
	}
	.editor, .table-card {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 16px;
	}
	.editor {
		display: flex;
		flex-direction: column;
		gap: 14px;
		padding: 16px;
	}
	.editor.compact {
		background: var(--panel);
		border-style: dashed;
	}
	.row {
		gap: 12px;
		flex-wrap: wrap;
	}
	.row.three > label { flex: 1 1 220px; }
	label {
		display: flex;
		flex-direction: column;
		gap: 6px;
		color: var(--sub);
		font-size: 12px;
	}
	input, select {
		background: var(--input-bg);
		border: 1px solid var(--border);
		border-radius: 10px;
		color: var(--heading);
		font-size: 13px;
		padding: 10px 12px;
	}
	.actions {
		gap: 10px;
		justify-content: flex-end;
	}
	.error {
		background: color-mix(in srgb, var(--danger) 14%, transparent);
		border: 1px solid color-mix(in srgb, var(--danger) 35%, transparent);
		border-radius: 12px;
		color: var(--danger);
		padding: 12px 14px;
	}
	.table-card {
		overflow: hidden;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th, td {
		border-top: 1px solid var(--border);
		padding: 14px 16px;
		text-align: left;
		vertical-align: top;
	}
	th {
		border-top: none;
		color: var(--sub);
		font-size: 11px;
		letter-spacing: 0.12em;
		text-transform: uppercase;
	}
	.role-cell {
		flex-direction: column;
		gap: 4px;
	}
	.role-cell strong {
		color: var(--heading);
		font-size: 14px;
	}
	.role-cell span, .description {
		color: var(--sub);
		font-size: 12px;
	}
	.status {
		display: inline-flex;
		align-items: center;
		border-radius: 999px;
		padding: 5px 10px;
		background: color-mix(in srgb, var(--btn) 16%, transparent);
		color: var(--btn);
		font-size: 12px;
	}
	.row-actions {
		gap: 12px;
	}
	.link, .danger-link {
		background: transparent;
		border: none;
		padding: 0;
	}
	.link { color: var(--btn); }
	.danger-link { color: var(--danger); }
</style>
