<script lang="ts">
	import { onMount } from 'svelte';
	import { listManagedUsers, type ManagedUser } from '$lib/services/userManagementService';

	type UserRow = {
		id: string;
		name: string;
		email: string;
		status: 'active' | 'inactive' | 'trial';
		admin: boolean;
		roles: string[];
	};

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let users = $state<UserRow[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

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
		if (user.roles?.includes('trial')) return 'trial';
		if (user.user_status && user.user_status !== 'active') return 'inactive';
		return 'active';
	}

	function mapUser(user: ManagedUser): UserRow {
		const roles = normalizeRoles((user.roles ?? []).join(','), Boolean(user.admin));
		return {
			id: user.id,
			name: displayName(user),
			email: user.email,
			status: mapStatus(user),
			admin: Boolean(user.admin),
			roles
		};
	}

	async function loadUsers() {
		loading = true;
		error = null;
		try {
			const managedUsers = await listManagedUsers();
			users = managedUsers.map(mapUser).sort((a, b) => a.name.localeCompare(b.name));
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load users.';
		} finally {
			loading = false;
		}
	}

	function notImplemented(message: string) {
		error = message;
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
								<button class="link" onclick={() => notImplemented('Edit user is not wired to Kratos yet.')}>Edit</button>
								<button class="danger-link" onclick={() => notImplemented('Delete user is not wired to Kratos yet.')}>Delete</button>
							</div>
						</td>
					</tr>
				{/each}
				{/if}
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
	.empty-state {
		color: var(--sub);
		padding: 32px 16px;
		text-align: center;
	}
</style>
