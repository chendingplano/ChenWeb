<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listTerminologyResources,
		downloadTerminologyResource,
		setAutoPromoteEnabled,
		type TerminologyResource
	} from '$lib/services/terminologyResourceService';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import LockIcon from '@lucide/svelte/icons/lock';
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';

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
	let progress = $state<Record<string, { done: number; total: number; startedAt: number }>>({});
	let togglingPromotion = $state<Record<string, boolean>>({});

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

	async function toggleAutoPromote(r: TerminologyResource) {
		const next = !r.auto_promote_enabled;
		togglingPromotion = { ...togglingPromotion, [r.id]: true };
		try {
			const enabled = await setAutoPromoteEnabled(r.id, next);
			resources = resources.map((x) =>
				x.id === r.id ? { ...x, auto_promote_enabled: enabled } : x
			);
		} catch (e) {
			pageError = e instanceof Error ? e.message : `Failed to update auto-promotion for ${r.name}`;
		} finally {
			togglingPromotion = { ...togglingPromotion, [r.id]: false };
		}
	}

	function expectedSizeLabel(r: TerminologyResource): string {
		if (r.expected_size_bytes > 0) return `≈ ${formatBytes(r.expected_size_bytes)}`;
		if (r.max_bytes > 0) return `Varies (≤ ${formatBytes(r.max_bytes)})`;
		return '—';
	}

	function cadenceLabel(r: TerminologyResource): string {
		if (!r.update_cadence) return 'Pinned';
		return r.update_cadence.charAt(0).toUpperCase() + r.update_cadence.slice(1);
	}

	function formatDuration(ms: number): string {
		const s = Math.max(0, Math.round(ms / 1000));
		return `${s}s`;
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
		const startedAt = Date.now();
		progress = { ...progress, [r.id]: { done: 0, total: 0, startedAt } };
		// The POST blocks until the server finishes streaming, so poll the
		// status endpoint for server-side progress while it runs.
		const timer = setInterval(async () => {
			try {
				const list = await listTerminologyResources();
				const live = list.find((x) => x.id === r.id);
				if (!live) return;
				if (live.downloading) {
					progress = {
						...progress,
						[r.id]: { done: live.downloaded_bytes, total: live.total_bytes, startedAt }
					};
				} else if (live.downloaded || live.error) {
					clearInterval(timer);
				}
			} catch {
				// Transient poll failure; the next tick retries.
			}
		}, 1000);
		try {
			await downloadTerminologyResource(r.id);
		} catch (e) {
			pageError = e instanceof Error ? e.message : `Failed to download ${r.name}`;
		} finally {
			clearInterval(timer);
			downloading = { ...downloading, [r.id]: false };
			const rest = { ...progress };
			delete rest[r.id];
			progress = rest;
			await refresh();
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

				<!-- Expected size + update cadence -->
				<div class="mt-2 grid grid-cols-2 gap-2 text-[11px]">
					<div class="rounded-lg px-2 py-1.5" style="background:{inputBg};">
						<div style="color:{textMuted};">Expected size</div>
						<div class="mt-0.5 font-medium" style="color:{textPrimary};">
							{expectedSizeLabel(r)}
						</div>
					</div>
					<div class="rounded-lg px-2 py-1.5" style="background:{inputBg};">
						<div style="color:{textMuted};">Update cadence</div>
						<div class="mt-0.5 font-medium" style="color:{textPrimary};">{cadenceLabel(r)}</div>
					</div>
				</div>

				<!-- Auto-promote toggle -->
				<div
					class="mt-2 flex items-center justify-between gap-2 rounded-lg px-2.5 py-2 text-[11px]"
					style="background:{inputBg};"
				>
					<div>
						<div class="font-medium" style="color:{textPrimary};">Auto-promote on approve</div>
						<div class="mt-0.5" style="color:{textMuted};">
							Staged entries become keyword concepts automatically, flagged for optional review.
						</div>
					</div>
					<button
						onclick={() => toggleAutoPromote(r)}
						disabled={togglingPromotion[r.id]}
						role="switch"
						aria-checked={r.auto_promote_enabled}
						aria-label="Toggle auto-promote for {r.name}"
						class="relative h-5 w-9 flex-shrink-0 cursor-pointer rounded-full transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-60"
						style="background:{r.auto_promote_enabled ? accent : borderColor};"
					>
						<span
							class="absolute top-0.5 h-4 w-4 rounded-full bg-white transition-transform duration-150"
							style="transform: translateX({r.auto_promote_enabled ? '18px' : '2px'});"
						></span>
					</button>
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

				<!-- Download progress -->
				{#if progress[r.id]}
					{@const p = progress[r.id]}
					{@const pct = p.total > 0 ? Math.min(100, Math.round((p.done / p.total) * 100)) : 0}
					<div
						class="mt-2 space-y-1.5 rounded-lg px-2.5 py-2 text-[11px]"
						style="background:{accentTint}; border:1px solid {accent}40;"
					>
						<div class="flex items-center justify-between gap-2">
							<span class="flex items-center gap-1.5 font-medium" style="color:{accent};">
								<LoaderCircleIcon class="h-3.5 w-3.5" style="animation:spin 1s linear infinite;" />
								Downloading…
							</span>
							<span style="color:{textSecondary};">
								{p.total > 0
									? `${formatBytes(p.done)} / ${formatBytes(p.total)} · ${pct}%`
									: `${formatBytes(p.done) || '0 B'} so far`}
								· {formatDuration(Date.now() - p.startedAt)}
							</span>
						</div>
						<div
							class="relative h-1.5 w-full overflow-hidden rounded-full"
							style="background:{inputBg};"
						>
							{#if p.total > 0}
								<div
									class="h-full rounded-full"
									style="width:{pct}%; background:{accent}; transition:width 0.4s ease;"
								></div>
							{:else}
								<div
									class="indeterminate absolute top-0 h-full rounded-full"
									style="background:{accent};"
								></div>
							{/if}
						</div>
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
							{#if downloading[r.id]}
								<LoaderCircleIcon class="h-3.5 w-3.5" style="animation:spin 1s linear infinite;" />
							{:else}
								<DownloadIcon class="h-3.5 w-3.5" />
							{/if}
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

<style>
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	.indeterminate {
		width: 35%;
		animation: indeterminate 1.2s ease-in-out infinite;
	}
	@keyframes indeterminate {
		0% {
			left: -35%;
		}
		100% {
			left: 100%;
		}
	}
</style>
