<script lang="ts">
	import { onMount } from 'svelte';
	import {
		deleteManagedUser,
		listManagedUsers,
		updateManagedUser,
		type ManagedUser
	} from '$lib/services/userManagementService';

	type UserRow = {
		id: string;
		name: string;
		email: string;
		firstName: string;
		lastName: string;
		status: 'active' | 'inactive' | 'trial';
		admin: boolean;
		roles: string[];
	};

	type EditDraft = {
		id: string;
		email: string;
		firstName: string;
		lastName: string;
		status: UserRow['status'];
		admin: boolean;
		roles: string;
	};

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let managedUsers = $state<ManagedUser[]>([]);
	let users = $state<UserRow[]>([]);
	let loading = $state(true);
	let saving = $state(false);
	let deletingId = $state<string | null>(null);
	let error = $state<string | null>(null);
	let success = $state<string | null>(null);
	let editDialogOpen = $state(false);
	let editDraft = $state<EditDraft>({
		id: '',
		email: '',
		firstName: '',
		lastName: '',
		status: 'active',
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

	function displayName(user: ManagedUser): string {
		const fullName = [user.first_name, user.last_name].map((part) => part?.trim()).filter(Boolean).join(' ');
		return fullName || user.name?.trim() || user.email?.trim() || user.id;
	}

	function mapStatus(user: ManagedUser): UserRow['status'] {
		if (user.user_status && user.user_status !== 'active') return 'inactive';
		if (user.roles?.includes('trial')) return 'trial';
		return 'active';
	}

	function mapUser(user: ManagedUser): UserRow {
		const roles = normalizeRoles((user.roles ?? []).join(','), Boolean(user.admin));
		return {
			id: user.id,
			name: displayName(user),
			email: user.email,
			firstName: user.first_name ?? '',
			lastName: user.last_name ?? '',
			status: mapStatus(user),
			admin: Boolean(user.admin),
			roles
		};
	}

	function syncUsers() {
		users = managedUsers.map(mapUser).sort((a, b) => a.name.localeCompare(b.name));
	}

	async function loadUsers() {
		loading = true;
		error = null;
		try {
			managedUsers = await listManagedUsers();
			syncUsers();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load users.';
		} finally {
			loading = false;
		}
	}

	function notImplemented(message: string) {
		success = null;
		error = message;
	}

	function startEdit(user: UserRow) {
		success = null;
		error = null;
		editDraft = {
			id: user.id,
			email: user.email,
			firstName: user.firstName,
			lastName: user.lastName,
			status: user.status,
			admin: user.admin,
			roles: user.roles.filter((role) => role !== 'admin').join(', ')
		};
		editDialogOpen = true;
	}

	function closeEditDialog() {
		if (saving) return;
		editDialogOpen = false;
	}

	async function submitEdit() {
		saving = true;
		success = null;
		error = null;
		try {
			const updated = await updateManagedUser(editDraft.id, {
				first_name: editDraft.firstName.trim(),
				last_name: editDraft.lastName.trim(),
				status: editDraft.status,
				admin: editDraft.admin,
				roles: normalizeRoles(editDraft.roles, editDraft.admin).filter((role) => role !== 'admin')
			});
			managedUsers = managedUsers.map((user) => (user.id === updated.id ? updated : user));
			syncUsers();
			editDialogOpen = false;
			success = `Updated account ${updated.email}.`;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update user.';
		} finally {
			saving = false;
		}
	}

	async function removeUser(user: UserRow) {
		if (!confirm(`Delete user "${user.email}"?`)) return;
		deletingId = user.id;
		success = null;
		error = null;
		try {
			await deleteManagedUser(user.id);
			managedUsers = managedUsers.filter((entry) => entry.id !== user.id);
			syncUsers();
			success = `Deleted account ${user.email}.`;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to delete user.';
		} finally {
			deletingId = null;
		}
	}

	onMount(() => {
		void loadUsers();
	});

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const panel = $derived(darkMode ? '#151A29' : '#FDFDFD');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived(darkMode ? '#6366F1' : '#4F46E5');
	const danger = '#dc2626';
	const overlay = $derived('rgba(4, 10, 22, 0.72)');
	const inputBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
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
	style:--danger={danger}
	style:--overlay={overlay}
	style:--input-bg={inputBg}
>
	<header class="toolbar">
		<div>
			<h2>User Management</h2>
			<p class="muted">View live Kratos-backed users and their current role assignments.</p>
		</div>
		<button class="primary" onclick={() => notImplemented('Create user is not wired to Kratos yet.')}>
			+ Add User
		</button>
	</header>

	{#if success}
		<div class="success" role="status">{success}</div>
	{/if}

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
				{#if loading}
					<tr>
						<td colspan="4" class="empty-state">Loading users from Kratos...</td>
					</tr>
				{:else if users.length === 0}
					<tr>
						<td colspan="4" class="empty-state">No Kratos users were returned.</td>
					</tr>
				{:else}
				{#each users as user (user.id)}
					<tr>
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
								{#if user.roles.length === 0}
									<span class="chip chip-muted">no roles</span>
								{/if}
							</div>
						</td>
						<td>
							<div class="row-actions">
								<button class="link" onclick={() => startEdit(user)}>Edit</button>
								<button class="danger-link" onclick={() => removeUser(user)} disabled={deletingId === user.id}>
									{deletingId === user.id ? 'Deleting…' : 'Delete'}
								</button>
							</div>
						</td>
					</tr>
				{/each}
				{/if}
			</tbody>
		</table>
	</div>
</div>

{#if editDialogOpen}
	<div
		class="dialog-backdrop"
		role="button"
		tabindex="-1"
		onclick={closeEditDialog}
		onkeydown={(event) => event.key === 'Escape' && closeEditDialog()}
	>
		<div
			class="dialog"
			role="dialog"
			aria-modal="true"
			aria-label="Edit user"
			tabindex="-1"
			onclick={(event) => event.stopPropagation()}
			onkeydown={(event) => event.stopPropagation()}
		>
			<div class="dialog-head">
				<div>
					<div class="dialog-title">Edit Account</div>
					<div class="dialog-subtitle">Use the email address below to confirm the exact account.</div>
				</div>
				<button type="button" class="ghost" onclick={closeEditDialog} disabled={saving}>Close</button>
			</div>
			<div class="dialog-body">
				<label class="wide">
					<span>Email Address</span>
					<input value={editDraft.email} readonly />
				</label>
				<label>
					<span>First Name</span>
					<input bind:value={editDraft.firstName} placeholder="Chen" />
				</label>
				<label>
					<span>Last Name</span>
					<input bind:value={editDraft.lastName} placeholder="Ding" />
				</label>
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
				<label class="wide">
					<span>Roles</span>
					<input bind:value={editDraft.roles} placeholder="dev, k_engineer" />
				</label>
			</div>
			<div class="dialog-foot">
				<button type="button" class="ghost" onclick={closeEditDialog} disabled={saving}>Cancel</button>
				<button type="button" class="primary" onclick={submitEdit} disabled={saving}>
					{saving ? 'Saving…' : 'Save Changes'}
				</button>
			</div>
		</div>
	</div>
{/if}

<style>
	.wrap {
		display: flex;
		flex-direction: column;
		gap: 16px;
		background: var(--page);
		min-height: 100%;
		padding: 16px 20px 32px;
	}
	.toolbar, .summary-grid, .row-actions, .chips, .user-cell {
		display: flex;
	}
	.toolbar {
		justify-content: space-between;
		align-items: flex-end;
		gap: 16px;
	}
	h2 { margin: 0; font-size: 20px; color: var(--heading); }
	.muted { margin: 4px 0 0; color: var(--sub); font-size: 12px; }
	.primary, .link, .danger-link {
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
	.summary-grid {
		gap: 16px;
		flex-wrap: wrap;
	}
	.summary-card, .table-card {
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
	.error {
		background: color-mix(in srgb, var(--danger) 14%, transparent);
		border: 1px solid color-mix(in srgb, var(--danger) 35%, transparent);
		border-radius: 12px;
		color: var(--danger);
		padding: 12px 14px;
	}
	.success {
		background: color-mix(in srgb, #16a34a 14%, transparent);
		border: 1px solid color-mix(in srgb, #16a34a 35%, transparent);
		border-radius: 12px;
		color: #22c55e;
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
	.chip-muted {
		color: var(--sub);
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
	.link:disabled, .danger-link:disabled, .ghost:disabled, .primary:disabled {
		cursor: default;
		opacity: 0.55;
	}
	.empty-state {
		color: var(--sub);
		padding: 32px 16px;
		text-align: center;
	}
	.dialog-backdrop {
		position: fixed;
		inset: 0;
		z-index: 70;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 24px;
		background: var(--overlay, rgba(4, 10, 22, 0.72));
	}
	.dialog {
		width: min(640px, 100%);
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 20px;
		box-shadow: 0 24px 90px rgba(0, 0, 0, 0.34);
	}
	.dialog-head, .dialog-foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		padding: 18px 20px;
	}
	.dialog-head {
		border-bottom: 1px solid var(--border);
	}
	.dialog-title {
		color: var(--heading);
		font-size: 18px;
		font-weight: 700;
	}
	.dialog-subtitle {
		color: var(--sub);
		font-size: 12px;
		margin-top: 4px;
	}
	.dialog-body {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 14px;
		padding: 20px;
	}
	.dialog-body label {
		display: flex;
		flex-direction: column;
		gap: 6px;
		color: var(--sub);
		font-size: 12px;
	}
	.dialog-body label.wide {
		grid-column: 1 / -1;
	}
	.dialog-body input,
	.dialog-body select {
		background: var(--input-bg, var(--panel));
		border: 1px solid var(--border);
		border-radius: 10px;
		color: var(--heading);
		font-size: 13px;
		padding: 10px 12px;
	}
	.dialog-body .toggle {
		justify-content: flex-end;
	}
	.dialog-body .toggle input {
		width: 18px;
		height: 18px;
		padding: 0;
	}
	.dialog-foot {
		border-top: 1px solid var(--border);
		justify-content: flex-end;
	}
	.ghost {
		background: transparent;
		border: 1px solid var(--border);
		color: var(--heading);
		padding: 8px 14px;
		border-radius: 10px;
		cursor: pointer;
	}
	@media (max-width: 720px) {
		.dialog-body {
			grid-template-columns: 1fr;
		}
	}
</style>
