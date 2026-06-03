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
	import Link2Icon from '@lucide/svelte/icons/link-2';

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

	// ---------- Related categories state ----------
	let relatedSelectedKey = $state<string | null>(null);
	let mergeTargetEl = $state<HTMLInputElement | null>(null);

	let selected = $derived(categories.find((c) => c.category_key === selectedKey) ?? null);

	// Compute closeness between two categories using weighted Jaccard on available data.
	// Weights: spec name overlap (0.4) + required attrs overlap (0.2) + key word overlap (0.4).
	function closeness(a: InventoryCategoryRecord, b: InventoryCategoryRecord): number {
		let score = 0;

		const specsA = new Set(Object.keys(a.specs ?? {}));
		const specsB = new Set(Object.keys(b.specs ?? {}));
		if (specsA.size > 0 || specsB.size > 0) {
			const inter = [...specsA].filter((s) => specsB.has(s)).length;
			const union = new Set([...specsA, ...specsB]).size;
			score += 0.4 * (inter / union);
		}

		const attrsA = new Set(a.required_attrs ?? []);
		const attrsB = new Set(b.required_attrs ?? []);
		if (attrsA.size > 0 || attrsB.size > 0) {
			const inter = [...attrsA].filter((x) => attrsB.has(x)).length;
			const union = new Set([...attrsA, ...attrsB]).size;
			score += 0.2 * (inter / union);
		}

		const wordsA = new Set(a.category_key.split('_').filter(Boolean));
		const wordsB = new Set(b.category_key.split('_').filter(Boolean));
		const interW = [...wordsA].filter((w) => wordsB.has(w)).length;
		const unionW = new Set([...wordsA, ...wordsB]).size;
		score += 0.4 * (interW / unionW);

		return score;
	}

	let relatedCategories = $derived(
		!selected
			? []
			: categories
					.filter((c) => c.category_key !== selected.category_key)
					.map((c) => ({ cat: c, score: closeness(selected, c) }))
					.sort((a, b) => b.score - a.score)
					.slice(0, 10)
	);

	let relatedSelected = $derived(
		relatedCategories.find((r) => r.cat.category_key === relatedSelectedKey)?.cat ?? null
	);

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
		relatedSelectedKey = null;
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

	function useAsMergeTarget() {
		if (!relatedSelected) return;
		canonicalOf = relatedSelected.category_key;
		mergeTargetEl?.focus();
	}

	// ---------- Panel resize ----------
	const PANEL_MIN = 180;
	const PANEL_MAX = 940;

	let leftWidth = $state(440);
	let rightWidth = $state(760);
	let isDraggingLeft = $state(false);
	let isDraggingRight = $state(false);
	let leftDivHover = $state(false);
	let rightDivHover = $state(false);
	let _dragStartX = 0;
	let _dragStartWidth = 0;

	function startDragLeft(e: MouseEvent) {
		isDraggingLeft = true;
		_dragStartX = e.clientX;
		_dragStartWidth = leftWidth;
		e.preventDefault();
	}

	function startDragRight(e: MouseEvent) {
		isDraggingRight = true;
		_dragStartX = e.clientX;
		_dragStartWidth = rightWidth;
		e.preventDefault();
	}

	// Attach global mouse handlers once; read state directly in callbacks.
	$effect(() => {
		const onMove = (e: MouseEvent) => {
			if (isDraggingLeft) {
				leftWidth = Math.max(PANEL_MIN, Math.min(PANEL_MAX, _dragStartWidth + e.clientX - _dragStartX));
			} else if (isDraggingRight) {
				rightWidth = Math.max(PANEL_MIN, Math.min(PANEL_MAX, _dragStartWidth - (e.clientX - _dragStartX)));
			}
		};
		const onUp = () => { isDraggingLeft = false; isDraggingRight = false; };
		window.addEventListener('mousemove', onMove);
		window.addEventListener('mouseup', onUp);
		return () => { window.removeEventListener('mousemove', onMove); window.removeEventListener('mouseup', onUp); };
	});

	// Lock cursor and suppress text selection while dragging.
	$effect(() => {
		const dragging = isDraggingLeft || isDraggingRight;
		document.body.style.cursor = dragging ? 'col-resize' : '';
		document.body.style.userSelect = dragging ? 'none' : '';
	});

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

	<!-- Body: master list + right side -->
	<div class="flex min-h-0 flex-1 overflow-hidden">
		<!-- Left: category list -->
		<aside
			class="flex flex-shrink-0 flex-col overflow-hidden"
			style="width:{leftWidth}px; background:{panelBgAlt};"
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

		<!-- Left resize handle -->
		<button
			type="button"
			class="cr-divider"
			onmousedown={startDragLeft}
			onmouseenter={() => (leftDivHover = true)}
			onmouseleave={() => (leftDivHover = false)}
			aria-label="Resize category list"
		>
			<div
				class="cr-divider-line"
				style="background:{leftDivHover || isDraggingLeft ? accent : borderColor}; width:{leftDivHover || isDraggingLeft ? '2px' : '1px'};"
				aria-hidden="true"
			></div>
		</button>

		<!-- Right side: Category Under Review + Related Categories -->
		<div class="flex min-h-0 flex-1 overflow-hidden">

			<!-- Category Under Review (detail editor) -->
			<main class="min-w-0 flex-1 flex flex-col overflow-hidden">
				<!-- Column header — matches the aesthetic of the left list and right related panels -->
				<div
					class="flex flex-shrink-0 items-center gap-2 px-5 py-3"
					style="border-bottom:1px solid {borderColor}; background:{panelBg};"
				>
					<ClipboardCheckIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
					<span style="font-size:12px; font-weight:700; color:{textSecondary}; letter-spacing:0.04em; text-transform:uppercase;">
						Category Under Review
					</span>
				</div>

				{#if !selected}
					<div class="flex flex-1 flex-col items-center justify-center p-8 text-center">
						<ClipboardCheckIcon class="mb-3 h-10 w-10" style="color:{textMuted}; opacity:0.4;" />
						<p style="font-size:14px; color:{textSecondary};">
							Select a category from the list to review and curate it.
						</p>
					</div>
				{:else}
					<!-- Scrollable content — full width, no max-w constraint -->
					<div class="flex-1 min-h-0 overflow-y-auto px-5 py-4" style="scrollbar-width:thin;">
						<!-- Title row -->
						<div class="mb-4 flex items-center gap-3">
							<h2
								class="truncate"
								style="font-size:17px; font-weight:700; color:{textPrimary}; font-family:{fontMono}; margin:0;"
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
							<span style="font-size:12px; color:{textMuted}; margin-left:auto; flex-shrink:0;">
								seen {selected.seen_count} time{selected.seen_count === 1 ? '' : 's'}
							</span>
						</div>

						<!-- Display names (read-only observed surface forms) -->
						<section class="mb-4">
							<div class="mb-1.5 flex items-center gap-2">
								<TagIcon class="h-3.5 w-3.5 flex-shrink-0" style="color:{accent};" />
								<h3 style="font-size:11px; font-weight:700; color:{textSecondary}; margin:0; text-transform:uppercase; letter-spacing:0.04em;">
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
						<section class="mb-4">
							<div class="mb-1.5 flex items-center gap-2">
								<CheckIcon class="h-3.5 w-3.5 flex-shrink-0" style="color:{accent};" />
								<h3 style="font-size:11px; font-weight:700; color:{textSecondary}; margin:0; text-transform:uppercase; letter-spacing:0.04em;">
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
						<section class="mb-4">
							<div class="mb-1.5 flex items-center gap-2">
								<LayersIcon class="h-3.5 w-3.5 flex-shrink-0" style="color:{accent};" />
								<h3 style="font-size:11px; font-weight:700; color:{textSecondary}; margin:0; text-transform:uppercase; letter-spacing:0.04em;">
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
									<div class="px-3 py-3" style="font-size:12px; color:{textMuted}; font-style:italic;">No specs defined.</div>
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
						<section class="mb-4">
							<div class="mb-1.5 flex items-center gap-2">
								<RulerIcon class="h-3.5 w-3.5 flex-shrink-0" style="color:{accent};" />
								<h3 style="font-size:11px; font-weight:700; color:{textSecondary}; margin:0; text-transform:uppercase; letter-spacing:0.04em;">
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
									<div class="px-3 py-3" style="font-size:12px; color:{textMuted}; font-style:italic;">No ranges defined.</div>
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
						<section>
							<div class="mb-1.5 flex items-center gap-2">
								<GitMergeIcon class="h-3.5 w-3.5 flex-shrink-0" style="color:{accent};" />
								<h3 style="font-size:11px; font-weight:700; color:{textSecondary}; margin:0; text-transform:uppercase; letter-spacing:0.04em;">
									Merge Into
								</h3>
								<span style="font-size:11px; color:{textMuted};">(surviving category key)</span>
							</div>
							<input
								type="text"
								bind:value={canonicalOf}
								bind:this={mergeTargetEl}
								placeholder="e.g. pump"
								class="w-full rounded-lg px-3 py-2"
								style="font-size:13px; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; font-family:{fontMono};"
							/>
						</section>
					</div>

					<!-- Action bar — pinned to column bottom, always visible regardless of scroll -->
					<div
						class="flex-shrink-0"
						style="border-top:1px solid {borderColor}; background:{panelBg};"
					>
						{#if saveError}
							<div
								class="px-5 py-2"
								style="font-size:12px; color:{statusColor('rejected')}; border-bottom:1px solid {statusColor('rejected')}30; background:{statusColor('rejected')}0D;"
							>
								{saveError}
							</div>
						{/if}
						{#if saveNotice}
							<div
								class="px-5 py-2"
								style="font-size:12px; color:{statusColor('approved')}; border-bottom:1px solid {statusColor('approved')}30; background:{statusColor('approved')}0D;"
							>
								{saveNotice}
							</div>
						{/if}
						<div class="flex flex-wrap items-center gap-2 px-5 py-3">
							<button
								type="button"
								onclick={saveSchema}
								disabled={saving}
								class="flex items-center gap-2 rounded-lg px-4 py-2"
								style="font-size:13px; font-weight:600; background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; cursor:{saving ? 'default' : 'pointer'}; opacity:{saving ? 0.6 : 1};"
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

			<!-- Right resize handle -->
			<button
				type="button"
				class="cr-divider"
				onmousedown={startDragRight}
				onmouseenter={() => (rightDivHover = true)}
				onmouseleave={() => (rightDivHover = false)}
				aria-label="Resize related categories"
			>
				<div
					class="cr-divider-line"
					style="background:{rightDivHover || isDraggingRight ? accent : borderColor}; width:{rightDivHover || isDraggingRight ? '2px' : '1px'};"
					aria-hidden="true"
				></div>
			</button>

			<!-- Related Categories panel -->
			<div
				class="flex flex-shrink-0 flex-col overflow-hidden"
				style="width:{rightWidth}px; background:{panelBgAlt};"
			>
				<!-- Upper: ranked list -->
				<div class="cr-related-upper flex flex-1 flex-col overflow-hidden" style="border-bottom:1px solid {borderColor};">
					<div
						class="flex flex-shrink-0 items-center gap-2 px-4 py-3"
						style="border-bottom:1px solid {borderColor}; background:{panelBg};"
					>
						<Link2Icon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
						<span style="font-size:12px; font-weight:700; color:{textSecondary}; letter-spacing:0.04em; text-transform:uppercase;">
							Related Categories
						</span>
						<span style="font-size:11px; color:{textMuted}; font-family:{fontMono}; margin-left:auto;">
							{relatedCategories.length}
						</span>
					</div>
					<div class="flex-1 overflow-y-auto" style="scrollbar-width:thin;">
						{#if !selected}
							<div class="px-4 py-6 text-center" style="font-size:12px; color:{textMuted};">
								Select a category to see related ones.
							</div>
						{:else if relatedCategories.length === 0}
							<div class="flex flex-col items-center px-4 py-8 text-center">
								<InboxIcon class="mb-2 h-6 w-6" style="color:{textMuted};" />
								<span style="font-size:12px; color:{textMuted};">No related categories found.</span>
							</div>
						{:else}
							{#each relatedCategories as { cat, score } (cat.category_key)}
								<button
									type="button"
									onclick={() => (relatedSelectedKey = cat.category_key)}
									class="block w-full px-4 py-2.5 text-left transition-colors duration-150"
									style="
										border-bottom:1px solid {borderColor};
										background:{relatedSelectedKey === cat.category_key ? accentTint : 'transparent'};
										border-left:3px solid {relatedSelectedKey === cat.category_key ? accent : 'transparent'};
										cursor:pointer;
									"
									onmouseenter={(e) => {
										if (relatedSelectedKey !== cat.category_key)
											(e.currentTarget as HTMLElement).style.background = hoverBg;
									}}
									onmouseleave={(e) => {
										if (relatedSelectedKey !== cat.category_key)
											(e.currentTarget as HTMLElement).style.background = 'transparent';
									}}
								>
									<div class="flex items-center justify-between gap-2">
										<span
											class="truncate"
											style="font-size:12px; font-weight:600; color:{textPrimary}; font-family:{fontMono};"
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
										<span>seen {cat.seen_count}</span>
										<span
											style="margin-left:auto; font-family:{fontMono}; color:{accent}; font-size:11px;"
											title="Closeness score (spec overlap + attr overlap + key word overlap)"
										>{score.toFixed(2)}</span>
									</div>
								</button>
							{/each}
						{/if}
					</div>
				</div>

				<!-- Lower: related category detail -->
				<div class="cr-related-lower flex flex-1 flex-col overflow-hidden">
					<div
						class="flex flex-shrink-0 items-center gap-2 px-4 py-3"
						style="border-bottom:1px solid {borderColor}; background:{panelBg};"
					>
						<LayersIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
						<span style="font-size:12px; font-weight:700; color:{textSecondary}; letter-spacing:0.04em; text-transform:uppercase;">
							Related Category Details
						</span>
					</div>
					<div class="flex-1 overflow-y-auto p-4" style="scrollbar-width:thin;">
						{#if !relatedSelected}
							<div class="flex flex-col items-center py-8 text-center">
								<Link2Icon class="mb-2 h-6 w-6" style="color:{textMuted}; opacity:0.5;" />
								<span style="font-size:12px; color:{textMuted};">
									Select a related category above to see its details.
								</span>
							</div>
						{:else}
							<!-- Header -->
							<div class="mb-4">
								<div class="flex items-center gap-2 flex-wrap">
									<span
										style="font-size:15px; font-weight:700; color:{textPrimary}; font-family:{fontMono};"
									>{relatedSelected.category_key}</span>
									<span
										class="rounded-full px-2 py-0.5"
										style="
											font-size:10px;
											font-weight:700;
											color:{statusColor(relatedSelected.status)};
											background:{statusColor(relatedSelected.status)}1A;
										"
									>{statusLabel(relatedSelected.status)}</span>
								</div>
								<p style="font-size:11px; color:{textMuted}; margin:4px 0 0;">
									Observed {relatedSelected.seen_count} time{relatedSelected.seen_count === 1 ? '' : 's'} in the corpus.
								</p>
							</div>

							<!-- Display names -->
							<div class="mb-4">
								<div class="mb-1.5 flex items-center gap-1.5">
									<TagIcon class="h-3.5 w-3.5 flex-shrink-0" style="color:{accent};" />
									<span style="font-size:11px; font-weight:700; color:{textSecondary}; text-transform:uppercase; letter-spacing:0.04em;">Display Names</span>
								</div>
								{#if (relatedSelected.display_names?.length ?? 0) === 0}
									<span style="font-size:11px; color:{textMuted};">—</span>
								{:else}
									<div class="flex flex-wrap gap-1.5">
										{#each relatedSelected.display_names as name (name)}
											<span
												class="rounded px-1.5 py-0.5"
												style="font-size:11px; background:{panelBg}; border:1px solid {borderColor}; color:{textSecondary}; font-family:{fontMono};"
											>{name}</span>
										{/each}
									</div>
								{/if}
							</div>

							<!-- Required attrs -->
							<div class="mb-4">
								<div class="mb-1.5 flex items-center gap-1.5">
									<CheckIcon class="h-3.5 w-3.5 flex-shrink-0" style="color:{accent};" />
									<span style="font-size:11px; font-weight:700; color:{textSecondary}; text-transform:uppercase; letter-spacing:0.04em;">Required Attrs</span>
								</div>
								{#if (relatedSelected.required_attrs?.length ?? 0) === 0}
									<span style="font-size:11px; color:{textMuted};">None defined.</span>
								{:else}
									<div class="flex flex-wrap gap-1.5">
										{#each relatedSelected.required_attrs as attr (attr)}
											<span
												class="rounded px-1.5 py-0.5"
												style="font-size:11px; background:{accentTint}; border:1px solid {accent}40; color:{accent}; font-family:{fontMono};"
											>{attr}</span>
										{/each}
									</div>
								{/if}
							</div>

							<!-- Spec definitions (read-only) -->
							<div class="mb-4">
								<div class="mb-1.5 flex items-center gap-1.5">
									<LayersIcon class="h-3.5 w-3.5 flex-shrink-0" style="color:{accent};" />
									<span style="font-size:11px; font-weight:700; color:{textSecondary}; text-transform:uppercase; letter-spacing:0.04em;">Specs</span>
								</div>
								{#if Object.keys(relatedSelected.specs ?? {}).length === 0}
									<span style="font-size:11px; color:{textMuted};">No specs defined.</span>
								{:else}
									<div class="overflow-hidden rounded-lg" style="border:1px solid {borderColor}; background:{cardBg};">
										<div
											class="grid gap-2 px-3 py-1.5"
											style="grid-template-columns: 1fr 0.8fr 1.2fr; border-bottom:1px solid {borderColor}; background:{panelBg};"
										>
											<span style="font-size:10px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Name</span>
											<span style="font-size:10px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Unit</span>
											<span style="font-size:10px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Aliases</span>
										</div>
										{#each Object.entries(relatedSelected.specs ?? {}) as [name, spec] (name)}
											<div
												class="grid gap-2 px-3 py-1.5"
												style="grid-template-columns: 1fr 0.8fr 1.2fr; border-bottom:1px solid {borderColor};"
											>
												<span style="font-size:11px; color:{textPrimary}; font-family:{fontMono};">{name}</span>
												<span style="font-size:11px; color:{textSecondary}; font-family:{fontMono};">{spec.canonical_unit || '—'}</span>
												<span style="font-size:11px; color:{textMuted};" class="truncate">{(spec.aliases ?? []).join(', ') || '—'}</span>
											</div>
										{/each}
									</div>
								{/if}
							</div>

							<!-- Plausible ranges (read-only) -->
							<div class="mb-4">
								<div class="mb-1.5 flex items-center gap-1.5">
									<RulerIcon class="h-3.5 w-3.5 flex-shrink-0" style="color:{accent};" />
									<span style="font-size:11px; font-weight:700; color:{textSecondary}; text-transform:uppercase; letter-spacing:0.04em;">Plausible Ranges</span>
								</div>
								{#if Object.keys(relatedSelected.plausible_ranges ?? {}).length === 0}
									<span style="font-size:11px; color:{textMuted};">No ranges defined.</span>
								{:else}
									<div class="overflow-hidden rounded-lg" style="border:1px solid {borderColor}; background:{cardBg};">
										<div
											class="grid gap-2 px-3 py-1.5"
											style="grid-template-columns: 1fr 0.6fr 0.6fr 0.6fr; border-bottom:1px solid {borderColor}; background:{panelBg};"
										>
											<span style="font-size:10px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Name</span>
											<span style="font-size:10px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Min</span>
											<span style="font-size:10px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Max</span>
											<span style="font-size:10px; font-weight:700; color:{textMuted}; text-transform:uppercase;">Unit</span>
										</div>
										{#each Object.entries(relatedSelected.plausible_ranges ?? {}) as [name, range] (name)}
											<div
												class="grid gap-2 px-3 py-1.5"
												style="grid-template-columns: 1fr 0.6fr 0.6fr 0.6fr; border-bottom:1px solid {borderColor};"
											>
												<span style="font-size:11px; color:{textPrimary}; font-family:{fontMono};">{name}</span>
												<span style="font-size:11px; color:{textSecondary}; font-family:{fontMono};">{range.min ?? '—'}</span>
												<span style="font-size:11px; color:{textSecondary}; font-family:{fontMono};">{range.max ?? '—'}</span>
												<span style="font-size:11px; color:{textMuted};">{range.unit || '—'}</span>
											</div>
										{/each}
									</div>
								{/if}
							</div>

							<!-- Use as Merge Target -->
							<button
								type="button"
								onclick={useAsMergeTarget}
								class="flex w-full items-center justify-center gap-2 rounded-lg px-3 py-2.5 transition-colors duration-150"
								style="
									font-size:12px;
									font-weight:600;
									border:1px solid {statusColor('merged')};
									background:transparent;
									color:{statusColor('merged')};
									cursor:pointer;
								"
								title="Copy this category key into the Merge Into field of the editor"
							>
								<GitMergeIcon class="h-4 w-4" />
								Use as Merge Target
							</button>
						{/if}
					</div>
				</div>
			</div>

		</div>
		<!-- end right side -->
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
	.cr-related-upper {
		/* Take up roughly the top half of the related panel */
		flex: 0 0 50%;
		min-height: 120px;
	}
	.cr-related-lower {
		flex: 1 1 0;
		min-height: 0;
	}
	.cr-divider {
		flex-shrink: 0;
		width: 6px;
		cursor: col-resize;
		display: flex;
		align-items: stretch;
		justify-content: center;
		z-index: 10;
		padding: 0;
		border: 0;
		background: transparent;
	}
	.cr-divider-line {
		height: 100%;
		transition: background 120ms ease-out, width 120ms ease-out;
	}
</style>
