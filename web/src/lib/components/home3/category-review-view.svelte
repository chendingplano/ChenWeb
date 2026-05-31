<script lang="ts">
	import {
		listInventoryCategories,
		updateInventoryCategory,
		type InventoryCategoryRecord,
		type InventoryCategoryStatus,
		type InventorySpecSchema,
		type InventoryPlausibleRange,
		type UpdateInventoryCategoryPayload
	} from '$lib/services/kbService';
	import ClipboardCheckIcon from '@lucide/svelte/icons/clipboard-check';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CheckIcon from '@lucide/svelte/icons/check';
	import XIcon from '@lucide/svelte/icons/x';
	import GitMergeIcon from '@lucide/svelte/icons/git-merge';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import SaveIcon from '@lucide/svelte/icons/save';
	import TagIcon from '@lucide/svelte/icons/tag';
	import LayersIcon from '@lucide/svelte/icons/layers';
	import RulerIcon from '@lucide/svelte/icons/ruler';
	import InboxIcon from '@lucide/svelte/icons/inbox';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// ---------- Design tokens (home3 indigo) ----------
	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let panelBg = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let panelBgAlt = $derived(darkMode ? '#1E2331' : '#F7F8FA');
	let cardBg = $derived(darkMode ? '#1E2331' : '#FFFFFF');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let hoverBg = $derived(darkMode ? 'rgba(45,51,72,0.6)' : 'rgba(228,230,235,0.7)');
	let inputBg = $derived(darkMode ? '#171B26' : '#FFFFFF');
	const fontMono = "'Fira Code', 'Cascadia Code', monospace";

	type StatusFilter = InventoryCategoryStatus | 'all';
	const STATUS_TABS: Array<{ id: StatusFilter; label: string }> = [
		{ id: 'pending_review', label: 'Pending Review' },
		{ id: 'approved', label: 'Approved' },
		{ id: 'rejected', label: 'Rejected' },
		{ id: 'merged', label: 'Merged' },
		{ id: 'all', label: 'All' }
	];

	function statusColor(status: string): string {
		switch (status) {
			case 'approved':
				return darkMode ? '#34D399' : '#059669';
			case 'pending_review':
				return darkMode ? '#FBBF24' : '#D97706';
			case 'rejected':
				return darkMode ? '#F87171' : '#DC2626';
			case 'merged':
				return darkMode ? '#A78BFA' : '#7C3AED';
			default:
				return textMuted;
		}
	}

	function statusLabel(status: string): string {
		return STATUS_TABS.find((t) => t.id === status)?.label ?? status;
	}

	// ---------- List state ----------
	let statusFilter = $state<StatusFilter>('pending_review');
	let categories = $state<InventoryCategoryRecord[]>([]);
	let loading = $state(false);
	let loadError = $state('');
	let selectedKey = $state<string | null>(null);

	// ---------- Editor state (working copy of the selected category) ----------
	type SpecRow = { name: string; canonical_unit: string; aliases: string };
	type RangeRow = { name: string; min: string; max: string; unit: string };

	let requiredAttrs = $state<string[]>([]);
	let newRequiredAttr = $state('');
	let specRows = $state<SpecRow[]>([]);
	let rangeRows = $state<RangeRow[]>([]);
	let canonicalOf = $state('');
	let saving = $state(false);
	let saveError = $state('');
	let saveNotice = $state('');

	let selected = $derived(categories.find((c) => c.category_key === selectedKey) ?? null);

	async function load() {
		loading = true;
		loadError = '';
		try {
			const res = await listInventoryCategories(statusFilter, 200);
			categories = res.results ?? [];
			// Keep selection if still present; otherwise select the first row.
			if (!categories.some((c) => c.category_key === selectedKey)) {
				selectCategory(categories[0] ?? null);
			}
		} catch (err) {
			categories = [];
			loadError = err instanceof Error ? err.message : 'Failed to load categories';
		} finally {
			loading = false;
		}
	}

	function selectCategory(cat: InventoryCategoryRecord | null) {
		saveError = '';
		saveNotice = '';
		selectedKey = cat?.category_key ?? null;
		if (!cat) {
			requiredAttrs = [];
			specRows = [];
			rangeRows = [];
			canonicalOf = '';
			return;
		}
		requiredAttrs = [...(cat.required_attrs ?? [])];
		specRows = Object.entries(cat.specs ?? {}).map(([name, spec]) => ({
			name,
			canonical_unit: spec.canonical_unit ?? '',
			aliases: (spec.aliases ?? []).join(', ')
		}));
		rangeRows = Object.entries(cat.plausible_ranges ?? {}).map(([name, range]) => ({
			name,
			min: range.min == null ? '' : String(range.min),
			max: range.max == null ? '' : String(range.max),
			unit: range.unit ?? ''
		}));
		canonicalOf = cat.canonical_of ?? '';
	}

	function selectFilter(id: StatusFilter) {
		if (statusFilter === id) return;
		statusFilter = id;
		void load();
	}

	// ---------- required_attrs editing ----------
	function addRequiredAttr() {
		const v = newRequiredAttr.trim();
		if (!v || requiredAttrs.includes(v)) {
			newRequiredAttr = '';
			return;
		}
		requiredAttrs = [...requiredAttrs, v];
		newRequiredAttr = '';
	}
	function removeRequiredAttr(attr: string) {
		requiredAttrs = requiredAttrs.filter((a) => a !== attr);
	}

	// ---------- specs editing ----------
	function addSpecRow() {
		specRows = [...specRows, { name: '', canonical_unit: '', aliases: '' }];
	}
	function removeSpecRow(index: number) {
		specRows = specRows.filter((_, i) => i !== index);
	}

	// ---------- plausible_ranges editing ----------
	function addRangeRow() {
		rangeRows = [...rangeRows, { name: '', min: '', max: '', unit: '' }];
	}
	function removeRangeRow(index: number) {
		rangeRows = rangeRows.filter((_, i) => i !== index);
	}

	// ---------- build payload ----------
	function buildSchemaPayload(): UpdateInventoryCategoryPayload {
		const specs: Record<string, InventorySpecSchema> = {};
		for (const row of specRows) {
			const name = row.name.trim();
			if (!name) continue;
			specs[name] = {
				canonical_unit: row.canonical_unit.trim(),
				aliases: row.aliases
					.split(',')
					.map((a) => a.trim())
					.filter(Boolean)
			};
		}
		const plausible_ranges: Record<string, InventoryPlausibleRange> = {};
		for (const row of rangeRows) {
			const name = row.name.trim();
			if (!name) continue;
			const min = row.min.trim() === '' ? null : Number(row.min);
			const max = row.max.trim() === '' ? null : Number(row.max);
			plausible_ranges[name] = {
				min: Number.isFinite(min as number) ? (min as number) : null,
				max: Number.isFinite(max as number) ? (max as number) : null,
				unit: row.unit.trim()
			};
		}
		return {
			required_attrs: requiredAttrs,
			specs,
			plausible_ranges
		};
	}

	async function applyUpdate(
		payload: UpdateInventoryCategoryPayload,
		successMsg: string
	) {
		if (!selected) return;
		saving = true;
		saveError = '';
		saveNotice = '';
		try {
			const res = await updateInventoryCategory(selected.category_key, payload);
			// Patch the local list in place so the UI reflects the new state.
			categories = categories.map((c) =>
				c.category_key === res.result.category_key ? res.result : c
			);
			saveNotice = successMsg;
			// If the row no longer matches the active filter, drop it from view.
			if (statusFilter !== 'all' && res.result.status !== statusFilter) {
				categories = categories.filter((c) => c.category_key !== res.result.category_key);
				selectCategory(categories[0] ?? null);
			} else {
				selectCategory(res.result);
				saveNotice = successMsg;
			}
		} catch (err) {
			saveError = err instanceof Error ? err.message : 'Update failed';
		} finally {
			saving = false;
		}
	}

	function saveSchema() {
		void applyUpdate(buildSchemaPayload(), 'Schema saved');
	}

	function approve() {
		// Persist any schema edits together with the approval.
		void applyUpdate({ ...buildSchemaPayload(), status: 'approved' }, 'Category approved');
	}

	function reject() {
		if (!selected) return;
		if (!window.confirm(`Reject category "${selected.category_key}"?`)) return;
		void applyUpdate({ status: 'rejected' }, 'Category rejected');
	}

	function merge() {
		if (!selected) return;
		const target = canonicalOf.trim();
		if (!target) {
			saveError = 'Enter a surviving category key in "Merge into" before merging.';
			return;
		}
		if (target === selected.category_key) {
			saveError = 'A category cannot be merged into itself.';
			return;
		}
		if (!window.confirm(`Merge "${selected.category_key}" into "${target}"?`)) return;
		void applyUpdate({ status: 'merged', canonical_of: target }, 'Category merged');
	}

	$effect(() => {
		void load();
	});
</script>

<div class="cr-page flex h-full flex-col" style="background:{pageBg}; color:{textPrimary};">
	<!-- Header -->
	<header
		class="flex flex-shrink-0 items-center gap-4 px-6 py-4"
		style="border-bottom:1px solid {borderColor};"
	>
		<div
			class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl"
			style="background:{accentTint}; color:{accent}; border:1px solid {accent}30;"
		>
			<ClipboardCheckIcon class="h-5 w-5" />
		</div>
		<div class="min-w-0 flex-1">
			<h1 style="font-size:16px; font-weight:700; color:{textPrimary}; margin:0;">
				Category Review
			</h1>
			<p style="font-size:12px; color:{textSecondary}; margin:0;">
				Curate the inventory category ontology — fill in schema, then approve, reject, or merge
				pending categories.
			</p>
		</div>
		<button
			type="button"
			onclick={() => load()}
			disabled={loading}
			class="flex items-center gap-2 rounded-lg px-3 py-2 transition-colors duration-150"
			style="
				border:1px solid {borderColor};
				background:{panelBg};
				color:{textSecondary};
				font-size:13px;
				cursor:{loading ? 'default' : 'pointer'};
				opacity:{loading ? 0.6 : 1};
			"
			title="Refresh"
		>
			<RefreshCwIcon class="h-4 w-4 {loading ? 'cr-spin' : ''}" />
			Refresh
		</button>
	</header>

	<!-- Status filter tabs -->
	<div class="flex flex-shrink-0 gap-1 px-6 py-3" style="border-bottom:1px solid {borderColor};">
		{#each STATUS_TABS as tab (tab.id)}
			<button
				type="button"
				onclick={() => selectFilter(tab.id)}
				class="rounded-full px-3 py-1.5 transition-colors duration-150"
				style="
					font-size:12px;
					font-weight:600;
					border:1px solid {statusFilter === tab.id ? accent : borderColor};
					background:{statusFilter === tab.id ? accentTint : 'transparent'};
					color:{statusFilter === tab.id ? accent : textSecondary};
					cursor:pointer;
				"
			>
				{tab.label}
			</button>
		{/each}
	</div>

	<!-- Body: master-detail -->
	<div class="flex min-h-0 flex-1 overflow-hidden">
		<!-- List -->
		<aside
			class="flex w-[340px] flex-shrink-0 flex-col overflow-hidden"
			style="border-right:1px solid {borderColor}; background:{panelBgAlt};"
		>
			<div
				class="flex items-center justify-between px-4 py-3"
				style="border-bottom:1px solid {borderColor};"
			>
				<span style="font-size:12px; font-weight:700; color:{textSecondary}; letter-spacing:0.04em; text-transform:uppercase;">
					Categories
				</span>
				<span
					style="font-size:11px; color:{textMuted}; font-family:{fontMono};"
				>{categories.length}</span>
			</div>
			<div class="flex-1 overflow-y-auto" style="scrollbar-width:thin;">
				{#if loading}
					<div class="px-4 py-6 text-center" style="font-size:13px; color:{textMuted};">
						Loading…
					</div>
				{:else if loadError}
					<div class="px-4 py-6" style="font-size:13px; color:{statusColor('rejected')};">
						{loadError}
					</div>
				{:else if categories.length === 0}
					<div class="flex flex-col items-center px-4 py-10 text-center">
						<InboxIcon class="mb-2 h-8 w-8" style="color:{textMuted};" />
						<span style="font-size:13px; color:{textMuted};">No categories in this view.</span>
					</div>
				{:else}
					{#each categories as cat (cat.category_key)}
						<button
							type="button"
							onclick={() => selectCategory(cat)}
							class="block w-full px-4 py-3 text-left transition-colors duration-150"
							style="
								border-bottom:1px solid {borderColor};
								background:{selectedKey === cat.category_key ? accentTint : 'transparent'};
								border-left:3px solid {selectedKey === cat.category_key ? accent : 'transparent'};
								cursor:pointer;
							"
							onmouseenter={(e) => {
								if (selectedKey !== cat.category_key)
									(e.currentTarget as HTMLElement).style.background = hoverBg;
							}}
							onmouseleave={(e) => {
								if (selectedKey !== cat.category_key)
									(e.currentTarget as HTMLElement).style.background = 'transparent';
							}}
						>
							<div class="flex items-center justify-between gap-2">
								<span
									class="truncate"
									style="font-size:13px; font-weight:600; color:{textPrimary}; font-family:{fontMono};"
								>{cat.category_key}</span>
								<span
									class="flex-shrink-0 rounded-full px-2 py-0.5"
									style="
										font-size:10px;
										font-weight:700;
										color:{statusColor(cat.status)};
										background:{statusColor(cat.status)}1A;
									"
								>{statusLabel(cat.status)}</span>
							</div>
							<div class="mt-1 flex items-center gap-3" style="font-size:11px; color:{textMuted};">
								<span title="Times observed in the corpus">seen {cat.seen_count}</span>
								{#if (cat.display_names?.length ?? 0) > 0}
									<span class="truncate">{cat.display_names.length} surface form{cat.display_names.length === 1 ? '' : 's'}</span>
								{/if}
							</div>
						</button>
					{/each}
				{/if}
			</div>
		</aside>

		<!-- Detail editor -->
		<main class="min-w-0 flex-1 overflow-y-auto" style="scrollbar-width:thin;">
			{#if !selected}
				<div class="flex h-full flex-col items-center justify-center p-8 text-center">
					<ClipboardCheckIcon class="mb-3 h-10 w-10" style="color:{textMuted}; opacity:0.5;" />
					<p style="font-size:14px; color:{textSecondary};">
						Select a category from the list to review and curate it.
					</p>
				</div>
			{:else}
				<div class="mx-auto max-w-3xl p-6">
					<!-- Title row -->
					<div class="mb-5 flex items-start justify-between gap-4">
						<div class="min-w-0">
							<div class="flex items-center gap-3">
								<h2
									class="truncate"
									style="font-size:20px; font-weight:700; color:{textPrimary}; font-family:{fontMono}; margin:0;"
								>{selected.category_key}</h2>
								<span
									class="flex-shrink-0 rounded-full px-2.5 py-1"
									style="
										font-size:11px;
										font-weight:700;
										color:{statusColor(selected.status)};
										background:{statusColor(selected.status)}1A;
									"
								>{statusLabel(selected.status)}</span>
							</div>
							<p style="font-size:12px; color:{textMuted}; margin:4px 0 0;">
								Observed {selected.seen_count} time{selected.seen_count === 1 ? '' : 's'} in the corpus.
							</p>
						</div>
					</div>

					<!-- Display names (read-only observed surface forms) -->
					<section class="mb-6">
						<div class="mb-2 flex items-center gap-2">
							<TagIcon class="h-4 w-4" style="color:{accent};" />
							<h3 style="font-size:13px; font-weight:700; color:{textPrimary}; margin:0;">
								Display Names
							</h3>
							<span style="font-size:11px; color:{textMuted};">(observed surface forms)</span>
						</div>
						<div class="flex flex-wrap gap-2">
							{#if (selected.display_names?.length ?? 0) === 0}
								<span style="font-size:12px; color:{textMuted};">—</span>
							{:else}
								{#each selected.display_names as name (name)}
									<span
										class="rounded-md px-2 py-1"
										style="font-size:12px; background:{panelBg}; border:1px solid {borderColor}; color:{textSecondary}; font-family:{fontMono};"
									>{name}</span>
								{/each}
							{/if}
						</div>
					</section>

					<!-- Required attrs -->
					<section class="mb-6">
						<div class="mb-2 flex items-center gap-2">
							<CheckIcon class="h-4 w-4" style="color:{accent};" />
							<h3 style="font-size:13px; font-weight:700; color:{textPrimary}; margin:0;">
								Required Attributes
							</h3>
						</div>
						<div class="mb-2 flex flex-wrap gap-2">
							{#if requiredAttrs.length === 0}
								<span style="font-size:12px; color:{textMuted};">None yet.</span>
							{:else}
								{#each requiredAttrs as attr (attr)}
									<span
										class="flex items-center gap-1 rounded-md px-2 py-1"
										style="font-size:12px; background:{accentTint}; border:1px solid {accent}40; color:{accent}; font-family:{fontMono};"
									>
										{attr}
										<button
											type="button"
											onclick={() => removeRequiredAttr(attr)}
											style="border:none; background:transparent; color:{accent}; cursor:pointer; display:flex;"
											aria-label="Remove {attr}"
										>
											<XIcon class="h-3 w-3" />
										</button>
									</span>
								{/each}
							{/if}
						</div>
						<div class="flex gap-2">
							<input
								type="text"
								bind:value={newRequiredAttr}
								onkeydown={(e) => {
									if (e.key === 'Enter') {
										e.preventDefault();
										addRequiredAttr();
									}
								}}
								placeholder="e.g. manufacturer"
								class="flex-1 rounded-lg px-3 py-2"
								style="font-size:13px; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; font-family:{fontMono};"
							/>
							<button
								type="button"
								onclick={addRequiredAttr}
								class="flex items-center gap-1 rounded-lg px-3 py-2"
								style="font-size:13px; background:{panelBg}; border:1px solid {borderColor}; color:{textSecondary}; cursor:pointer;"
							>
								<PlusIcon class="h-4 w-4" /> Add
							</button>
						</div>
					</section>

					<!-- Specs -->
					<section class="mb-6">
						<div class="mb-2 flex items-center gap-2">
							<LayersIcon class="h-4 w-4" style="color:{accent};" />
							<h3 style="font-size:13px; font-weight:700; color:{textPrimary}; margin:0;">
								Spec Definitions
							</h3>
						</div>
						<div
							class="overflow-hidden rounded-lg"
							style="border:1px solid {borderColor}; background:{cardBg};"
						>
							<div
								class="grid items-center gap-2 px-3 py-2"
								style="grid-template-columns: 1fr 1fr 1.4fr 32px; border-bottom:1px solid {borderColor}; background:{panelBg};"
							>
								<span style="font-size:11px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Spec name</span>
								<span style="font-size:11px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Canonical unit</span>
								<span style="font-size:11px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Aliases (comma sep.)</span>
								<span></span>
							</div>
							{#if specRows.length === 0}
								<div class="px-3 py-3" style="font-size:12px; color:{textMuted};">No specs defined.</div>
							{:else}
								{#each specRows as row, i (i)}
									<div
										class="grid items-center gap-2 px-3 py-2"
										style="grid-template-columns: 1fr 1fr 1.4fr 32px; border-bottom:1px solid {borderColor};"
									>
										<input
											type="text"
											bind:value={row.name}
											placeholder="power"
											class="rounded px-2 py-1.5"
											style="font-size:12px; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; font-family:{fontMono}; min-width:0;"
										/>
										<input
											type="text"
											bind:value={row.canonical_unit}
											placeholder="w"
											class="rounded px-2 py-1.5"
											style="font-size:12px; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; font-family:{fontMono}; min-width:0;"
										/>
										<input
											type="text"
											bind:value={row.aliases}
											placeholder="watt, watts"
											class="rounded px-2 py-1.5"
											style="font-size:12px; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; min-width:0;"
										/>
										<button
											type="button"
											onclick={() => removeSpecRow(i)}
											class="flex items-center justify-center rounded"
											style="border:none; background:transparent; color:{textMuted}; cursor:pointer; height:28px;"
											aria-label="Remove spec"
										>
											<Trash2Icon class="h-4 w-4" />
										</button>
									</div>
								{/each}
							{/if}
							<button
								type="button"
								onclick={addSpecRow}
								class="flex w-full items-center gap-1 px-3 py-2"
								style="font-size:12px; background:transparent; border:none; color:{accent}; cursor:pointer;"
							>
								<PlusIcon class="h-4 w-4" /> Add spec
							</button>
						</div>
					</section>

					<!-- Plausible ranges -->
					<section class="mb-6">
						<div class="mb-2 flex items-center gap-2">
							<RulerIcon class="h-4 w-4" style="color:{accent};" />
							<h3 style="font-size:13px; font-weight:700; color:{textPrimary}; margin:0;">
								Plausible Ranges
							</h3>
						</div>
						<div
							class="overflow-hidden rounded-lg"
							style="border:1px solid {borderColor}; background:{cardBg};"
						>
							<div
								class="grid items-center gap-2 px-3 py-2"
								style="grid-template-columns: 1.4fr 1fr 1fr 1fr 32px; border-bottom:1px solid {borderColor}; background:{panelBg};"
							>
								<span style="font-size:11px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Spec name</span>
								<span style="font-size:11px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Min</span>
								<span style="font-size:11px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Max</span>
								<span style="font-size:11px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Unit</span>
								<span></span>
							</div>
							{#if rangeRows.length === 0}
								<div class="px-3 py-3" style="font-size:12px; color:{textMuted};">No ranges defined.</div>
							{:else}
								{#each rangeRows as row, i (i)}
									<div
										class="grid items-center gap-2 px-3 py-2"
										style="grid-template-columns: 1.4fr 1fr 1fr 1fr 32px; border-bottom:1px solid {borderColor};"
									>
										<input
											type="text"
											bind:value={row.name}
											placeholder="power"
											class="rounded px-2 py-1.5"
											style="font-size:12px; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; font-family:{fontMono}; min-width:0;"
										/>
										<input
											type="number"
											bind:value={row.min}
											placeholder="1"
											class="rounded px-2 py-1.5"
											style="font-size:12px; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; font-family:{fontMono}; min-width:0;"
										/>
										<input
											type="number"
											bind:value={row.max}
											placeholder="100000"
											class="rounded px-2 py-1.5"
											style="font-size:12px; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; font-family:{fontMono}; min-width:0;"
										/>
										<input
											type="text"
											bind:value={row.unit}
											placeholder="w"
											class="rounded px-2 py-1.5"
											style="font-size:12px; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; font-family:{fontMono}; min-width:0;"
										/>
										<button
											type="button"
											onclick={() => removeRangeRow(i)}
											class="flex items-center justify-center rounded"
											style="border:none; background:transparent; color:{textMuted}; cursor:pointer; height:28px;"
											aria-label="Remove range"
										>
											<Trash2Icon class="h-4 w-4" />
										</button>
									</div>
								{/each}
							{/if}
							<button
								type="button"
								onclick={addRangeRow}
								class="flex w-full items-center gap-1 px-3 py-2"
								style="font-size:12px; background:transparent; border:none; color:{accent}; cursor:pointer;"
							>
								<PlusIcon class="h-4 w-4" /> Add range
							</button>
						</div>
					</section>

					<!-- Merge target -->
					<section class="mb-6">
						<div class="mb-2 flex items-center gap-2">
							<GitMergeIcon class="h-4 w-4" style="color:{accent};" />
							<h3 style="font-size:13px; font-weight:700; color:{textPrimary}; margin:0;">
								Merge Into
							</h3>
							<span style="font-size:11px; color:{textMuted};">(surviving category key)</span>
						</div>
						<input
							type="text"
							bind:value={canonicalOf}
							placeholder="e.g. pump"
							class="w-full rounded-lg px-3 py-2"
							style="font-size:13px; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; font-family:{fontMono};"
						/>
					</section>

					<!-- Notices -->
					{#if saveError}
						<div
							class="mb-3 rounded-lg px-3 py-2"
							style="font-size:13px; background:{statusColor('rejected')}1A; color:{statusColor('rejected')}; border:1px solid {statusColor('rejected')}40;"
						>
							{saveError}
						</div>
					{/if}
					{#if saveNotice}
						<div
							class="mb-3 rounded-lg px-3 py-2"
							style="font-size:13px; background:{statusColor('approved')}1A; color:{statusColor('approved')}; border:1px solid {statusColor('approved')}40;"
						>
							{saveNotice}
						</div>
					{/if}

					<!-- Action bar -->
					<div
						class="sticky bottom-0 flex flex-wrap items-center gap-2 py-3"
						style="background:{pageBg}; border-top:1px solid {borderColor};"
					>
						<button
							type="button"
							onclick={saveSchema}
							disabled={saving}
							class="flex items-center gap-2 rounded-lg px-4 py-2"
							style="font-size:13px; font-weight:600; background:{panelBg}; border:1px solid {borderColor}; color:{textPrimary}; cursor:{saving ? 'default' : 'pointer'}; opacity:{saving ? 0.6 : 1};"
						>
							<SaveIcon class="h-4 w-4" /> Save Schema
						</button>
						<button
							type="button"
							onclick={approve}
							disabled={saving}
							class="flex items-center gap-2 rounded-lg px-4 py-2"
							style="font-size:13px; font-weight:700; background:{statusColor('approved')}; border:none; color:#fff; cursor:{saving ? 'default' : 'pointer'}; opacity:{saving ? 0.6 : 1};"
						>
							<CheckIcon class="h-4 w-4" /> Approve
						</button>
						<button
							type="button"
							onclick={merge}
							disabled={saving}
							class="flex items-center gap-2 rounded-lg px-4 py-2"
							style="font-size:13px; font-weight:600; background:transparent; border:1px solid {statusColor('merged')}; color:{statusColor('merged')}; cursor:{saving ? 'default' : 'pointer'}; opacity:{saving ? 0.6 : 1};"
						>
							<GitMergeIcon class="h-4 w-4" /> Merge
						</button>
						<button
							type="button"
							onclick={reject}
							disabled={saving}
							class="ml-auto flex items-center gap-2 rounded-lg px-4 py-2"
							style="font-size:13px; font-weight:600; background:transparent; border:1px solid {statusColor('rejected')}; color:{statusColor('rejected')}; cursor:{saving ? 'default' : 'pointer'}; opacity:{saving ? 0.6 : 1};"
						>
							<XIcon class="h-4 w-4" /> Reject
						</button>
					</div>
				</div>
			{/if}
		</main>
	</div>
</div>

<style>
	.cr-page {
		height: 100%;
	}
	:global(.cr-spin) {
		animation: cr-spin 1s linear infinite;
	}
	@keyframes cr-spin {
		from {
			transform: rotate(0deg);
		}
		to {
			transform: rotate(360deg);
		}
	}
</style>
