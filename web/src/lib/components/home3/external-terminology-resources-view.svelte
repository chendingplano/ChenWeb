<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listTerminologyResources,
		downloadTerminologyResource,
		type TerminologyResource
	} from '$lib/services/terminologyResourceService';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import LockIcon from '@lucide/svelte/icons/lock';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Design tokens (match the Dashboard app shell) ---
	let cardBg = $derived(darkMode ? '#252A3A' : '#FFFFFF');
	let inputBg = $derived(darkMode ? '#171B26' : '#F9FAFB');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let green = $derived(darkMode ? '#34D399' : '#059669');
	let greenTint = $derived(darkMode ? 'rgba(52,211,153,0.15)' : 'rgba(5,150,105,0.10)');
	let amber = $derived(darkMode ? '#FBBF24' : '#D97706');
	let amberTint = $derived(darkMode ? 'rgba(251,191,36,0.15)' : 'rgba(217,119,6,0.10)');
	let dangerColor = $derived(darkMode ? '#F87171' : '#DC2626');
	let dangerTint = $derived(darkMode ? 'rgba(248,113,113,0.15)' : 'rgba(220,38,38,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let mono = $derived(darkMode ? '#A5B4FC' : '#4F46E5');

	let resources = $state<TerminologyResource[]>([]);
	let loading = $state(false);
	let pageError = $state<string | null>(null);
	let downloading = $state<Record<string, boolean>>({});

	onMount(() => {
		refresh();
	});

	function formatBytes(n: number): string {
		if (!n) return '';
		if (n < 1024) return `${n} B`;
		const units = ['KB', 'MB', 'GB', 'TB'];
		let value = n / 1024;
		let i = 0;
		while (value >= 1024 && i < units.length - 1) {
			value /= 1024;
			i++;
		}
		return `${value.toFixed(1)} ${units[i]}`;
	}

	function formatDate(iso: string | null): string {
		if (!iso) return '';
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
	}

	function shortSha(sha: string): string {
		return sha ? `${sha.slice(0, 16)}…` : '';
	}

	async function refresh() {
		loading = true;
		pageError = null;
		try {
			resources = await listTerminologyResources();
		} catch (e) {
			pageError = e instanceof Error ? e.message : 'Failed to load terminology resources';
		} finally {
			loading = false;
		}
	}

	async function download(r: TerminologyResource) {
		downloading = { ...downloading, [r.id]: true };
		try {
			await downloadTerminologyResource(r.id);
			await refresh();
		} catch (e) {
			pageError = e instanceof Error ? e.message : `Failed to download ${r.name}`;
		} finally {
			downloading = { ...downloading, [r.id]: false };
		}
	}

	function statusPill(r: TerminologyResource): { label: string; color: string; bg: string } {
		if (r.permission_required) return { label: 'Permission required', color: amber, bg: amberTint };
		if (r.downloaded) return { label: 'Downloaded', color: green, bg: greenTint };
		return { label: 'Not downloaded', color: textMuted, bg: 'transparent' };
	}
</script>

<div
	class="flex h-full flex-col overflow-y-auto"
	style="background:transparent; scrollbar-width:thin;"
>
	<!-- Header -->
	<div class="flex flex-shrink-0 flex-wrap items-center justify-between gap-3 px-6 py-4">
		<div>
			<h1 class="text-lg font-semibold" style="color:{textPrimary};">
				External Terminology Resources
			</h1>
			<p class="mt-0.5 text-xs" style="color:{textSecondary};">
				Freely downloadable sources can be fetched automatically; downloads write local artifacts
				plus an unapproved draft manifest awaiting operator license review.
			</p>
		</div>
		<button
			onclick={refresh}
			disabled={loading}
			class="flex cursor-pointer items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-60"
			style="background:{accentTint}; color:{accent}; border:1px solid transparent;"
			aria-label="Refresh resource statuses"
		>
			<RefreshCwIcon class="h-3.5 w-3.5" />
			{loading ? 'Loading…' : 'Refresh'}
		</button>
	</div>

	{#if pageError}
		<div
			class="mx-6 mb-3 rounded-lg px-3 py-2 text-xs"
			style="background:{dangerTint}; color:{dangerColor}; border:1px solid {dangerColor}33;"
		>
			{pageError}
		</div>
	{/if}

	<!-- Resource cards -->
	<div class="grid flex-1 grid-cols-1 gap-4 px-6 pb-6 md:grid-cols-2 xl:grid-cols-3">
		{#each resources as r (r.id)}
			{@const pill = statusPill(r)}
			<div
				class="flex flex-col rounded-xl border p-4"
				style="background:{cardBg}; border-color:{borderColor}; box-shadow:0 1px 3px rgba(0,0,0,0.06);"
			>
				<!-- Name + status -->
				<div class="flex items-start justify-between gap-2">
					<h2 class="text-sm font-semibold" style="color:{textPrimary};">{r.name}</h2>
					<span
						class="flex-shrink-0 rounded-full px-2 py-0.5 text-[10px] font-medium"
						style="background:{pill.bg}; color:{pill.color}; border:1px solid {pill.color}40;"
					>
						{pill.label}
					</span>
				</div>

				<p class="mt-1.5 text-xs leading-relaxed" style="color:{textSecondary};">
					{r.description}
				</p>

				<!-- URL -->
				<!-- eslint-disable svelte/no-navigation-without-resolve -->
				<a
					href={r.url}
					target="_blank"
					rel="noopener noreferrer"
					class="mt-2 flex items-center gap-1 truncate text-xs underline-offset-2 hover:underline"
					style="color:{accent};"
				>
					<ExternalLinkIcon class="h-3 w-3 flex-shrink-0" />
					<span class="truncate">{r.url}</span>
				</a>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->

				<!-- Release + license -->
				<div class="mt-3 grid grid-cols-2 gap-2 text-[11px]">
					<div class="rounded-lg px-2 py-1.5" style="background:{inputBg};">
						<div style="color:{textMuted};">Release</div>
						<div class="mt-0.5 font-medium" style="color:{textPrimary};">{r.release || '—'}</div>
					</div>
					<div class="rounded-lg px-2 py-1.5" style="background:{inputBg};">
						<div style="color:{textMuted};">License</div>
						<!-- eslint-disable svelte/no-navigation-without-resolve -->
						<a
							href={r.license_url}
							target="_blank"
							rel="noopener noreferrer"
							class="mt-0.5 block truncate font-medium underline-offset-2 hover:underline"
							style="color:{textPrimary};"
						>
							{r.license}
						</a>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					</div>
				</div>

				<!-- Downloaded metadata -->
				{#if r.downloaded}
					<div
						class="mt-2 space-y-1 rounded-lg px-2.5 py-2 text-[11px]"
						style="background:{greenTint}; border:1px solid {green}33;"
					>
						<div class="flex justify-between gap-2">
							<span style="color:{textMuted};">Downloaded at</span>
							<span style="color:{textPrimary};">{formatDate(r.downloaded_at)}</span>
						</div>
						<div class="flex justify-between gap-2">
							<span style="color:{textMuted};">Size</span>
							<span style="color:{textPrimary};">{formatBytes(r.size_bytes)}</span>
						</div>
						<div class="flex items-center justify-between gap-2">
							<span style="color:{textMuted};">SHA-256</span>
							<span class="font-mono" style="color:{mono};">{shortSha(r.sha256)}</span>
						</div>
						{#if r.artifact}
							<div class="flex justify-between gap-2">
								<span style="color:{textMuted};">Artifact</span>
								<span class="truncate" style="color:{textPrimary};">{r.artifact}</span>
							</div>
						{/if}
						{#if r.manifest_draft}
							<div class="flex justify-between gap-2">
								<span style="color:{textMuted};">Draft manifest</span>
								<span style="color:{textPrimary};">{r.manifest_draft}</span>
							</div>
						{/if}
					</div>
				{/if}

				{#if r.error}
					<div
						class="mt-2 rounded-lg px-2.5 py-1.5 text-[11px]"
						style="background:{dangerTint}; color:{dangerColor}; border:1px solid {dangerColor}33;"
					>
						{r.error}
					</div>
				{/if}

				<!-- Notes -->
				{#if r.notes}
					<p class="mt-2 text-[11px] leading-relaxed italic" style="color:{textMuted};">
						{r.notes}
					</p>
				{/if}

				<!-- Action -->
				<div class="mt-auto pt-3">
					{#if r.can_download}
						<button
							onclick={() => download(r)}
							disabled={downloading[r.id]}
							class="flex w-full cursor-pointer items-center justify-center gap-1.5 rounded-lg px-3 py-2 text-xs font-medium transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-60"
							style="background:{accent}; color:#FFFFFF; border:none;"
						>
							<DownloadIcon class="h-3.5 w-3.5" />
							{downloading[r.id] ? 'Downloading…' : r.downloaded ? 'Re-download' : 'Download'}
						</button>
					{:else}
						<button
							disabled
							class="flex w-full cursor-not-allowed items-center justify-center gap-1.5 rounded-lg px-3 py-2 text-xs font-medium opacity-60"
							style="background:{inputBg}; color:{textMuted}; border:1px solid {borderColor};"
							title="This resource is copyright-gated and requires an IEC license."
						>
							<LockIcon class="h-3.5 w-3.5" />
							Requires license
						</button>
					{/if}
				</div>
			</div>
		{/each}
	</div>
</div>
