<script lang="ts">
	import { onMount } from 'svelte';
	import { listReviewRuns, listTiers } from '$lib/services/docReviewService';
	import type { ReviewRunListItem, TierInfo } from '$lib/services/docReviewService';
	import DocReviewSearchDialog from './doc-review-search-dialog.svelte';
	import SearchIcon from '@lucide/svelte/icons/search';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import XIcon from '@lucide/svelte/icons/x';

	let {
		darkMode = true,
		onView,
		refreshKey = 0,
		// Lifted to the parent so the list + selection survive while the results
		// "View" window is open (which unmounts this component) and are restored
		// on return instead of resetting to the default full list.
		requests = $bindable([]),
		showingSelection = $bindable(false)
	}: {
		darkMode?: boolean;
		onView?: (run: ReviewRunListItem) => void;
		// Bump this from the parent (e.g. after a submit) to force a reload.
		refreshKey?: number;
		requests?: ReviewRunListItem[];
		showingSelection?: boolean;
	} = $props();

	// Design tokens (match document-review-view).
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let colorSuccess = $derived(darkMode ? '#34D399' : '#10B981');
	let colorError = $derived(darkMode ? '#F87171' : '#EF4444');

	let tiers = $state<TierInfo[]>([]);
	let tierLabels = $derived.by(() => {
		const m: Record<string, string> = {};
		for (const t of tiers) m[t.key] = t.label;
		return m;
	});
	let loading = $state(false);
	let loadError = $state('');
	let searchOpen = $state(false);

	async function load() {
		loading = true;
		loadError = '';
		try {
			requests = await listReviewRuns();
			showingSelection = false;
		} catch (e: any) {
			loadError = e?.message || 'Failed to load review runs';
		} finally {
			loading = false;
		}
	}

	// The Search dialog returns the requests the user checked + confirmed.
	function applySelection(records: ReviewRunListItem[]) {
		requests = records;
		showingSelection = true;
	}

	function statusColor(status: string): string {
		if (status === 'completed') return colorSuccess;
		if (status === 'failed' || status === 'stopped') return colorError;
		if (status === 'running' || status === 'accepted') return accent;
		return textMuted;
	}

	function tierLabel(key: string): string {
		return tierLabels[key] || key;
	}

	function fmtClock(s?: string): string {
		if (!s) return '—';
		return s.slice(0, 19).replace('T', ' ');
	}

	// Reload whenever the parent bumps refreshKey (e.g. after a new submit). Seed
	// with the initial key so onMount owns the first load (no redundant fetch).
	let lastRefreshKey = $state(refreshKey);
	$effect(() => {
		if (refreshKey !== lastRefreshKey) {
			lastRefreshKey = refreshKey;
			void load();
		}
	});

	onMount(async () => {
		try {
			tiers = await listTiers();
		} catch {
			/* tier labels fall back to keys */
		}
		// On a fresh mount with no preserved selection, load the full list. If the
		// parent handed back a selection (returning from the View window), keep it.
		if (!showingSelection) await load();
	});
</script>

<div style="background:{cardBg}; border:1px solid {borderColor}; border-radius:12px; padding:16px 18px;">
	<div style="display:flex; align-items:center; justify-content:space-between; gap:12px; margin-bottom:12px; flex-wrap:wrap;">
		<div style="font-size:15px; font-weight:600; color:{textPrimary};">
			Review Runs
			<span style="color:{textMuted}; font-weight:400; font-size:13px;">({requests.length})</span>
		</div>
		<div style="display:flex; align-items:center; gap:8px;">
			{#if showingSelection}
				<button
					onclick={() => load()}
					style="display:flex; align-items:center; gap:4px; padding:5px 10px; background:{accentTint}; border:1px solid {accent}30; border-radius:8px; color:{accent}; font-size:12px; cursor:pointer;"
				>
					<XIcon class="h-3 w-3" />
					Show all
				</button>
			{/if}
			<button
				onclick={() => load()}
				disabled={loading}
				title="Refresh"
				style="display:flex; align-items:center; gap:4px; padding:5px 10px; background:{surface2}; border:1px solid {borderColor}; border-radius:8px; color:{textSecondary}; font-size:12px; cursor:{loading ? 'not-allowed' : 'pointer'};"
			>
				<RefreshCwIcon class="h-3 w-3" style={loading ? 'animation:drl-spin 1s linear infinite;' : ''} />
				Refresh
			</button>
			<button
				onclick={() => (searchOpen = true)}
				style="display:flex; align-items:center; gap:5px; padding:5px 12px; background:{accent}; border:none; border-radius:8px; color:#fff; font-size:12px; font-weight:600; cursor:pointer;"
			>
				<SearchIcon class="h-3.5 w-3.5" />
				Search
			</button>
		</div>
	</div>

	{#if loadError}
		<div style="margin-bottom:10px; font-size:12px; color:{colorError};">{loadError}</div>
	{/if}

	{#if requests.length === 0 && !loading}
		<div style="padding:40px 16px; text-align:center; color:{textMuted}; font-size:13px;">
			{showingSelection ? 'No runs selected.' : 'No review runs yet.'}
		</div>
	{:else}
		<div style="overflow-x:auto;">
			<table style="width:100%; border-collapse:collapse; font-size:13px;">
				<thead>
					<tr style="text-align:left; color:{textMuted}; font-size:11px; text-transform:uppercase; letter-spacing:0.06em;">
						<th style="padding:8px 10px; border-bottom:1px solid {borderColor};">ID</th>
						<th style="padding:8px 10px; border-bottom:1px solid {borderColor};">Request</th>
						<th style="padding:8px 10px; border-bottom:1px solid {borderColor};">Document</th>
						<th style="padding:8px 10px; border-bottom:1px solid {borderColor};">Tier</th>
						<th style="padding:8px 10px; border-bottom:1px solid {borderColor};">Status</th>
						<th style="padding:8px 10px; border-bottom:1px solid {borderColor};">Requester</th>
						<th style="padding:8px 10px; border-bottom:1px solid {borderColor}; text-align:center;">Aspects</th>
						<th style="padding:8px 10px; border-bottom:1px solid {borderColor}; text-align:center;">Findings</th>
						<th style="padding:8px 10px; border-bottom:1px solid {borderColor};">Created</th>
						<th style="padding:8px 10px; border-bottom:1px solid {borderColor};"></th>
					</tr>
				</thead>
				<tbody>
					{#each requests as req (req.run_id)}
						<tr style="border-bottom:1px solid {borderColor};">
							<td style="padding:9px 10px; font-family:monospace; color:{textSecondary};">#{req.run_id}</td>
							<td style="padding:9px 10px; font-family:monospace; color:{textMuted};">#{req.request_id}</td>
							<td style="padding:9px 10px; color:{textPrimary}; max-width:280px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
								{req.doc_title || `Document #${req.input_record_id}`}
							</td>
							<td style="padding:9px 10px; color:{textSecondary};">{tierLabel(req.tier)}</td>
							<td style="padding:9px 10px;">
								<span style="display:inline-block; padding:2px 9px; border-radius:999px; font-size:11px; font-weight:600; color:{statusColor(req.status)}; background:{statusColor(req.status)}1f;">
									{req.status}
								</span>
							</td>
							<td style="padding:9px 10px; color:{textSecondary};">{req.requester_name || '—'}</td>
							<td style="padding:9px 10px; text-align:center; color:{textSecondary};">{req.aspect_count}</td>
							<td style="padding:9px 10px; text-align:center; color:{req.total_findings > 0 ? colorError : textMuted}; font-weight:{req.total_findings > 0 ? 600 : 400};">
								{req.status === 'completed' ? req.total_findings : '—'}
							</td>
							<td style="padding:9px 10px; font-family:monospace; font-size:11px; color:{textMuted};">{fmtClock(req.create_time)}</td>
							<td style="padding:9px 10px; text-align:right;">
								<button
									onclick={() => onView?.(req)}
									style="display:inline-flex; align-items:center; gap:4px; padding:4px 10px; background:{accentTint}; border:1px solid {accent}30; border-radius:8px; color:{accent}; font-size:12px; font-weight:500; cursor:pointer;"
								>
									<EyeIcon class="h-3 w-3" />
									View
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<DocReviewSearchDialog bind:open={searchOpen} {tiers} onSelect={applySelection} />

<style>
	@keyframes drl-spin {
		to {
			transform: rotate(360deg);
		}
	}
</style>
