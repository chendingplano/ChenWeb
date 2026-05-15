<script lang="ts">
	import { onMount } from 'svelte';
	import DatabaseIcon from '@lucide/svelte/icons/database';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import WifiIcon from '@lucide/svelte/icons/wifi';
	import WifiOffIcon from '@lucide/svelte/icons/wifi-off';

	type SessionResponse = {
		status: boolean;
		launch_url: string;
		proxy_base_path: string;
		callback_url?: string;
		display_name: string;
		user_id: string;
		sso_mode: string;
		capabilities: string[];
		auth_boundary_note?: string;
		message?: string;
	};

	let { darkMode = true }: { darkMode: boolean } = $props();

	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#2AA198' : '#0F766E');
	let accentTint = $derived(darkMode ? 'rgba(42,161,152,0.15)' : 'rgba(15,118,110,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let danger = $derived(darkMode ? '#F87171' : '#DC2626');
	let dangerTint = $derived(darkMode ? 'rgba(248,113,113,0.12)' : 'rgba(220,38,38,0.08)');
	let success = $derived(darkMode ? '#34D399' : '#059669');

	let loading = $state(true);
	let loadError = $state('');
	let session = $state<SessionResponse | null>(null);
	let syncContext = $state(false);
	let iframeNonce = $state(0);

	let iframeSrc = $derived.by(() => {
		if (!session?.launch_url) return '';
		const url = new URL(session.launch_url, window.location.origin);
		url.searchParams.set('embed', '1');
		url.searchParams.set('shell', 'chenweb');
		url.searchParams.set('reload', String(iframeNonce));
		return url.toString();
	});

	onMount(async () => {
		await loadSession();
	});

	async function loadSession() {
		loading = true;
		loadError = '';
		try {
			const res = await fetch('/api/v1/integrations/openmetadata/session', {
				credentials: 'same-origin'
			});
			if (!res.ok) {
				const message = await readErrorMessage(res);
				throw new Error(message);
			}
			session = (await res.json()) as SessionResponse;
		} catch (err) {
			loadError = err instanceof Error ? err.message : 'Failed to start OpenMetadata';
			session = null;
		} finally {
			loading = false;
		}
	}

	async function readErrorMessage(res: Response) {
		try {
			const body = (await res.json()) as { message?: string; error_msg?: string };
			return body.message || body.error_msg || `Request failed with status ${res.status}`;
		} catch {
			return `Request failed with status ${res.status}`;
		}
	}

	function reloadWorkspace() {
		iframeNonce += 1;
	}

	function openInNewTab() {
		if (!session?.launch_url) return;
		window.open(session.launch_url, '_blank', 'noopener');
	}
</script>

<section class="p-6">
	<div
		class="rounded-xl"
		style="background:{cardBg}; border:1px solid {borderColor}; overflow:hidden;"
	>
		<div
			class="flex flex-wrap items-center justify-between gap-4 px-5 py-4"
			style="background:linear-gradient(135deg, {accentTint}, transparent 55%), {cardBg}; border-bottom:1px solid {borderColor};"
		>
			<div class="flex min-w-0 items-center gap-3">
				<div
					class="flex h-11 w-11 items-center justify-center rounded-xl"
					style="background:{accentTint}; color:{accent}; border:1px solid {accent}33;"
				>
					<DatabaseIcon class="h-5 w-5" />
				</div>
				<div class="min-w-0">
					<div class="flex items-center gap-2">
						<h2 style="font-size:17px; font-weight:600; color:{textPrimary};">
							OpenMetadata Workspace
						</h2>
						<span
							class="inline-flex items-center gap-1 rounded-full px-2 py-0.5"
							style="background:{surface2}; color:{textMuted}; font-size:11px; border:1px solid {borderColor};"
						>
							{#if loadError}
								<WifiOffIcon class="h-3 w-3" />
								Disconnected
							{:else}
								<WifiIcon class="h-3 w-3" style="color:{success};" />
								Session ready
							{/if}
						</span>
					</div>
					<p style="font-size:13px; color:{textSecondary}; margin-top:4px;">
						ChenWeb owns the shell and session bootstrap. OpenMetadata stays embedded in this panel.
					</p>
				</div>
			</div>

			<div class="flex flex-wrap items-center gap-2">
				<label
					class="inline-flex items-center gap-2 rounded-lg px-3 py-2"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary}; font-size:12px;"
				>
					<input bind:checked={syncContext} type="checkbox" />
					Sync context
				</label>
				<button
					class="inline-flex items-center gap-2 rounded-lg px-3 py-2"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:12px; cursor:pointer;"
					onclick={reloadWorkspace}
					disabled={loading || !session}
				>
					<RefreshCwIcon class="h-3.5 w-3.5" />
					Reload
				</button>
				<button
					class="inline-flex items-center gap-2 rounded-lg px-3 py-2"
					style="background:{accent}; border:1px solid {accent}; color:white; font-size:12px; cursor:pointer;"
					onclick={openInNewTab}
					disabled={loading || !session}
				>
					<ExternalLinkIcon class="h-3.5 w-3.5" />
					Open in new tab
				</button>
			</div>
		</div>

		{#if loading}
			<div class="px-5 py-12" style="background:{pageBg}; color:{textSecondary};">
				<div class="mx-auto max-w-xl rounded-xl px-5 py-6" style="background:{surface2}; border:1px solid {borderColor};">
					<div style="font-size:14px; font-weight:600; color:{textPrimary};">Starting OpenMetadata</div>
					<p style="font-size:13px; line-height:1.6; margin-top:6px;">
						ChenWeb is requesting a session and preparing the embedded workspace.
					</p>
				</div>
			</div>
		{:else if loadError}
			<div class="px-5 py-12" style="background:{pageBg}; color:{textSecondary};">
				<div class="mx-auto max-w-xl rounded-xl px-5 py-6" style="background:{dangerTint}; border:1px solid {danger}44;">
					<div style="font-size:14px; font-weight:600; color:{danger};">OpenMetadata is unavailable</div>
					<p style="font-size:13px; line-height:1.6; margin-top:6px; color:{textPrimary};">
						{loadError}
					</p>
					<div class="mt-4 flex items-center gap-2">
						<button
							class="inline-flex items-center gap-2 rounded-lg px-3 py-2"
							style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; font-size:12px; cursor:pointer;"
							onclick={loadSession}
						>
							<RefreshCwIcon class="h-3.5 w-3.5" />
							Try again
						</button>
					</div>
				</div>
			</div>
		{:else if session}
			<div style="background:{pageBg};">
				<div
					class="flex items-center justify-between gap-3 px-5 py-3"
					style="border-bottom:1px solid {borderColor}; color:{textMuted}; font-size:12px;"
				>
					<div class="flex min-w-0 items-center gap-2">
						<span style="color:{textSecondary};">Launch path</span>
						<code style="color:{textPrimary};">{session.proxy_base_path}</code>
					</div>
					{#if session.callback_url}
						<div class="flex min-w-0 items-center gap-2">
							<span style="color:{textSecondary};">Callback URL</span>
							<code style="color:{textPrimary};">{session.callback_url}</code>
						</div>
					{/if}
					<div class="flex items-center gap-2">
						<span style="color:{textSecondary};">SSO mode</span>
						<code style="color:{textPrimary};">{session.sso_mode}</code>
					</div>
					<div class="flex items-center gap-2">
						<span style="color:{textSecondary};">Capabilities</span>
						<code style="color:{textPrimary};">{session.capabilities.join(', ')}</code>
					</div>
				</div>
				{#if session.auth_boundary_note}
					<div
						class="px-5 py-3"
						style="border-bottom:1px solid {borderColor}; background:{surface2}; color:{textSecondary}; font-size:12px; line-height:1.55;"
					>
						{session.auth_boundary_note}
					</div>
				{/if}
				<iframe
					title="OpenMetadata workspace"
					src={iframeSrc}
					class="block w-full"
					style="height:calc(100vh - 370px); min-height:720px; border:0; background:white;"
				></iframe>
			</div>
		{/if}
	</div>
</section>
