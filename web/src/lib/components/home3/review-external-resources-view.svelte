<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listTerminologyResources,
		approveTerminologyResource,
		disapproveTerminologyResource,
		type TerminologyResource
	} from '$lib/services/terminologyResourceService';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import ClipboardCheckIcon from '@lucide/svelte/icons/clipboard-check';
	import ClipboardListIcon from '@lucide/svelte/icons/clipboard-list';
	import CheckCircle2Icon from '@lucide/svelte/icons/check-circle-2';
	import CircleXIcon from '@lucide/svelte/icons/circle-x';
	import XIcon from '@lucide/svelte/icons/x';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Design tokens (match the External Terminology Resources page) ---
	let cardBg = $derived(darkMode ? '#252A3A' : '#FFFFFF');
	let inputBg = $derived(darkMode ? '#171B26' : '#F9FAFB');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let amber = $derived(darkMode ? '#FBBF24' : '#D97706');
	let amberTint = $derived(darkMode ? 'rgba(251,191,36,0.15)' : 'rgba(217,119,6,0.10)');
	let green = $derived(darkMode ? '#34D399' : '#059669');
	let dangerColor = $derived(darkMode ? '#F87171' : '#DC2626');
	let dangerTint = $derived(darkMode ? 'rgba(248,113,113,0.15)' : 'rgba(220,38,38,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let mono = $derived(darkMode ? '#A5B4FC' : '#4F46E5');

	let resources = $state<TerminologyResource[]>([]);
	let loading = $state(false);
	let pageError = $state<string | null>(null);
	let approving = $state<Record<string, boolean>>({});

	// --- Review dialog state ---
	let reviewTarget = $state<TerminologyResource | null>(null);
	let reviewComments = $state('');
	let reviewBusy = $state(false);
	let dialogError = $state<string | null>(null);
	let closeButton = $state<HTMLButtonElement | null>(null);

	// Only downloaded resources whose draft manifest is still pending review.
	let pending = $derived(
		resources.filter((r) => r.downloaded && r.review_status === 'pending_review')
	);

	onMount(() => {
		refresh();
	});

	$effect(() => {
		if (reviewTarget) {
			requestAnimationFrame(() => closeButton?.focus());
		}
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

	async function approve(r: TerminologyResource) {
		if (
			!confirm(
				`Approve the "${r.name}" draft manifest and start importing it? This records license_review_status=approved with the signed-in user, removes it from this list, and runs the offline import of the local manifest.`
			)
		) {
			return;
		}
		approving = { ...approving, [r.id]: true };
		try {
			const { import: outcome } = await approveTerminologyResource(r.id);
			await refresh();
			if (!outcome.ok) {
				pageError = `"${r.name}" was approved, but the import did not run: ${outcome.error ?? 'unknown error'}`;
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : `Failed to approve ${r.name}`;
		} finally {
			approving = { ...approving, [r.id]: false };
		}
	}

	function openReview(r: TerminologyResource) {
		reviewTarget = r;
		reviewComments = '';
		reviewBusy = false;
		dialogError = null;
	}

	function closeReview() {
		if (reviewBusy) return;
		reviewTarget = null;
		reviewComments = '';
		dialogError = null;
	}

	function handleDialogKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && !reviewBusy) {
			event.preventDefault();
			closeReview();
		}
	}

	async function approveFromDialog() {
		const r = reviewTarget;
		if (!r) return;
		if (
			!confirm(
				`Approve "${r.name}" and start importing it? This marks the resource as reviewed and approved, saves your review comments, and runs the offline import of the local manifest.`
			)
		) {
			return;
		}
		reviewBusy = true;
		dialogError = null;
		try {
			const { import: outcome } = await approveTerminologyResource(r.id, {
				comments: reviewComments.trim()
			});
			reviewTarget = null;
			await refresh();
			if (!outcome.ok) {
				pageError = `"${r.name}" was approved, but the import did not run: ${outcome.error ?? 'unknown error'}`;
			}
		} catch (e) {
			dialogError = e instanceof Error ? e.message : `Failed to approve ${r.name}`;
		} finally {
			reviewBusy = false;
		}
	}

	async function disapproveFromDialog() {
		const r = reviewTarget;
		if (!r) return;
		if (
			!confirm(
				`Disapprove "${r.name}"? Your review comments will be saved and the resource will be marked disapproved; it leaves this list and nothing is imported.`
			)
		) {
			return;
		}
		reviewBusy = true;
		dialogError = null;
		try {
			await disapproveTerminologyResource(r.id, { comments: reviewComments.trim() });
			reviewTarget = null;
			await refresh();
		} catch (e) {
			dialogError = e instanceof Error ? e.message : `Failed to disapprove ${r.name}`;
		} finally {
			reviewBusy = false;
		}
	}
</script>

<div
	class="flex h-full flex-col overflow-y-auto"
	style="background:transparent; scrollbar-width:thin;"
>
	<!-- Header -->
	<div class="flex flex-shrink-0 flex-wrap items-center justify-between gap-3 px-6 py-4">
		<div>
			<h1 class="text-lg font-semibold" style="color:{textPrimary};">Review External Resources</h1>
			<p class="mt-0.5 text-xs" style="color:{textSecondary};">
				Downloaded resources whose draft manifests still await operator license review and approval.
			</p>
		</div>
		<button
			onclick={refresh}
			disabled={loading}
			class="flex cursor-pointer items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-60"
			style="background:{accentTint}; color:{accent}; border:1px solid transparent;"
			aria-label="Refresh pending review statuses"
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

	<!-- Empty state -->
	{#if !loading && pending.length === 0}
		<div class="flex flex-1 items-center justify-center px-6">
			<div
				class="max-w-md rounded-xl border p-8 text-center"
				style="background:{cardBg}; border-color:{borderColor};"
			>
				<ClipboardCheckIcon class="mx-auto h-8 w-8" style="color:{textMuted};" />
				<h2 class="mt-3 text-sm font-semibold" style="color:{textPrimary};">
					No resources awaiting review
				</h2>
				<p class="mt-1.5 text-xs leading-relaxed" style="color:{textSecondary};">
					Downloaded resources with approved drafts (or no downloads yet) do not appear here.
					Download sources from External Terminology Resources and they will show up for review.
				</p>
			</div>
		</div>
	{/if}

	<!-- Pending review cards -->
	<div class="grid flex-1 grid-cols-1 gap-4 px-6 pb-6 md:grid-cols-2 xl:grid-cols-3">
		{#each pending as r (r.id)}
			<div
				class="flex flex-col rounded-xl border p-4"
				style="background:{cardBg}; border-color:{borderColor}; box-shadow:0 1px 3px rgba(0,0,0,0.06);"
			>
				<!-- Name + status -->
				<div class="flex items-start justify-between gap-2">
					<h2 class="text-sm font-semibold" style="color:{textPrimary};">{r.name}</h2>
					<span
						class="flex-shrink-0 rounded-full px-2 py-0.5 text-[10px] font-medium"
						style="background:{amberTint}; color:{amber}; border:1px solid {amber}40;"
					>
						Pending review
					</span>
				</div>

				<p class="mt-1.5 text-xs leading-relaxed" style="color:{textSecondary};">
					{r.description}
				</p>

				<!-- Source URL -->
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
				<div
					class="mt-2 space-y-1 rounded-lg px-2.5 py-2 text-[11px]"
					style="background:{amberTint}; border:1px solid {amber}33;"
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

				{#if r.error}
					<div
						class="mt-2 rounded-lg px-2.5 py-1.5 text-[11px]"
						style="background:{dangerTint}; color:{dangerColor}; border:1px solid {dangerColor}33;"
					>
						{r.error}
					</div>
				{/if}

				<!-- Review steps -->
				<div
					class="mt-2 rounded-lg px-2.5 py-2 text-[11px] leading-relaxed"
					style="background:{inputBg};"
				>
					<div class="font-medium" style="color:{textPrimary};">What review requires</div>
					<ul class="mt-1 list-disc pl-4" style="color:{textSecondary};">
						<li>Confirm license, role, scopes, relations, and checksum in {r.manifest_draft}.</li>
						<li>
							Open <span class="font-medium" style="color:{textPrimary};">Review</span> to inspect the
							resource, add comments, then approve (starts import) or disapprove.
						</li>
					</ul>
				</div>

				<!-- Review + approve actions -->
				<div class="mt-auto space-y-2 pt-3">
					<button
						onclick={() => openReview(r)}
						class="flex w-full cursor-pointer items-center justify-center gap-1.5 rounded-lg px-3 py-2 text-xs font-medium transition-colors duration-150"
						style="background:{accentTint}; color:{accent}; border:1px solid {accent}55;"
					>
						<ClipboardListIcon class="h-3.5 w-3.5" />
						Review
					</button>
					<button
						onclick={() => approve(r)}
						disabled={approving[r.id]}
						class="flex w-full cursor-pointer items-center justify-center gap-1.5 rounded-lg px-3 py-2 text-xs font-medium transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-60"
						style="background:{green}; color:#FFFFFF; border:none;"
					>
						<CheckCircle2Icon class="h-3.5 w-3.5" />
						{approving[r.id] ? 'Approving…' : 'Mark approved'}
					</button>
				</div>

				<!-- Notes -->
				{#if r.notes}
					<p class="mt-2 text-[11px] leading-relaxed italic" style="color:{textMuted};">
						{r.notes}
					</p>
				{/if}
			</div>
		{/each}
	</div>
</div>

<!-- Review dialog -->
{#if reviewTarget}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-6"
		style="background:rgba(10,12,20,0.6);"
		role="presentation"
		onclick={(event) => {
			if (event.target === event.currentTarget) closeReview();
		}}
	>
		<div
			tabindex="-1"
			class="flex max-h-[85vh] w-full max-w-2xl flex-col rounded-xl"
			role="dialog"
			aria-modal="true"
			aria-label="Review {reviewTarget.name}"
			onkeydown={handleDialogKeydown}
			style="background:{cardBg}; border:1px solid {borderColor}; box-shadow:0 20px 50px rgba(0,0,0,0.35);"
		>
			<!-- Dialog header -->
			<div
				class="flex flex-shrink-0 items-center justify-between gap-3 px-5 py-4"
				style="border-bottom:1px solid {borderColor};"
			>
				<div class="flex min-w-0 items-center gap-2">
					<ClipboardListIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
					<h2 class="truncate text-sm font-semibold" style="color:{textPrimary};">
						Review — {reviewTarget.name}
					</h2>
				</div>
				<button
					bind:this={closeButton}
					onclick={closeReview}
					disabled={reviewBusy}
					class="cursor-pointer rounded p-1.5 transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-60"
					style="background:{inputBg}; color:{textMuted}; border:1px solid {borderColor};"
					aria-label="Close review dialog"
				>
					<XIcon class="h-4 w-4" />
				</button>
			</div>

			<!-- Dialog body -->
			<div class="flex-1 space-y-3 overflow-y-auto px-5 py-4" style="scrollbar-width:thin;">
				<p class="text-xs leading-relaxed" style="color:{textSecondary};">
					{reviewTarget.description}
				</p>

				<div class="grid grid-cols-1 gap-2 text-[11px] sm:grid-cols-2">
					<div class="rounded-lg px-2.5 py-2" style="background:{inputBg};">
						<div style="color:{textMuted};">URL</div>
						<!-- eslint-disable svelte/no-navigation-without-resolve -->
						<a
							href={reviewTarget.url}
							target="_blank"
							rel="noopener noreferrer"
							class="mt-0.5 block truncate font-medium underline-offset-2 hover:underline"
							style="color:{accent};"
						>
							{reviewTarget.url}
						</a>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					</div>
					<div class="rounded-lg px-2.5 py-2" style="background:{inputBg};">
						<div style="color:{textMuted};">Release</div>
						<div class="mt-0.5 font-medium" style="color:{textPrimary};">
							{reviewTarget.release || '—'}
						</div>
					</div>
					<div class="rounded-lg px-2.5 py-2" style="background:{inputBg};">
						<div style="color:{textMuted};">License</div>
						<!-- eslint-disable svelte/no-navigation-without-resolve -->
						<a
							href={reviewTarget.license_url}
							target="_blank"
							rel="noopener noreferrer"
							class="mt-0.5 block truncate font-medium underline-offset-2 hover:underline"
							style="color:{textPrimary};"
						>
							{reviewTarget.license}
						</a>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					</div>
					<div class="rounded-lg px-2.5 py-2" style="background:{inputBg};">
						<div style="color:{textMuted};">Review status</div>
						<div class="mt-0.5 font-medium" style="color:{amber};">
							{reviewTarget.review_status || '—'}
						</div>
					</div>
					<div class="rounded-lg px-2.5 py-2" style="background:{inputBg};">
						<div style="color:{textMuted};">Downloaded at</div>
						<div class="mt-0.5 font-medium" style="color:{textPrimary};">
							{formatDate(reviewTarget.downloaded_at)}
						</div>
					</div>
					<div class="rounded-lg px-2.5 py-2" style="background:{inputBg};">
						<div style="color:{textMuted};">Size</div>
						<div class="mt-0.5 font-medium" style="color:{textPrimary};">
							{formatBytes(reviewTarget.size_bytes) || '—'}
						</div>
					</div>
					<div class="rounded-lg px-2.5 py-2 sm:col-span-2" style="background:{inputBg};">
						<div style="color:{textMuted};">SHA-256</div>
						<div class="mt-0.5 font-mono font-medium break-all" style="color:{mono};">
							{reviewTarget.sha256 || '—'}
						</div>
					</div>
					{#if reviewTarget.artifact}
						<div class="rounded-lg px-2.5 py-2" style="background:{inputBg};">
							<div style="color:{textMuted};">Artifact</div>
							<div class="mt-0.5 font-medium break-all" style="color:{textPrimary};">
								{reviewTarget.artifact}
							</div>
						</div>
					{/if}
					{#if reviewTarget.manifest_draft}
						<div class="rounded-lg px-2.5 py-2" style="background:{inputBg};">
							<div style="color:{textMuted};">Draft manifest</div>
							<div class="mt-0.5 font-medium break-all" style="color:{textPrimary};">
								{reviewTarget.manifest_draft}
							</div>
						</div>
					{/if}
				</div>

				<!-- Review comments -->
				<div>
					<label
						for="review-comments"
						class="block text-[11px] font-medium"
						style="color:{textPrimary};"
					>
						Review comments
					</label>
					<textarea
						id="review-comments"
						bind:value={reviewComments}
						rows={5}
						disabled={reviewBusy}
						placeholder="License, scope, role, relations, checksum — anything the operator considered during review."
						class="mt-1 w-full resize-y rounded-lg px-3 py-2 text-xs leading-relaxed transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-60"
						style="background:{inputBg}; color:{textPrimary}; border:1px solid {borderColor}; outline:none;"
					></textarea>
				</div>

				{#if dialogError}
					<div
						class="rounded-lg px-3 py-2 text-xs"
						style="background:{dangerTint}; color:{dangerColor}; border:1px solid {dangerColor}33;"
					>
						{dialogError}
					</div>
				{/if}
			</div>

			<!-- Dialog footer -->
			<div
				class="flex flex-shrink-0 flex-wrap items-center justify-end gap-2 px-5 py-4"
				style="border-top:1px solid {borderColor};"
			>
				<button
					onclick={closeReview}
					disabled={reviewBusy}
					class="cursor-pointer rounded-lg px-3 py-2 text-xs font-medium transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-60"
					style="background:{inputBg}; color:{textSecondary}; border:1px solid {borderColor};"
				>
					Cancel
				</button>
				<button
					onclick={disapproveFromDialog}
					disabled={reviewBusy}
					class="flex cursor-pointer items-center gap-1.5 rounded-lg px-3 py-2 text-xs font-medium transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-60"
					style="background:{dangerColor}; color:#FFFFFF; border:none;"
				>
					<CircleXIcon class="h-3.5 w-3.5" />
					{reviewBusy ? 'Working…' : 'Disapprove'}
				</button>
				<button
					onclick={approveFromDialog}
					disabled={reviewBusy}
					class="flex cursor-pointer items-center gap-1.5 rounded-lg px-3 py-2 text-xs font-medium transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-60"
					style="background:{green}; color:#FFFFFF; border:none;"
				>
					<CheckCircle2Icon class="h-3.5 w-3.5" />
					{reviewBusy ? 'Working…' : 'Approve'}
				</button>
			</div>
		</div>
	</div>
{/if}
