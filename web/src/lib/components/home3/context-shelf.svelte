<script lang="ts">
	import { untrack } from 'svelte';
	import BotIcon         from '@lucide/svelte/icons/bot';
	import ZapIcon         from '@lucide/svelte/icons/zap';
	import ActivityIcon    from '@lucide/svelte/icons/activity';
	import CpuIcon         from '@lucide/svelte/icons/cpu';
	import DatabaseIcon    from '@lucide/svelte/icons/database';
	import ClockIcon       from '@lucide/svelte/icons/clock';
	import TrendingUpIcon  from '@lucide/svelte/icons/trending-up';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import RefreshCwIcon   from '@lucide/svelte/icons/refresh-cw';
	import CheckIcon       from '@lucide/svelte/icons/check';
	import PlusIcon        from '@lucide/svelte/icons/plus';
	import XIcon           from '@lucide/svelte/icons/x';
	import WrenchIcon      from '@lucide/svelte/icons/wrench';
	import FindingDetailsPanel from '$lib/components/home3/finding-details-panel.svelte';
	import { findingShelf } from '$lib/components/home3/finding-shelf-store.svelte';
	import {
		rangeTypeMapShelf,
		loadRangeTypeMapEntries,
		applyRangeTypeMapEntry,
		applyRangeTypeMapEntryToMetrics
	} from '$lib/components/home3/resolve-metric-range-types-shelf-store.svelte';
	import {
		isInvalidMapEntry,
		splitMapEntriesByStatus,
		sortMapEntriesBy,
		filterMapEntries,
		CANONICAL_BUCKET_OPTIONS,
		type MapEntrySortKey
	} from '$lib/components/home3/resolve-metric-range-types-client.js';

	type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	let {
		darkMode    = true,
		activeMenu  = null,
		width       = 280,
		open        = true,
		onDragStart,
		onClose
	}: {
		darkMode:    boolean;
		activeMenu:  ActiveSelection | null;
		width:       number;
		open:        boolean;
		onDragStart: (e: MouseEvent) => void;
		onClose:     () => void;
	} = $props();

	// --- Layout constants ---
	const radiusCard = '12px'; // cards, panels

	// --- Design tokens ---
	let surface       = $derived(darkMode ? '#1F2333'  : '#FFFFFF');  // shelf background
	let surface2      = $derived(darkMode ? '#252A3A'  : '#ECEEF2');  // card surface inside shelf
	let borderColor   = $derived(darkMode ? '#2D3348'  : '#E4E6EB');  // border
	let accent        = $derived(darkMode ? '#818CF8'  : '#6366F1');  // primary accent
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');

	// Suppress unused props/consts (declared for API completeness / design-token reference)
	void radiusCard; void open;

	let sectionId = $derived(activeMenu?.itemId ?? 'dashboard');

	// --- Value Range Type Map (rendered while resolve-metric-range-types-view.svelte
	// is mounted; see resolve-metric-range-types-shelf-store) ---
	let mapBucketDraft = $state<Record<string, string>>({});
	let mapApplying    = $state<Record<string, boolean>>({});
	let mapApplyResult = $state<Record<string, string>>({});
	let mapApplyError  = $state<Record<string, string>>({});

	// Per-entry state for the separate "Apply to metrics" action (approved tab only).
	let metricsApplying    = $state<Record<string, boolean>>({});
	let metricsApplyResult = $state<Record<string, string>>({});
	let metricsApplyError  = $state<Record<string, string>>({});

	// The map is split across two tabs rather than one sorted list: triage and
	// bulk-apply are different jobs, and mixing them buried the handful of
	// entries needing a decision under the already-approved majority.
	let mapTab = $state<'pending' | 'approved'>('pending');

	// Approving an entry moves it out of the Needs Triage tab, taking its
	// per-entry confirmation with it -- this tab-independent notice is what the
	// operator actually reads after a successful approve.
	let mapNotice = $state('');

	let newEntryRawValue = $state('');
	let newEntryBucket   = $state('');
	let addingEntry      = $state(false);
	let addEntryError    = $state('');
	let addEntryResult   = $state('');

	// Sort/search apply on top of the pending/approved split, shared across both
	// tabs so switching tabs doesn't reset what the operator was looking for.
	let mapSortKey     = $state<MapEntrySortKey | null>(null);
	let mapSearchRaw   = $state('');
	let mapSearchMapped = $state('');

	let splitMapEntries  = $derived(splitMapEntriesByStatus(rangeTypeMapShelf.entries));
	let visibleMapEntries = $derived(
		sortMapEntriesBy(
			filterMapEntries(
				mapTab === 'pending' ? splitMapEntries.pending : splitMapEntries.approved,
				mapSearchRaw,
				mapSearchMapped
			),
			mapSortKey
		)
	);

	// Seed drafts for newly-seen entries only; never clobber an in-progress edit.
	$effect(() => {
		const entries = rangeTypeMapShelf.entries;
		untrack(() => {
			for (const e of entries) {
				if (mapBucketDraft[e.raw_value] === undefined) {
					mapBucketDraft[e.raw_value] = e.canonical_bucket ?? '';
				}
			}
		});
	});

	async function applyMapEntry(rawValue: string) {
		const bucket = (mapBucketDraft[rawValue] ?? '').trim();
		if (!bucket) {
			mapApplyError = { ...mapApplyError, [rawValue]: 'Enter a canonical bucket first.' };
			return;
		}
		mapApplying = { ...mapApplying, [rawValue]: true };
		mapApplyError = { ...mapApplyError, [rawValue]: '' };
		mapApplyResult = { ...mapApplyResult, [rawValue]: '' };
		mapNotice = '';
		try {
			const res = await applyRangeTypeMapEntry(rawValue, bucket);
			const summary = `Approved. ${res.corrected_count} row${res.corrected_count === 1 ? '' : 's'} corrected.`;
			mapApplyResult = { ...mapApplyResult, [rawValue]: summary };
			mapNotice = `${rawValue} → ${bucket}. ${summary}`;
		} catch (e) {
			mapApplyError = { ...mapApplyError, [rawValue]: e instanceof Error ? e.message : String(e) };
		} finally {
			mapApplying = { ...mapApplying, [rawValue]: false };
		}
	}

	// Distinct from applyMapEntry above: that governs the mapping (and only
	// clears the error flag), this rewrites kb.metrics.value_range_type itself to
	// the approved canonical bucket on every row carrying this raw value.
	async function applyEntryToMetrics(rawValue: string) {
		metricsApplying = { ...metricsApplying, [rawValue]: true };
		metricsApplyError = { ...metricsApplyError, [rawValue]: '' };
		metricsApplyResult = { ...metricsApplyResult, [rawValue]: '' };
		try {
			const res = await applyRangeTypeMapEntryToMetrics(rawValue);
			metricsApplyResult = {
				...metricsApplyResult,
				[rawValue]: `${res.applied_count} metric row${res.applied_count === 1 ? '' : 's'} updated.`
			};
		} catch (e) {
			metricsApplyError = {
				...metricsApplyError,
				[rawValue]: e instanceof Error ? e.message : String(e)
			};
		} finally {
			metricsApplying = { ...metricsApplying, [rawValue]: false };
		}
	}

	async function addNewMapEntry() {
		const rawValue = newEntryRawValue.trim();
		const bucket = newEntryBucket.trim();
		if (!rawValue || !bucket) {
			addEntryError = 'raw_value and canonical_bucket are both required.';
			return;
		}
		addingEntry = true;
		addEntryError = '';
		addEntryResult = '';
		try {
			const res = await applyRangeTypeMapEntry(rawValue, bucket);
			addEntryResult = `Added "${res.entry.raw_value}" as approved.`;
			newEntryRawValue = '';
			newEntryBucket = '';
		} catch (e) {
			addEntryError = e instanceof Error ? e.message : String(e);
		} finally {
			addingEntry = false;
		}
	}

	// Model status data
	const models = [
		{ name: 'Claude Sonnet 4.6', status: 'online',   latency: '124ms', usage: 78 },
		{ name: 'GPT-4o',            status: 'online',   latency: '198ms', usage: 41 },
		{ name: 'Gemini Pro',        status: 'degraded', latency: '450ms', usage: 12 }
	];

	const systemMetrics = [
		{ label: 'CPU',       value: 23, unit: '%', color: '#34D399' },
		{ label: 'Memory',    value: 61, unit: '%', color: '#818CF8' },
		{ label: 'API Quota', value: 34, unit: '%', color: '#FBBF24' }
	];

	function modelStatusColor(status: string): string {
		return status === 'online' ? '#34D399' : '#FBBF24';
	}

	const fontMono = "'Fira Code', 'Cascadia Code', monospace";
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<aside
	class="flex-shrink-0 flex flex-col overflow-y-auto relative"
	style="width:{width}px; background:{surface}; border-left:1px solid {borderColor}; scrollbar-width:thin; scrollbar-color:{borderColor} transparent;"
>
	<!-- Resize handle (left edge) -->
	<div
		class="absolute left-0 top-0 bottom-0 cursor-col-resize"
		style="width:4px; z-index:10; background:transparent;"
		onmousedown={onDragStart}
		onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = accent + '40'; }}
		onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = 'transparent'; }}
	></div>

	<div class="p-4 space-y-4" style="padding-left:12px;">
		<!-- Header -->
		<div class="flex items-center justify-between pt-1 pl-1">
			<span class="text-xs font-semibold uppercase tracking-widest" style="color:{textMuted}; font-family:{fontMono};">
				{#if findingShelf.active}Finding
				{:else if rangeTypeMapShelf.active}Value Range Type Map
				{:else if sectionId === 'dashboard'}System Status
				{:else if sectionId === 'agents'}Agent Insights
				{:else if sectionId === 'skills'}Skill Analytics
				{:else if sectionId === 'applications'}App Status
				{:else if sectionId === 'coding'}Code Stats
				{:else if sectionId === 'personal'}Today's Plan
				{:else if sectionId === 'knowledge'}KB Insights
				{:else}Context Info
				{/if}
			</span>
			<button
				onclick={onClose}
				class="flex items-center justify-center w-6 h-6 rounded-md cursor-pointer transition-colors duration-150"
				style="color:{textMuted};"
				onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; (e.currentTarget as HTMLElement).style.background = borderColor; }}
				onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; (e.currentTarget as HTMLElement).style.background = 'transparent'; }}
				aria-label="Close context panel"
			>
				<XIcon class="w-3.5 h-3.5" />
			</button>
		</div>

		{#if findingShelf.active}
			<div class="finding-shelf-body">
				<FindingDetailsPanel
					finding={findingShelf.finding}
					requestId={findingShelf.requestId}
					runId={findingShelf.runId}
					dark={darkMode}
					onFocusMatchedUnit={findingShelf.onFocusMatchedUnit ?? undefined}
					onCloseMatchedUnits={findingShelf.onCloseMatchedUnits ?? undefined}
				/>
			</div>

		{:else if rangeTypeMapShelf.active}
			<div class="flex items-center justify-between gap-2">
				<div class="flex items-center gap-0.5 rounded-md p-0.5 min-w-0" style="background:{surface2};">
					{#each [
						{ id: 'pending'  as const, label: 'Needs Triage', count: splitMapEntries.pending.length },
						{ id: 'approved' as const, label: 'Approved',     count: splitMapEntries.approved.length }
					] as tab (tab.id)}
						<button
							onclick={() => (mapTab = tab.id)}
							class="cursor-pointer whitespace-nowrap"
							style="padding:3px 8px; border-radius:5px; border:none; font-size:11px; font-weight:500;
								background:{mapTab === tab.id ? (darkMode ? '#1F2333' : '#FFFFFF') : 'transparent'};
								color:{mapTab === tab.id ? textPrimary : textMuted};"
						>
							{tab.label} ({tab.count})
						</button>
					{/each}
				</div>
				<button
					onclick={loadRangeTypeMapEntries}
					disabled={rangeTypeMapShelf.loading}
					title="Refresh"
					class="flex items-center justify-center w-6 h-6 rounded-md cursor-pointer flex-shrink-0"
					style="color:{textMuted}; background:transparent; border:none;"
				>
					<RefreshCwIcon class="w-3.5 h-3.5 {rangeTypeMapShelf.loading ? 'animate-spin' : ''}" />
				</button>
			</div>

			{#if rangeTypeMapShelf.error}
				<div class="text-xs" style="color:#F87171;">{rangeTypeMapShelf.error}</div>
			{/if}

			{#if mapNotice}
				<div class="text-[11px] rounded-md px-2 py-1.5" style="background:rgba(52,211,153,0.10); color:#34D399;">
					{mapNotice}
				</div>
			{/if}

			<datalist id="rmrt-shelf-canonical-bucket-options">
				{#each CANONICAL_BUCKET_OPTIONS as opt}
					<option value={opt}></option>
				{/each}
			</datalist>

			<div class="rounded-lg p-2.5" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="text-[11px] font-semibold mb-1.5" style="color:{textMuted};">Add New Entry</div>
				<div class="flex flex-col gap-1.5">
					<input
						type="text"
						placeholder="raw_value"
						bind:value={newEntryRawValue}
						style="background:{darkMode ? '#141824' : '#FFFFFF'}; border:1px solid {borderColor}; color:{textPrimary};
							border-radius:5px; padding:5px 8px; font-size:12px;"
					/>
					<input
						type="text"
						list="rmrt-shelf-canonical-bucket-options"
						placeholder="canonical_bucket"
						bind:value={newEntryBucket}
						style="background:{darkMode ? '#141824' : '#FFFFFF'}; border:1px solid {borderColor}; color:{textPrimary};
							border-radius:5px; padding:5px 8px; font-size:12px;"
					/>
					<button
						onclick={addNewMapEntry}
						disabled={addingEntry}
						class="flex items-center justify-center gap-1.5 cursor-pointer"
						style="padding:6px 10px; border-radius:6px; border:none; background:{accent}; color:white;
							font-size:12px; font-weight:500; opacity:{addingEntry ? 0.6 : 1};"
					>
						<PlusIcon class="w-3.5 h-3.5" />
						{addingEntry ? 'Adding…' : 'Add'}
					</button>
					{#if addEntryError}
						<span class="text-[11px]" style="color:#F87171;">{addEntryError}</span>
					{:else if addEntryResult}
						<span class="text-[11px]" style="color:#34D399;">{addEntryResult}</span>
					{/if}
				</div>
			</div>

			<div class="flex items-center gap-1.5">
				<span class="text-[10px] font-semibold uppercase flex-shrink-0" style="color:{textMuted};">Sort:</span>
				{#each [
					{ id: 'raw_value' as const, label: 'Sort by Raw Values' },
					{ id: 'canonical_bucket' as const, label: 'Sort by Mapped Values' }
				] as opt (opt.id)}
					<button
						onclick={() => (mapSortKey = mapSortKey === opt.id ? null : opt.id)}
						class="cursor-pointer whitespace-nowrap"
						style="padding:3px 8px; border-radius:5px; font-size:10.5px; font-weight:500;
							border:1px solid {mapSortKey === opt.id ? accent : borderColor};
							background:{mapSortKey === opt.id ? accent + '20' : 'transparent'};
							color:{mapSortKey === opt.id ? accent : textMuted};"
					>
						{opt.label}
					</button>
				{/each}
			</div>

			<div class="flex flex-col gap-1.5">
				<input
					type="text"
					placeholder="Search Raw Values"
					bind:value={mapSearchRaw}
					style="background:{darkMode ? '#141824' : '#FFFFFF'}; border:1px solid {borderColor}; color:{textPrimary};
						border-radius:5px; padding:5px 8px; font-size:12px;"
				/>
				<input
					type="text"
					placeholder="Search Mapped Values"
					bind:value={mapSearchMapped}
					style="background:{darkMode ? '#141824' : '#FFFFFF'}; border:1px solid {borderColor}; color:{textPrimary};
						border-radius:5px; padding:5px 8px; font-size:12px;"
				/>
			</div>

			<div class="space-y-2">
				{#if visibleMapEntries.length === 0 && !rangeTypeMapShelf.loading}
					<div class="text-xs" style="color:{textMuted};">
						{mapTab === 'pending' ? 'No entries need triage.' : 'No approved entries yet.'}
					</div>
				{/if}
				{#each visibleMapEntries as entry (entry.raw_value)}
					{@const invalid = isInvalidMapEntry(entry)}
					<div
						class="rounded-lg p-2.5"
						style="background:{surface2}; border:1px solid {invalid ? (darkMode ? 'rgba(251,191,36,0.35)' : 'rgba(217,119,6,0.3)') : borderColor};"
					>
						<div class="flex items-start justify-between gap-2 mb-1.5">
							<span class="text-xs font-medium break-all" style="color:{textPrimary};">{entry.raw_value}</span>
							<span class="text-[10px] font-semibold uppercase flex-shrink-0" style="color:{invalid ? '#FBBF24' : '#34D399'};">
								{entry.status}
							</span>
						</div>
						<div class="flex items-center gap-1.5">
							<input
								type="text"
								list="rmrt-shelf-canonical-bucket-options"
								bind:value={mapBucketDraft[entry.raw_value]}
								placeholder="bucket…"
								class="flex-1 min-w-0"
								style="background:{darkMode ? '#141824' : '#FFFFFF'}; border:1px solid {borderColor}; color:{textPrimary};
									border-radius:5px; padding:4px 6px; font-size:11px;"
							/>
							<button
								onclick={() => applyMapEntry(entry.raw_value)}
								disabled={mapApplying[entry.raw_value]}
								title="Save this bucket and approve the mapping"
								class="flex items-center justify-center flex-shrink-0 cursor-pointer"
								style="width:24px; height:24px; border-radius:5px; border:none; background:{accent}; color:white;
									opacity:{mapApplying[entry.raw_value] ? 0.6 : 1};"
							>
								<CheckIcon class="w-3 h-3" />
							</button>
						</div>
						{#if mapApplyError[entry.raw_value]}
							<div class="text-[11px] mt-1" style="color:#F87171;">{mapApplyError[entry.raw_value]}</div>
						{:else if mapApplyResult[entry.raw_value]}
							<div class="text-[11px] mt-1" style="color:#34D399;">{mapApplyResult[entry.raw_value]}</div>
						{/if}
						{#if !invalid}
							<button
								onclick={() => applyEntryToMetrics(entry.raw_value)}
								disabled={metricsApplying[entry.raw_value]}
								title="Rewrite value_range_type to '{entry.canonical_bucket ?? ''}' on every metric row with this raw value, and clear its error"
								class="w-full flex items-center justify-center gap-1.5 cursor-pointer mt-1.5"
								style="padding:4px 8px; border-radius:5px; border:1px solid {accent}; background:transparent;
									color:{accent}; font-size:11px; font-weight:500;
									opacity:{metricsApplying[entry.raw_value] ? 0.6 : 1};"
							>
								<WrenchIcon class="w-3 h-3" />
								{metricsApplying[entry.raw_value] ? 'Applying…' : 'Apply'}
							</button>
							{#if metricsApplyError[entry.raw_value]}
								<div class="text-[11px] mt-1" style="color:#F87171;">{metricsApplyError[entry.raw_value]}</div>
							{:else if metricsApplyResult[entry.raw_value]}
								<div class="text-[11px] mt-1" style="color:#34D399;">{metricsApplyResult[entry.raw_value]}</div>
							{/if}
						{/if}
					</div>
				{/each}
			</div>

		{:else if sectionId === 'dashboard' || !activeMenu}
			<!-- System Health card -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<CpuIcon class="w-4 h-4" style="color:{accent};" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">System Health</span>
				</div>
				<div class="space-y-3">
					{#each systemMetrics as m}
						<div>
							<div class="flex justify-between mb-1">
								<span class="text-xs" style="color:{textMuted};">{m.label}</span>
								<span class="text-xs font-medium" style="color:{textPrimary}; font-family:{fontMono};">{m.value}{m.unit}</span>
							</div>
							<div class="h-1.5 rounded-full" style="background:{borderColor};">
								<div
									class="h-full rounded-full transition-all duration-500"
									style="width:{m.value}%; background:{m.color}; box-shadow:0 0 6px {m.color}60;"
								></div>
							</div>
						</div>
					{/each}
				</div>
			</div>

			<!-- AI Models card -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<DatabaseIcon class="w-4 h-4" style="color:{accent};" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">AI Models</span>
				</div>
				<div class="space-y-3">
					{#each models as m}
						<div class="flex items-center gap-3">
							<div
								class="w-2 h-2 rounded-full flex-shrink-0"
								style="background:{modelStatusColor(m.status)}; box-shadow:0 0 6px {modelStatusColor(m.status)}80;"
							></div>
							<div class="flex-1 min-w-0">
								<div class="text-xs font-medium truncate" style="color:{textPrimary};">{m.name}</div>
								<div class="text-xs" style="color:{textMuted}; font-family:{fontMono};">{m.latency}</div>
							</div>
							<div class="text-xs" style="color:{textMuted};">{m.usage}%</div>
						</div>
					{/each}
				</div>
			</div>

			<!-- Upcoming card -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<ClockIcon class="w-4 h-4" style="color:#06b6d4;" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">Upcoming</span>
				</div>
				<div class="space-y-1">
					{#each [
						{ time: '10:00', label: 'Team standup' },
						{ time: '14:00', label: 'Code review PR #142' },
						{ time: '16:30', label: 'Daily briefing digest' }
					] as event}
						<div class="flex items-center gap-3 py-1.5">
							<span class="text-xs w-10 flex-shrink-0" style="color:{textMuted}; font-family:{fontMono};">{event.time}</span>
							<div class="w-1 h-1 rounded-full flex-shrink-0" style="background:{accent};"></div>
							<span class="text-xs truncate" style="color:{textPrimary};">{event.label}</span>
						</div>
					{/each}
				</div>
			</div>

		{:else if sectionId === 'agents'}
			<!-- Agent stats 2x2 grid -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-4">
					<BotIcon class="w-4 h-4" style="color:{accent};" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">Agent Overview</span>
				</div>
				<div class="grid grid-cols-2 gap-3">
					{#each [
						{ label: 'Total',        value: '4',   color: accent },
						{ label: 'Active',       value: '2',   color: '#34D399' },
						{ label: 'Tasks Today',  value: '37',  color: '#FBBF24' },
						{ label: 'Success Rate', value: '98%', color: '#06b6d4' }
					] as stat}
						<div class="rounded-lg p-3 text-center" style="background:{darkMode ? '#1F2333' : '#FFFFFF'}; border:1px solid {borderColor};">
							<div class="text-lg font-bold" style="color:{stat.color}; font-family:{fontMono};">{stat.value}</div>
							<div class="text-xs mt-0.5" style="color:{textMuted};">{stat.label}</div>
						</div>
					{/each}
				</div>
			</div>
			<!-- Suggested agents -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<TrendingUpIcon class="w-4 h-4" style="color:#FBBF24;" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">Suggested Agents</span>
				</div>
				{#each ['DataAnalystBot', 'EmailCopilot', 'MeetingScribe'] as name}
					<div class="flex items-center gap-3 py-2" style="border-bottom:1px solid {borderColor};">
						<div class="w-7 h-7 rounded-lg flex items-center justify-center" style="background:{accent}20;">
							<BotIcon class="w-3.5 h-3.5" style="color:{accent};" />
						</div>
						<span class="text-sm flex-1" style="color:{textPrimary};">{name}</span>
						<button class="text-xs px-2 py-0.5 rounded-md cursor-pointer" style="background:{accent}15; color:{accent}; border:none;">Add</button>
					</div>
				{/each}
			</div>

		{:else if sectionId === 'skills'}
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-4">
					<ZapIcon class="w-4 h-4" style="color:#FBBF24;" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">Usage This Month</span>
				</div>
				<div class="space-y-3">
					{#each [
						{ name: 'Summarizer',      pct: 85 },
						{ name: 'Code Formatter',  pct: 64 },
						{ name: 'PDF Extractor',   pct: 45 },
						{ name: 'Web Scraper',     pct: 28 }
					] as s}
						<div>
							<div class="flex justify-between mb-1">
								<span class="text-xs" style="color:{textPrimary};">{s.name}</span>
								<span class="text-xs" style="color:{textMuted}; font-family:{fontMono};">{s.pct}%</span>
							</div>
							<div class="h-1.5 rounded-full" style="background:{borderColor};">
								<div class="h-full rounded-full" style="width:{s.pct}%; background:{accent};"></div>
							</div>
						</div>
					{/each}
				</div>
			</div>

		{:else if sectionId === 'coding'}
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<ActivityIcon class="w-4 h-4" style="color:#FBBF24;" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">Code Stats</span>
				</div>
				{#each [
					{ label: 'Reviews completed', value: '14', icon: CircleCheckIcon, color: '#34D399' },
					{ label: 'Issues flagged',    value: '38', icon: AlertCircleIcon, color: '#FBBF24' },
					{ label: 'Lines generated',   value: '2.4k', icon: ZapIcon,        color: accent }
				] as s}
					<div class="flex items-center gap-3 py-2.5" style="border-bottom:1px solid {borderColor};">
						<s.icon class="w-4 h-4" style="color:{s.color};" />
						<span class="text-sm flex-1" style="color:{textPrimary};">{s.label}</span>
						<span class="text-sm font-bold" style="color:{s.color}; font-family:{fontMono};">{s.value}</span>
					</div>
				{/each}
			</div>

		{:else if sectionId === 'knowledge'}
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<DatabaseIcon class="w-4 h-4" style="color:#06b6d4;" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">KB Stats</span>
				</div>
				{#each [
					{ label: 'Total documents', value: '234' },
					{ label: 'Indexed chunks',  value: '8.7k' },
					{ label: 'Searches today',  value: '42' },
					{ label: 'Avg relevance',   value: '94%' }
				] as s}
					<div class="flex justify-between py-2" style="border-bottom:1px solid {borderColor};">
						<span class="text-sm" style="color:{textMuted};">{s.label}</span>
						<span class="text-sm font-semibold" style="color:{textPrimary}; font-family:{fontMono};">{s.value}</span>
					</div>
				{/each}
			</div>

		{:else}
			<!-- Generic context -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="text-sm font-semibold mb-2" style="color:{textPrimary};">
					{activeMenu?.itemTitle ?? 'Information'}
				</div>
				<p class="text-sm" style="color:{textMuted};">
					Select an item from the navigation to see contextual information here.
				</p>
			</div>
		{/if}
	</div>
</aside>
