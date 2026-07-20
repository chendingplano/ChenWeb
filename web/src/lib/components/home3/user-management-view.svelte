<script lang="ts">
	type UserRow = {
		id: string;
		name: string;
		email: string;
		status: 'active' | 'inactive' | 'trial';
		admin: boolean;
		roles: string[];
	};

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let users = $state<UserRow[]>([
		{
			id: 'u-001',
			name: 'Alex Johnson',
			email: 'alex@example.com',
			status: 'active',
			admin: true,
			roles: ['admin', 'dev']
		},
		{
			id: 'u-002',
			name: 'Mia Chen',
			email: 'mia.chen@example.com',
			status: 'active',
			admin: false,
			roles: ['k_engineer']
		},
		{
			id: 'u-003',
			name: 'Noah Patel',
			email: 'noah.patel@example.com',
			status: 'trial',
			admin: false,
			roles: ['trial']
		}
	]);

	let showCreate = $state(false);
	let editingId = $state<string | null>(null);
	let error = $state<string | null>(null);

	let createDraft = $state({
		name: '',
		email: '',
		status: 'active' as UserRow['status'],
		admin: false,
		roles: ''
	});

	let editDraft = $state({
		name: '',
		email: '',
		status: 'active' as UserRow['status'],
		admin: false,
		roles: ''
	});

	function normalizeRoles(raw: string, admin: boolean): string[] {
		const roles = Array.from(
			new Set(
				raw
					.split(',')
					.map((role) => role.trim().toLowerCase())
					.filter(Boolean)
			)
		).sort();
		if (admin && !roles.includes('admin')) roles.unshift('admin');
		if (!admin) return roles.filter((role) => role !== 'admin');
		return Array.from(new Set(roles)).sort();
	}

	function validateDraft(name: string, email: string) {
		if (!name.trim()) return 'Name is required.';
		if (!email.trim()) return 'Email is required.';
		if (!email.includes('@')) return 'Email must look valid.';
		return null;
	}

	function submitCreate() {
		error = validateDraft(createDraft.name, createDraft.email);
		if (error) return;
		users = [
			{
				id: `u-${String(users.length + 1).padStart(3, '0')}`,
				name: createDraft.name.trim(),
				email: createDraft.email.trim(),
				status: createDraft.status,
				admin: createDraft.admin,
				roles: normalizeRoles(createDraft.roles, createDraft.admin)
			},
			...users
		];
		createDraft = { name: '', email: '', status: 'active', admin: false, roles: '' };
		showCreate = false;
		error = null;
	}

	function startEdit(user: UserRow) {
		editingId = user.id;
		editDraft = {
			name: user.name,
			email: user.email,
			status: user.status,
			admin: user.admin,
			roles: user.roles.filter((role) => role !== 'admin').join(', ')
		};
		error = null;
	}

	function cancelEdit() {
		editingId = null;
		error = null;
	}

	function saveEdit(id: string) {
		error = validateDraft(editDraft.name, editDraft.email);
		if (error) return;
		users = users.map((user) =>
			user.id === id
				? {
						...user,
						name: editDraft.name.trim(),
						email: editDraft.email.trim(),
						status: editDraft.status,
						admin: editDraft.admin,
						roles: normalizeRoles(editDraft.roles, editDraft.admin)
					}
				: user
		);
		editingId = null;
		error = null;
	}

	function removeUser(user: UserRow) {
		if (!confirm(`Delete user "${user.name}"?`)) return;
		users = users.filter((row) => row.id !== user.id);
	}

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const panel = $derived(darkMode ? '#151A29' : '#FDFDFD');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived(darkMode ? '#6366F1' : '#4F46E5');
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
			<h2>User Management</h2>
			<p class="muted">View, add, update, and remove Kratos-backed users.</p>
		</div>
		<button class="primary" onclick={() => (showCreate = !showCreate)}>
			{showCreate ? 'Cancel' : '+ Add User'}
		</button>
	</header>

	<div class="summary-grid">
		<div class="summary-card">
			<div class="summary-label">Total Users</div>
			<div class="summary-value">{users.length}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Admins</div>
			<div class="summary-value">{users.filter((user) => user.admin).length}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Trial Users</div>
			<div class="summary-value">{users.filter((user) => user.status === 'trial').length}</div>
		</div>
	</div>

	{#if showCreate}
		<form class="editor" onsubmit={(event) => { event.preventDefault(); submitCreate(); }}>
			<div class="row two">
				<label><span>Name</span><input bind:value={createDraft.name} placeholder="Jane Doe" /></label>
				<label><span>Email</span><input bind:value={createDraft.email} placeholder="jane@example.com" /></label>
			</div>
			<div class="row three">
				<label>
					<span>Status</span>
					<select bind:value={createDraft.status}>
						<option value="active">active</option>
						<option value="inactive">inactive</option>
						<option value="trial">trial</option>
					</select>
				</label>
				<label class="toggle">
					<span>Admin</span>
					<input type="checkbox" bind:checked={createDraft.admin} />
				</label>
				<label><span>Roles</span><input bind:value={createDraft.roles} placeholder="dev, k_engineer" /></label>
			</div>
			<div class="actions"><button class="primary" type="submit">Create User</button></div>
		</form>
	{/if}

	{#if error}
		<div class="error" role="alert">{error}</div>
	{/if}

	<div class="table-card">
		<table>
			<thead>
				<tr>
					<th>User</th>
					<th>Status</th>
					<th>Roles</th>
					<th>Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each users as user (user.id)}
					<tr>
						{#if editingId === user.id}
							<td colspan="4">
								<form class="editor compact" onsubmit={(event) => { event.preventDefault(); saveEdit(user.id); }}>
									<div class="row two">
										<label><span>Name</span><input bind:value={editDraft.name} /></label>
										<label><span>Email</span><input bind:value={editDraft.email} /></label>
									</div>
									<div class="row three">
										<label>
											<span>Status</span>
											<select bind:value={editDraft.status}>
												<option value="active">active</option>
												<option value="inactive">inactive</option>
												<option value="trial">trial</option>
											</select>
										</label>
										<label class="toggle">
											<span>Admin</span>
											<input type="checkbox" bind:checked={editDraft.admin} />
										</label>
										<label><span>Roles</span><input bind:value={editDraft.roles} placeholder="dev, trial" /></label>
									</div>
									<div class="actions">
										<button class="primary" type="submit">Save</button>
										<button class="ghost" type="button" onclick={cancelEdit}>Cancel</button>
									</div>
								</form>
							</td>
						{:else}
							<td>
								<div class="user-cell">
									<strong>{user.name}</strong>
									<span>{user.email}</span>
								</div>
							</td>
							<td><span class="status">{user.status}</span></td>
							<td>
								<div class="chips">
									{#each user.roles as role}
										<span class="chip">{role}</span>
									{/each}
								</div>
							</td>
							<td>
								<div class="row-actions">
									<button class="link" onclick={() => startEdit(user)}>Edit</button>
									<button class="danger-link" onclick={() => removeUser(user)}>Delete</button>
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
	.toolbar, .summary-grid, .actions, .row, .row-actions, .chips, .user-cell {
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
	.summary-grid {
		gap: 16px;
		flex-wrap: wrap;
	}
	.summary-card, .editor, .table-card {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 16px;
	}
	.summary-card {
		min-width: 170px;
		padding: 16px;
	}
	.summary-label {
		color: var(--sub);
		font-size: 11px;
		letter-spacing: 0.12em;
		text-transform: uppercase;
	}
	.summary-value {
		color: var(--heading);
		font-size: 28px;
		font-weight: 700;
		margin-top: 6px;
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
	.row.two > label { flex: 1 1 280px; }
	.row.three > label { flex: 1 1 200px; }
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
	.toggle {
		justify-content: flex-end;
	}
	.toggle input {
		width: 18px;
		height: 18px;
		padding: 0;
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
	.user-cell {
		flex-direction: column;
		gap: 4px;
	}
	.user-cell strong {
		color: var(--heading);
		font-size: 14px;
	}
	.user-cell span {
		color: var(--sub);
		font-size: 12px;
	}
	.status, .chip {
		display: inline-flex;
		align-items: center;
		border-radius: 999px;
		padding: 5px 10px;
		font-size: 12px;
	}
	.status {
		background: color-mix(in srgb, var(--btn) 16%, transparent);
		color: var(--btn);
	}
	.chips {
		flex-wrap: wrap;
		gap: 8px;
	}
	.chip {
		background: var(--panel);
		border: 1px solid var(--border);
		color: var(--heading);
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
