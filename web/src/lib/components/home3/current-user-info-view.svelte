<script lang="ts">
	import { onMount } from 'svelte';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	type UserProfile = { name: string; email: string };
	let user = $state<UserProfile | null>(null);
	let loading = $state(true);
	let error = $state('');

	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');

	onMount(() => {
		void loadUserInfo();
	});

	async function loadUserInfo() {
		loading = true;
		error = '';

		try {
			const response = await fetch('/api/v1/ai-assistant/user-info', {
				credentials: 'same-origin'
			});
			if (!response.ok) throw new Error('Could not load your user information.');

			const payload: unknown = await response.json();
			if (
				typeof payload !== 'object' ||
				payload === null ||
				!('user' in payload) ||
				typeof payload.user !== 'object' ||
				payload.user === null ||
				!('name' in payload.user) ||
				!('email' in payload.user) ||
				typeof payload.user.name !== 'string' ||
				typeof payload.user.email !== 'string'
			) {
				throw new Error('The user information response was invalid.');
			}

			user = {
				name: payload.user.name.trim(),
				email: payload.user.email.trim()
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
		<h1 id="current-user-info-title" style="color:{textPrimary}; font-size:20px; font-weight:600;">
			User Info
		</h1>

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
			<dl class="mt-5 grid gap-4 sm:grid-cols-2">
				<div class="rounded-lg p-4" style="border:1px solid {borderColor};">
					<dt style="color:{textSecondary}; font-size:12px;">Name</dt>
					<dd class="mt-1" style="color:{textPrimary}; font-size:15px;">{user.name || '—'}</dd>
				</div>
				<div class="rounded-lg p-4" style="border:1px solid {borderColor};">
					<dt style="color:{textSecondary}; font-size:12px;">Email</dt>
					<dd class="mt-1" style="color:{textPrimary}; font-size:15px;">{user.email || '—'}</dd>
				</div>
			</dl>
		{/if}
	</section>
</div>
