<script lang="ts">
	import { onMount } from 'svelte';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	type UserProfile = {
		firstName: string;
		lastName: string;
		email: string;
		phone: string;
		roles: string[];
		createdAt: string;
		lastLogin: string;
	};
	let user = $state<UserProfile | null>(null);
	let loading = $state(true);
	let error = $state('');
	let editing = $state(false);
	let saving = $state(false);
	let saveError = $state('');
	let saveSuccess = $state('');
	let draft = $state({ firstName: '', lastName: '', phone: '' });

	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');

	onMount(() => {
		void loadUserInfo();
	});

	function isRecord(value: unknown): value is Record<string, unknown> {
		return typeof value === 'object' && value !== null && !Array.isArray(value);
	}

	function formatDateTime(value: string): string {
		if (!value) return '—';
		const date = new Date(value);
		return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString();
	}

	function beginEdit() {
		if (!user) return;
		draft = { firstName: user.firstName, lastName: user.lastName, phone: user.phone };
		saveError = '';
		saveSuccess = '';
		editing = true;
	}

	function cancelEdit() {
		if (user) {
			draft = { firstName: user.firstName, lastName: user.lastName, phone: user.phone };
		}
		saveError = '';
		editing = false;
	}

	async function saveProfile() {
		const firstName = draft.firstName.trim();
		const lastName = draft.lastName.trim();
		const phone = draft.phone.trim();
		if (!firstName || !lastName) {
			saveError = 'First and last name are required.';
			return;
		}

		saving = true;
		saveError = '';
		saveSuccess = '';
		try {
			const response = await fetch('/api/v1/ai-assistant/user-info', {
				method: 'PUT',
				credentials: 'same-origin',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({
					first_name: firstName,
					last_name: lastName,
					phone_number: phone
				})
			});
			const payload: unknown = await response.json().catch(() => null);
			if (!response.ok) {
				const message = isRecord(payload) && typeof payload.error === 'string'
					? payload.error
					: 'Failed to update your profile.';
				throw new Error(message);
			}

			if (user) user = { ...user, firstName, lastName, phone };
			editing = false;
			saveSuccess = 'Profile updated.';
		} catch (cause) {
			saveError = cause instanceof Error ? cause.message : 'Failed to update your profile.';
		} finally {
			saving = false;
		}
	}

	async function loadUserInfo() {
		loading = true;
		error = '';

		try {
			const response = await fetch('/auth/me', {
				credentials: 'same-origin'
			});
			if (!response.ok) throw new Error('Could not load your user information.');

			const payload: unknown = await response.json();
			if (!isRecord(payload) || !isRecord(payload.session)) {
				throw new Error('The user information response was invalid.');
			}
			const session = payload.session;
			if (!isRecord(session.identity)) {
				throw new Error('The user information response was invalid.');
			}
			const identity = session.identity;
			if (!isRecord(identity.traits) || !isRecord(identity.metadata_public)) {
				throw new Error('The user information response was invalid.');
			}
			const traits = identity.traits;
			const metadata = identity.metadata_public;
			const name = isRecord(traits.name) ? traits.name : {};
			if (typeof traits.email !== 'string') {
				throw new Error('The user information response was invalid.');
			}

			user = {
				firstName: typeof name.first === 'string' ? name.first.trim() : '',
				lastName: typeof name.last === 'string' ? name.last.trim() : '',
				email: traits.email.trim(),
				phone: typeof traits.phone === 'string' ? traits.phone.trim() : '',
				roles: Array.isArray(metadata.roles)
					? metadata.roles.filter((role): role is string => typeof role === 'string')
					: [],
				createdAt: typeof identity.created_at === 'string' ? identity.created_at : '',
				lastLogin: typeof session.authenticated_at === 'string' ? session.authenticated_at : ''
			};
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load your user information.';
		} finally {
			loading = false;
		}
	}
</script>

<div class="p-6">
	<section
		class="rounded-xl p-6"
		style="background:{cardBg}; border:1px solid {borderColor};"
		aria-labelledby="current-user-info-title"
	>
		<div class="flex items-center justify-between gap-4">
			<h1 id="current-user-info-title" style="color:{textPrimary}; font-size:20px; font-weight:600;">
				User Info
			</h1>
			{#if user && !editing}
				<button
					type="button"
					class="cursor-pointer rounded-lg px-4 py-2"
					style="background:{accent}; color:white; border:none; font-size:13px; font-weight:600;"
					onclick={beginEdit}>Edit</button
				>
			{/if}
		</div>

		{#if loading}
			<p class="mt-4" style="color:{textSecondary};" aria-live="polite">Loading user information…</p>
		{:else if error}
			<div class="mt-4" role="alert" style="color:{textSecondary};">
				<p>{error}</p>
				<button
					class="mt-3 cursor-pointer rounded-md px-3 py-1.5"
					style="background:{accent}; color:white; border:none;"
					onclick={loadUserInfo}>Retry</button
				>
			</div>
		{:else if user}
			<dl class="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
				<div class="rounded-lg p-4" style="border:1px solid {borderColor};">
					<dt style="color:{textSecondary}; font-size:12px;">First Name</dt>
					<dd class="mt-1" style="color:{textPrimary}; font-size:15px;">
						{#if editing}
							<input
								class="w-full rounded-md px-3 py-2"
								style="background:{darkMode ? '#171B26' : '#FFFFFF'}; color:{textPrimary}; border:1px solid {borderColor};"
								bind:value={draft.firstName}
								aria-label="First Name"
								required
								maxlength="100"
							/>
						{:else}{user.firstName || '—'}{/if}
					</dd>
				</div>
				<div class="rounded-lg p-4" style="border:1px solid {borderColor};">
					<dt style="color:{textSecondary}; font-size:12px;">Last Name</dt>
					<dd class="mt-1" style="color:{textPrimary}; font-size:15px;">
						{#if editing}
							<input
								class="w-full rounded-md px-3 py-2"
								style="background:{darkMode ? '#171B26' : '#FFFFFF'}; color:{textPrimary}; border:1px solid {borderColor};"
								bind:value={draft.lastName}
								aria-label="Last Name"
								required
								maxlength="100"
							/>
						{:else}{user.lastName || '—'}{/if}
					</dd>
				</div>
				<div class="rounded-lg p-4" style="border:1px solid {borderColor};">
					<dt style="color:{textSecondary}; font-size:12px;">Email</dt>
					<dd class="mt-1" style="color:{textPrimary}; font-size:15px;">{user.email || '—'}</dd>
				</div>
				<div class="rounded-lg p-4" style="border:1px solid {borderColor};">
					<dt style="color:{textSecondary}; font-size:12px;">Phone Number</dt>
					<dd class="mt-1" style="color:{textPrimary}; font-size:15px;">
						{#if editing}
							<input
								class="w-full rounded-md px-3 py-2"
								style="background:{darkMode ? '#171B26' : '#FFFFFF'}; color:{textPrimary}; border:1px solid {borderColor};"
								bind:value={draft.phone}
								aria-label="Phone Number"
								type="tel"
								placeholder="+8613812345678"
								maxlength="14"
							/>
						{:else}{user.phone || '—'}{/if}
					</dd>
				</div>
				<div class="rounded-lg p-4" style="border:1px solid {borderColor};">
					<dt style="color:{textSecondary}; font-size:12px;">Roles</dt>
					<dd class="mt-2 flex flex-wrap gap-2">
						{#if user.roles.length > 0}
							{#each user.roles as role (role)}
								<span
									class="rounded-full px-2.5 py-1"
									style="background:{darkMode ? '#2D3348' : '#ECEEF2'}; color:{textPrimary}; font-size:12px;"
									>{role}</span
								>
							{/each}
						{:else}
							<span style="color:{textPrimary}; font-size:15px;">—</span>
						{/if}
					</dd>
				</div>
				<div class="rounded-lg p-4" style="border:1px solid {borderColor};">
					<dt style="color:{textSecondary}; font-size:12px;">Create Time</dt>
					<dd class="mt-1" style="color:{textPrimary}; font-size:15px;">{formatDateTime(user.createdAt)}</dd>
				</div>
				<div class="rounded-lg p-4" style="border:1px solid {borderColor};">
					<dt style="color:{textSecondary}; font-size:12px;">Last Login Time</dt>
					<dd class="mt-1" style="color:{textPrimary}; font-size:15px;">{formatDateTime(user.lastLogin)}</dd>
				</div>
			</dl>
			{#if editing}
				<div class="mt-5 flex justify-end gap-3">
					<button
						type="button"
						class="cursor-pointer rounded-lg px-4 py-2"
						style="background:transparent; color:{textPrimary}; border:1px solid {borderColor};"
						disabled={saving}
						onclick={cancelEdit}>Cancel</button
					>
					<button
						type="button"
						class="cursor-pointer rounded-lg px-4 py-2"
						style="background:{accent}; color:white; border:none; font-weight:600;"
						disabled={saving}
						onclick={saveProfile}>{saving ? 'Saving…' : 'Save'}</button
					>
				</div>
				{#if saveError}
					<p class="mt-3 text-right" role="alert" style="color:#F87171;">{saveError}</p>
				{/if}
			{:else if saveSuccess}
				<p class="mt-3 text-right" role="status" style="color:{textSecondary};">{saveSuccess}</p>
			{/if}
		{/if}
	</section>
</div>
