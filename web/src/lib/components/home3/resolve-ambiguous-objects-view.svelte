<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { onMount } from 'svelte';
	import {
		listOpenAmbiguousItems,
		getAmbiguousItemArtifactObject,
		closeAttentionItem,
		getAmbiguousObjectDetail,
		updateArtifactObject,
		updateObjectNode,
		createObjectNode,
		findArtifactObjectsByObjectNode,
		mergeObjectNodes,
		buildArtifactObjectPatch,
		buildObjectNodePatch,
		neighborAmbiguousId,
		nextIndexAfterResolve,
		rebindArtifactObjectsToMaster,
		fieldDirty,
		describeCandidateMatches,
		RECONCILE_STATUS_OPTIONS,
		type AttentionItem,
		type ArtifactObjectDetail,
		type LinkedArtifactObjectSummary,
		type ObjectNodeCandidate,
		type CandidateFieldMatchKey,
		type CandidateMatchDetails
	} from './resolve-ambiguous-objects-client.js';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Design tokens (matches db-consistency-view.svelte / db-maint-log-view.svelte) ---
	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');

	// --- Left panel state: open ambiguous-object-name attention items ---
	let items = $state<AttentionItem[]>([]);
	let listLoading = $state(false);
	let listError = $state('');

	// --- Selected attention item ---
	let selectedItemId = $state<number | null>(null);
	let selectedItem = $derived(items.find((item) => item.id === selectedItemId) ?? null);
	let itemNoRow = $state(false); // no occurrence still has a kb.artifact_objects row
	let closeNote = $state('');
	let closing = $state(false);
	let closeError = $state('');

	// --- Right panel state ---
	let selectedId = $state<number | null>(null);
	let detailLoading = $state(false);
	let detailError = $state('');
	let snapshotObject = $state<ArtifactObjectDetail | null>(null);
	let currentObject = $state<ArtifactObjectDetail | null>(null);
	let snapshotNodes = $state<ObjectNodeCandidate[]>([]);
	let currentNodes = $state<ObjectNodeCandidate[]>([]);

	let saving = $state(false);
	let saveError = $state('');
	let helpOpen = $state(false);

	let creatingNode = $state(false);
	let createNodeError = $state('');
	let existsNotice = $state('');
	let rebindNotice = $state('');

	// --- Merge (Related Object Nodes) state ---
	let masterNodeId = $state<string | null>(null);
	let selectedNodeIds = $state<Set<string>>(new Set());
	let merging = $state(false);
	let mergeError = $state('');
	let mergeConfirm = $state(false);
	let rebinding = $state(false);

	type FindContext = {
		sourceObjectID: string;
		sourceLabel: string;
		masterObjectID: string | null;
	};

	let findContext = $state<FindContext | null>(null);
	let findRows = $state<LinkedArtifactObjectSummary[]>([]);
	let findLoading = $state(false);
	let findError = $state('');

	// Merge is allowed only with exactly one master and at least one other node
	// picked to merge into it. The master is never in selectedNodeIds (see
	// setMaster), so size >= 1 already guarantees "at least one other".
	let canMerge = $derived(
		masterNodeId !== null && selectedNodeIds.size >= 1 && !merging && !saving
	);

	function setMaster(objectId: string, checked: boolean) {
		if (checked) {
			masterNodeId = objectId;
			if (selectedNodeIds.has(objectId)) {
				const next = new Set(selectedNodeIds);
				next.delete(objectId);
				selectedNodeIds = next;
			}
		} else if (masterNodeId === objectId) {
			masterNodeId = null;
		}
	}

	function setSelected(objectId: string, checked: boolean) {
		const next = new Set(selectedNodeIds);
		if (checked) next.add(objectId);
		else next.delete(objectId);
		selectedNodeIds = next;
	}

	function resetMergeSelection() {
		masterNodeId = null;
		selectedNodeIds = new Set();
		mergeError = '';
		mergeConfirm = false;
	}

	function clearFindMode() {
		findContext = null;
		findRows = [];
		findLoading = false;
		findError = '';
	}

	function requestMerge() {
		if (!canMerge) return;
		mergeConfirm = true;
	}

	async function confirmMerge() {
		if (masterNodeId === null || selectedNodeIds.size === 0) return;
		mergeConfirm = false;
		merging = true;
		mergeError = '';
		const survivor = masterNodeId;
		const losers = [...selectedNodeIds];
		try {
			for (const loser of losers) {
				await mergeObjectNodes(survivor, loser);
			}
			resetMergeSelection();
			if (selectedId !== null) await selectRow(selectedId);
		} catch (e) {
			mergeError = e instanceof Error ? e.message : String(e);
		} finally {
			merging = false;
		}
	}

	let navConfirm = $state<{ kind: 'prev' | 'next' | 'switch' } | null>(null);
	let pendingSelectId = $state<number | null>(null);
	let cancelConfirm = $state(false);

	let isDirty = $derived.by(() => {
		if (!snapshotObject || !currentObject) return false;
		if (JSON.stringify(snapshotObject) !== JSON.stringify(currentObject)) return true;
		return JSON.stringify(snapshotNodes) !== JSON.stringify(currentNodes);
	});

	function objDirty(field: keyof ArtifactObjectDetail): boolean {
		if (!currentObject || !snapshotObject) return false;
		return fieldDirty(currentObject[field], snapshotObject[field]);
	}

	function nodeDirty(index: number, field: keyof ObjectNodeCandidate): boolean {
		if (!currentNodes[index] || !snapshotNodes[index]) return false;
		return fieldDirty(currentNodes[index][field], snapshotNodes[index][field]);
	}

	function dirtyStyle(dirty: boolean): string {
		return dirty ? 'border-color:#F87171; color:#F87171;' : '';
	}

	function emptyMatchDetails(): CandidateMatchDetails {
		return {
			matchedFields: {},
			matchedTerms: [],
			hasLexicalMatch: false,
			hasMatch: false,
			objectTypeMatched: false
		};
	}

	function nodeMatchDetails(node: ObjectNodeCandidate): CandidateMatchDetails {
		if (!currentObject) return emptyMatchDetails();
		return describeCandidateMatches(currentObject, node);
	}

	function matchedFieldStyle(
		dirty: boolean,
		matches: CandidateMatchDetails,
		field: CandidateFieldMatchKey | 'object_type'
	): string {
		if (dirty) return dirtyStyle(true);
		const hasMatch =
			field === 'object_type'
				? matches.hasLexicalMatch && matches.objectTypeMatched
				: (matches.matchedFields[field]?.length ?? 0) > 0;
		return hasMatch ? 'border-color:#34D399; background:rgba(52,211,153,0.08); color:#A7F3D0;' : '';
	}

	function matchesSummaryText(matches: CandidateMatchDetails): string {
		const parts: string[] = [];
		if (matches.matchedTerms.length > 0) {
			parts.push(`terms: ${matches.matchedTerms.join(', ')}`);
		}
		if (matches.hasLexicalMatch && matches.objectTypeMatched) {
			parts.push('type');
		}
		return parts.join(' · ');
	}

	// The left list shows attention items, or artifact objects in find mode;
	// navigation moves through whichever is shown.
	let navKey = $derived(findContext ? selectedId : selectedItemId);
	let prevId = $derived.by(() =>
		navKey === null ? null : neighborAmbiguousId(currentVisibleRowIds(), navKey, -1)
	);
	let nextId = $derived.by(() =>
		navKey === null ? null : neighborAmbiguousId(currentVisibleRowIds(), navKey, 1)
	);

	function currentVisibleRowIds(): number[] {
		return findContext ? findRows.map((row) => row.id) : items.map((item) => item.id);
	}

	function openTarget(id: number): Promise<void> {
		return findContext ? selectRow(id) : selectItem(id);
	}

	function requestNav(kind: 'prev' | 'next' | 'switch', id: number) {
		if (saving) return;
		if (isDirty) {
			navConfirm = { kind };
			pendingSelectId = id;
			return;
		}
		openTarget(id);
	}

	function goPrev() {
		if (prevId !== null) requestNav('prev', prevId);
	}

	function goNext() {
		if (nextId !== null) requestNav('next', nextId);
	}

	function clickCard(id: number) {
		if (id === navKey) return;
		requestNav('switch', id);
	}

	async function confirmNavSave() {
		const target = pendingSelectId;
		navConfirm = null;
		pendingSelectId = null;
		const success = await doSave();
		if (success && target !== null) await openTarget(target);
	}

	function confirmNavDiscard() {
		const target = pendingSelectId;
		navConfirm = null;
		pendingSelectId = null;
		if (target !== null) openTarget(target);
	}

	function stayOnNav() {
		navConfirm = null;
		pendingSelectId = null;
	}

	function requestCancel() {
		if (saving || !isDirty) return;
		cancelConfirm = true;
	}

	function confirmCancel() {
		if (snapshotObject) currentObject = { ...snapshotObject };
		currentNodes = snapshotNodes.map((c) => ({ ...c }));
		cancelConfirm = false;
	}

	function dismissCancel() {
		cancelConfirm = false;
	}

	async function doSave(): Promise<boolean> {
		if (!currentObject || !snapshotObject || selectedId === null) return false;
		if (saving) return false;
		saving = true;
		saveError = '';
		try {
			const objectPatch = buildArtifactObjectPatch(snapshotObject, currentObject);
			if (Object.keys(objectPatch).length > 0) {
				await updateArtifactObject(selectedId, objectPatch);
			}
			for (let i = 0; i < currentNodes.length; i++) {
				const nodePatch = buildObjectNodePatch(snapshotNodes[i], currentNodes[i]);
				if (Object.keys(nodePatch).length > 0) {
					await updateObjectNode(currentNodes[i].object_id, nodePatch);
				}
			}
			// Saving never closes the attention item; Resolve or Dismiss does.
			snapshotObject = { ...currentObject };
			snapshotNodes = currentNodes.map((c) => ({ ...c }));
			return true;
		} catch (e) {
			saveError = e instanceof Error ? e.message : String(e);
			return false;
		} finally {
			saving = false;
		}
	}

	function clearSelection() {
		selectedItemId = null;
		selectedId = null;
		itemNoRow = false;
		detailLoading = false;
		currentObject = null;
		snapshotObject = null;
		currentNodes = [];
		snapshotNodes = [];
	}

	async function closeSelectedItem(action: 'resolve' | 'dismiss') {
		if (selectedItemId === null || closing || isDirty || saving) return;
		const id = selectedItemId;
		closing = true;
		closeError = '';
		try {
			await closeAttentionItem(id, action, closeNote);
			clearFindMode();
			const nextIndex = nextIndexAfterResolve(
				items.map((item) => item.id),
				id
			);
			items = items.filter((item) => item.id !== id);
			closeNote = '';
			if (nextIndex !== null) {
				await selectItem(items[nextIndex].id);
			} else {
				clearSelection();
			}
		} catch (e) {
			closeError = e instanceof Error ? e.message : String(e);
		} finally {
			closing = false;
		}
	}

	async function createNew() {
		if (!currentObject || selectedId === null || saving || creatingNode) return;
		const objectName = currentObject.object_name.trim();
		if (!objectName) {
			createNodeError = m.resolve_ambiguous_objects_object_name_is_required_to();
			return;
		}
		creatingNode = true;
		createNodeError = '';
		try {
			const result = await createObjectNode(selectedId, {
				object_name: currentObject.object_name,
				object_name_en: currentObject.object_name_en,
				object_name_zh: currentObject.object_name_zh,
				language: currentObject.language,
				object_type: currentObject.object_type,
				aliases: currentObject.aliases,
				acronyms: currentObject.acronyms,
				description: currentObject.description
			});
			if (result.created) {
				if (!currentObject) return;
				currentObject.object_id = result.node.object_id;
				currentObject.reconcile_status = 'new';
				await doSave();
			} else {
				let addedAny = false;
				for (const node of result.nodes) {
					if (currentNodes.some((n) => n.object_id === node.object_id)) continue;
					currentNodes = [...currentNodes, node];
					snapshotNodes = [...snapshotNodes, { ...node }];
					addedAny = true;
				}
				existsNotice = addedAny
					? m.resolve_ambiguous_objects_an_object_node_with_this()
					: m.resolve_ambiguous_objects_an_object_node_with_this_2();
			}
		} catch (e) {
			createNodeError = e instanceof Error ? e.message : String(e);
		} finally {
			creatingNode = false;
		}
	}

	async function loadList() {
		listLoading = true;
		listError = '';
		try {
			const res = await listOpenAmbiguousItems();
			items = res.items;
			if (selectedItemId !== null && !items.some((item) => item.id === selectedItemId)) {
				clearSelection();
			}
			if (items.length > 0 && selectedItemId === null) {
				await selectItem(items[0].id);
			}
		} catch (e) {
			listError = e instanceof Error ? e.message : String(e);
		} finally {
			listLoading = false;
		}
	}

	async function refreshLeftPanel() {
		const leavingFind = findContext !== null;
		clearFindMode();
		await loadList();
		// Back from find mode: show the selected item's artifact object again.
		if (leavingFind && selectedItemId !== null) await selectItem(selectedItemId);
	}

	async function findLinkedArtifactObjects(node: ObjectNodeCandidate) {
		findLoading = true;
		findError = '';
		try {
			const res = await findArtifactObjectsByObjectNode(node.object_id);
			findContext = {
				sourceObjectID: node.object_id,
				sourceLabel: node.canonical_name || node.object_id,
				masterObjectID: masterNodeId
			};
			findRows = res.rows;
			if (findRows.length > 0 && !findRows.some((row) => row.id === selectedId)) {
				await selectRow(findRows[0].id);
			}
		} catch (e) {
			findError = e instanceof Error ? e.message : String(e);
		} finally {
			findLoading = false;
		}
	}

	async function rebindFoundArtifactObjects() {
		if (!findContext || rebinding || findRows.length === 0) return;
		const survivorObjectID = masterNodeId ?? findContext.masterObjectID;
		if (!survivorObjectID) {
			rebindNotice = m.resolve_ambiguous_objects_please_select_one_related_object();
			return;
		}
		if (survivorObjectID === findContext.sourceObjectID) {
			rebindNotice = m.resolve_ambiguous_objects_the_selected_master_is_already();
			return;
		}
		rebinding = true;
		saveError = '';
		try {
			await rebindArtifactObjectsToMaster(
				findRows.map((row) => row.id),
				survivorObjectID
			);
			rebindNotice = m.resolve_ambiguous_objects_rebound_artifact_object_to({
				findRowsCount: findRows.length,
				plural: findRows.length === 1 ? '' : 's',
				survivorObjectID
			});
			await refreshLeftPanel();
		} catch (e) {
			saveError = e instanceof Error ? e.message : String(e);
		} finally {
			rebinding = false;
		}
	}

	async function selectItem(itemId: number) {
		selectedItemId = itemId;
		closeNote = '';
		closeError = '';
		itemNoRow = false;
		selectedId = null;
		currentObject = null;
		snapshotObject = null;
		currentNodes = [];
		snapshotNodes = [];
		detailLoading = true;
		detailError = '';
		try {
			const res = await getAmbiguousItemArtifactObject(itemId);
			if (selectedItemId !== itemId) return; // a newer selection superseded this one
			if (!res.found) {
				itemNoRow = true;
				detailLoading = false;
				return;
			}
			await selectRow(res.artifact_object_id);
		} catch (e) {
			if (selectedItemId !== itemId) return;
			detailError = e instanceof Error ? e.message : String(e);
			detailLoading = false;
		}
	}

	async function selectRow(id: number) {
		selectedId = id;
		detailLoading = true;
		detailError = '';
		saveError = '';
		resetMergeSelection();
		try {
			const detail = await getAmbiguousObjectDetail(id);
			if (selectedId !== id) return; // a newer selectRow call superseded this one
			snapshotObject = detail.artifact_object;
			currentObject = { ...detail.artifact_object };
			snapshotNodes = detail.candidates;
			currentNodes = detail.candidates.map((c) => ({ ...c }));
		} catch (e) {
			if (selectedId !== id) return;
			detailError = e instanceof Error ? e.message : String(e);
		} finally {
			if (selectedId === id) detailLoading = false;
		}
	}

	function useCandidate(node: ObjectNodeCandidate) {
		if (!currentObject) return;
		currentObject.object_id = node.object_id;
		currentObject.object_name = node.canonical_name;
		currentObject.object_name_en = node.canonical_name_en;
		currentObject.object_name_zh = node.canonical_name_zh;
		currentObject.acronyms = [...node.acronyms];
		currentObject.reconcile_status = 'ambiguous_resolved';
	}

	function aliasesText(values: string[]): string {
		return values.join(', ');
	}

	function leftRowMeta(row: LinkedArtifactObjectSummary): string {
		return `${row.artifact_type} · ${row.reconcile_status}${row.object_id ? ` · ${row.object_id}` : ''}`;
	}

	function formatTime(value: string | undefined | null): string {
		if (!value) return '';
		const date = new Date(value);
		return Number.isNaN(date.getTime()) ? value : date.toLocaleString(getLocale());
	}

	function llmOutcomeLabel(outcome: string | undefined): string {
		switch (outcome) {
			case 'unresolved':
				return m.resolve_ambiguous_objects_llm_unresolved();
			case 'failed':
				return m.resolve_ambiguous_objects_llm_failed();
			default:
				return m.resolve_ambiguous_objects_llm_not_configured();
		}
	}

	function parseAliasesText(text: string): string[] {
		return text
			.split(',')
			.map((s) => s.trim())
			.filter(Boolean);
	}

	onMount(() => {
		loadList();
	});
</script>

<div
	class="flex"
	style="height:100%; overflow:hidden; background:{pageBg}; user-select:text; -webkit-user-select:text;"
>
	<!-- Left panel -->
	<div class="flex shrink-0 flex-col" style="width:320px; border-right:1px solid {borderColor};">
		<div class="shrink-0 p-4" style="border-bottom:1px solid {borderColor};">
			<div class="flex items-start justify-between gap-3">
				<div>
					<h1 style="font-size:16px; font-weight:600; color:{textPrimary}; margin-bottom:2px;">
						{findContext
							? m.resolve_ambiguous_objects_linked_artifact_objects()
							: m.resolve_ambiguous_objects_resolve_ambiguous_objects()}
					</h1>
					<p style="font-size:12px; color:{textSecondary};">
						{#if findContext}
							{m.resolve_ambiguous_objects_row_linked_to({
								findRowsCount: findRows.length,
								plural: findRows.length === 1 ? '' : 's',
								sourceLabel: findContext.sourceLabel
							})}
						{:else}
							{m.resolve_ambiguous_objects_open_items({
								count: items.length,
								plural: items.length === 1 ? '' : 's'
							})}
						{/if}
					</p>
				</div>
				<div class="flex items-center gap-2">
					{#if findContext}
						<button
							type="button"
							onclick={goPrev}
							disabled={prevId === null || detailLoading}
							style="font-size:12px; font-weight:500; padding:6px 10px; border-radius:6px; border:1px solid {borderColor}; cursor:{prevId ===
								null || detailLoading
								? 'not-allowed'
								: 'pointer'}; background:{cardBg}; color:{textPrimary}; opacity:{prevId === null ||
							detailLoading
								? 0.5
								: 1};"
						>
							{m.resolve_ambiguous_objects_prev()}
						</button>
						<button
							type="button"
							onclick={goNext}
							disabled={nextId === null || detailLoading}
							style="font-size:12px; font-weight:500; padding:6px 10px; border-radius:6px; border:1px solid {borderColor}; cursor:{nextId ===
								null || detailLoading
								? 'not-allowed'
								: 'pointer'}; background:{cardBg}; color:{textPrimary}; opacity:{nextId === null ||
							detailLoading
								? 0.5
								: 1};"
						>
							{m.resolve_ambiguous_objects_next()}
						</button>
						<button
							type="button"
							onclick={rebindFoundArtifactObjects}
							disabled={rebinding || findLoading || findRows.length === 0}
							style="font-size:12px; font-weight:600; padding:6px 10px; border-radius:6px; border:none; cursor:{rebinding ||
							findLoading ||
							findRows.length === 0
								? 'not-allowed'
								: 'pointer'}; background:{accent}; color:white; opacity:{rebinding ||
							findLoading ||
							findRows.length === 0
								? 0.5
								: 1};"
						>
							{rebinding
								? m.resolve_ambiguous_objects_rebinding()
								: m.resolve_ambiguous_objects_rebind()}
						</button>
					{/if}
					<button
						type="button"
						onclick={refreshLeftPanel}
						disabled={listLoading}
						style="font-size:12px; font-weight:500; padding:6px 10px; border-radius:6px; border:1px solid {borderColor}; cursor:{listLoading
							? 'not-allowed'
							: 'pointer'}; background:{cardBg}; color:{textPrimary}; opacity:{listLoading
							? 0.5
							: 1};"
					>
						{findContext
							? m.resolve_ambiguous_objects_back()
							: listLoading
								? m.resolve_ambiguous_objects_refreshing()
								: m.resolve_ambiguous_objects_refresh()}
					</button>
				</div>
			</div>
		</div>

		<div class="flex-1 overflow-y-auto" style="min-height:0;">
			{#if listLoading}
				<div class="p-4" style="color:{textMuted}; font-size:13px;">
					{m.resolve_ambiguous_objects_loading()}
				</div>
			{:else if findLoading}
				<div class="p-4" style="color:{textMuted}; font-size:13px;">
					{m.resolve_ambiguous_objects_finding_linked_artifact_objects()}
				</div>
			{:else if listError}
				<div class="p-4" style="color:#F87171; font-size:13px;">
					{m.resolve_ambiguous_objects_error({ listError })}
				</div>
			{:else if findError}
				<div class="p-4" style="color:#F87171; font-size:13px;">
					{m.resolve_ambiguous_objects_error_2({ findError })}
				</div>
			{:else if findContext && findRows.length === 0}
				<div class="p-4" style="color:{textMuted}; font-size:13px;">
					{m.resolve_ambiguous_objects_no_artifact_objects_are_currently()}
				</div>
			{:else if !findContext && items.length === 0}
				<div class="p-4" style="color:{textMuted}; font-size:13px;">
					{m.resolve_ambiguous_objects_no_open_items()}
				</div>
			{:else if !findContext}
				{#each items as item (item.id)}
					<button
						type="button"
						onclick={() => clickCard(item.id)}
						class="w-full cursor-pointer p-3 text-left"
						style="
						border-bottom:1px solid {borderColor};
						background:{selectedItemId === item.id ? accentTint : 'transparent'};
					"
					>
						<div
							class="flex items-center gap-2"
							style="font-size:13px; font-weight:500; color:{selectedItemId === item.id
								? accent
								: textPrimary};"
						>
							<span>{item.title || item.dedupe_key}</span>
							{#if item.reopen_count > 0}
								<span
									style="font-size:10px; font-weight:600; padding:1px 6px; border-radius:9999px; background:rgba(248,113,113,0.15); color:#F87171;"
								>
									{m.resolve_ambiguous_objects_reopened({ count: item.reopen_count })}
								</span>
							{/if}
						</div>
						<div style="font-size:11px; color:{textMuted}; margin-top:2px;">
							{m.resolve_ambiguous_objects_item_meta({
								nodes: item.details.tied_candidates?.length ?? 0,
								count: item.occurrence_count,
								lastSeen: formatTime(item.last_seen)
							})}
						</div>
					</button>
				{/each}
			{:else}
				{#each findRows as row (row.id)}
					<button
						type="button"
						onclick={() => clickCard(row.id)}
						class="w-full cursor-pointer p-3 text-left"
						style="
						border-bottom:1px solid {borderColor};
						background:{selectedId === row.id ? accentTint : 'transparent'};
					"
					>
						<div
							style="font-size:13px; font-weight:500; color:{selectedId === row.id
								? accent
								: textPrimary};"
						>
							{row.object_name}{row.object_name_en ? ` (${row.object_name_en})` : ''}
						</div>
						<div style="font-size:11px; color:{textMuted}; margin-top:2px;">
							{leftRowMeta(row)}
						</div>
					</button>
				{/each}
			{/if}
		</div>
	</div>

	<!-- Right panel -->
	<div class="flex min-w-0 flex-1 flex-col overflow-hidden p-6">
		<div class="mb-4 flex shrink-0 items-center gap-2">
			<button
				type="button"
				onclick={goPrev}
				disabled={prevId === null || saving}
				style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary}; opacity:{prevId ===
					null || saving
					? 0.5
					: 1}; margin-left:auto;"
			>
				{m.resolve_ambiguous_objects_prev()}
			</button>
			<button
				type="button"
				onclick={goNext}
				disabled={nextId === null || saving}
				style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary}; opacity:{nextId ===
					null || saving
					? 0.5
					: 1};"
			>
				{m.resolve_ambiguous_objects_next()}
			</button>
			<button
				type="button"
				onclick={requestCancel}
				disabled={!isDirty || saving}
				style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary}; opacity:{isDirty &&
				!saving
					? 1
					: 0.5};"
			>
				{m.resolve_ambiguous_objects_cancel()}
			</button>
			<button
				type="button"
				onclick={doSave}
				disabled={!isDirty || saving}
				style="font-size:12px; font-weight:600; padding:6px 14px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white; opacity:{!isDirty ||
				saving
					? 0.5
					: 1};"
			>
				{saving ? m.resolve_ambiguous_objects_saving() : m.resolve_ambiguous_objects_save()}
			</button>
			<button
				type="button"
				onclick={() => (helpOpen = true)}
				style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textSecondary};"
			>
				{m.resolve_ambiguous_objects_help()}
			</button>
		</div>
		{#if saveError}
			<div class="mb-4" style="font-size:12px; color:#F87171;">
				{m.resolve_ambiguous_objects_save_failed({ saveError })}
			</div>
		{/if}
		{#if createNodeError}
			<div class="mb-4" style="font-size:12px; color:#F87171;">
				{m.resolve_ambiguous_objects_create_new_failed({ createNodeError })}
			</div>
		{/if}
		{#if selectedItem}
			<div
				class="mb-4 shrink-0 rounded-xl p-4"
				style="background:{cardBg}; border:1px solid {borderColor}; max-height:40%; overflow-y:auto;"
			>
				<div class="flex flex-wrap items-start justify-between gap-4">
					<div>
						<h2 style="font-size:14px; font-weight:600; color:{textPrimary};">
							{m.resolve_ambiguous_objects_item_heading({
								name: selectedItem.title || selectedItem.dedupe_key
							})}
						</h2>
						<p style="font-size:11px; color:{textMuted}; margin-top:2px;">
							{m.resolve_ambiguous_objects_item_summary({
								count: selectedItem.occurrence_count,
								firstSeen: formatTime(selectedItem.first_seen),
								lastSeen: formatTime(selectedItem.last_seen),
								llm: llmOutcomeLabel(selectedItem.details.llm_outcome)
							})}
						</p>
					</div>
					<div class="flex items-center gap-2">
						<input
							bind:value={closeNote}
							placeholder={m.resolve_ambiguous_objects_close_note_placeholder()}
							style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:12px; width:240px;"
						/>
						<button
							type="button"
							onclick={() => closeSelectedItem('resolve')}
							disabled={closing || isDirty || saving}
							title={isDirty
								? m.resolve_ambiguous_objects_save_or_cancel_first()
								: m.resolve_ambiguous_objects_resolve_hint()}
							style="font-size:12px; font-weight:600; padding:6px 12px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white; opacity:{closing ||
							isDirty ||
							saving
								? 0.5
								: 1};"
						>
							{m.resolve_ambiguous_objects_resolve_item()}
						</button>
						<button
							type="button"
							onclick={() => closeSelectedItem('dismiss')}
							disabled={closing || isDirty || saving}
							title={isDirty
								? m.resolve_ambiguous_objects_save_or_cancel_first()
								: m.resolve_ambiguous_objects_dismiss_hint()}
							style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary}; opacity:{closing ||
							isDirty ||
							saving
								? 0.5
								: 1};"
						>
							{m.resolve_ambiguous_objects_dismiss_item()}
						</button>
					</div>
				</div>
				{#if closeError}
					<div class="mt-2" style="font-size:12px; color:#F87171;">
						{m.resolve_ambiguous_objects_close_failed({ closeError })}
					</div>
				{/if}
				<div class="mt-3 grid grid-cols-1 gap-4 lg:grid-cols-2" style="font-size:12px;">
					<div>
						<div style="font-size:11px; color:{textMuted}; margin-bottom:4px;">
							{m.resolve_ambiguous_objects_tied_nodes_recorded()}
						</div>
						{#each selectedItem.details.tied_candidates ?? [] as cand (cand.object_id)}
							<div style="color:{textPrimary}; padding:2px 0;">
								<code>{cand.object_id}</code> · {cand.canonical_name} · {cand.object_type} · {cand.score}
								{#if cand.object_id === selectedItem.details.picked_object_id}
									<span
										style="font-size:10px; font-weight:600; padding:1px 6px; border-radius:9999px; background:{accentTint}; color:{accent};"
										>{m.resolve_ambiguous_objects_picked()}</span
									>
								{/if}
							</div>
						{/each}
					</div>
					<div>
						<div style="font-size:11px; color:{textMuted}; margin-bottom:4px;">
							{m.resolve_ambiguous_objects_occurrences()}
						</div>
						{#each selectedItem.details.occurrences ?? [] as occ (`${occ.input_record_id}|${occ.artifact_type}|${occ.artifact_id}|${occ.object_name}`)}
							<div style="color:{textPrimary}; padding:2px 0;">
								{m.resolve_ambiguous_objects_occurrence_row({
									record: occ.input_record_id,
									artifactType: occ.artifact_type,
									artifactId: occ.artifact_id,
									name: occ.object_name,
									seenAt: formatTime(occ.seen_at)
								})}
							</div>
						{/each}
					</div>
				</div>
				{#if (selectedItem.details.resolution_history ?? []).length > 0}
					<div class="mt-3" style="font-size:12px;">
						<div style="font-size:11px; color:#F87171; margin-bottom:4px;">
							{m.resolve_ambiguous_objects_reopened_history()}
						</div>
						{#each selectedItem.details.resolution_history ?? [] as entry, i (i)}
							<div style="color:{textSecondary}; padding:2px 0;">
								{m.resolve_ambiguous_objects_history_row({
									status: entry.status,
									by: entry.resolved_by ?? '',
									at: formatTime(entry.resolved_at),
									note: entry.resolution ?? '',
									reopenedAt: formatTime(entry.reopened_at)
								})}
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
		{#if detailLoading}
			<div style="color:{textMuted}; font-size:13px;">{m.resolve_ambiguous_objects_loading()}</div>
		{:else if detailError}
			<div style="color:#F87171; font-size:13px;">
				{m.resolve_ambiguous_objects_error_3({ detailError })}
			</div>
		{:else if itemNoRow}
			<div style="color:{textMuted}; font-size:13px;">
				{m.resolve_ambiguous_objects_no_current_row()}
			</div>
		{:else if !currentObject}
			<div style="color:{textMuted}; font-size:13px;">
				{m.resolve_ambiguous_objects_select_a_record_on_the()}
			</div>
		{:else}
			<div
				class="grid flex-1 grid-cols-1 gap-5 lg:grid-cols-2"
				style="align-items:stretch; min-height:0; grid-auto-rows:minmax(0, 1fr);"
			>
				<!-- Left column: Artifact Object -->
				<div
					class="min-h-0 overflow-y-auto rounded-xl p-5"
					style="background:{cardBg}; border:1px solid {borderColor};"
				>
					<div class="mb-4 flex items-center justify-between">
						<h2 style="font-size:14px; font-weight:600; color:{textPrimary};">
							{m.resolve_ambiguous_objects_artifact_object()}
						</h2>
						<span style="font-size:11px; color:{textMuted};">
							{m.resolve_ambiguous_objects_id({
								id: currentObject.id,
								artifact_type: currentObject.artifact_type,
								artifact_id: currentObject.artifact_id
							})}
						</span>
					</div>
					<div class="grid grid-cols-2 gap-3">
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_object_name()}</span
							>
							<input
								bind:value={currentObject.object_name}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('object_name')
								)}"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_object_name_en()}</span
							>
							<input
								bind:value={currentObject.object_name_en}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('object_name_en')
								)}"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_object_name_zh()}</span
							>
							<input
								bind:value={currentObject.object_name_zh}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('object_name_zh')
								)}"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_language()}</span
							>
							<input
								bind:value={currentObject.language}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('language')
								)}"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_object_type()}</span
							>
							<input
								bind:value={currentObject.object_type}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('object_type')
								)}"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_object_role()}</span
							>
							<input
								bind:value={currentObject.object_role}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('object_role')
								)}"
							/>
						</label>
						<label class="col-span-2 flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_aliases_comma_separated()}</span
							>
							<input
								value={aliasesText(currentObject.aliases)}
								oninput={(e) => {
									if (currentObject)
										currentObject.aliases = parseAliasesText(
											(e.currentTarget as HTMLInputElement).value
										);
								}}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('aliases')
								)}"
							/>
						</label>
						<label class="col-span-2 flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_acronyms_comma_separated()}</span
							>
							<input
								value={aliasesText(currentObject.acronyms)}
								oninput={(e) => {
									if (currentObject)
										currentObject.acronyms = parseAliasesText(
											(e.currentTarget as HTMLInputElement).value
										);
								}}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('acronyms')
								)}"
							/>
						</label>
						<label class="col-span-2 flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_description()}</span
							>
							<textarea
								bind:value={currentObject.description}
								rows="2"
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('description')
								)}"
							></textarea>
						</label>
						<label class="col-span-2 flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_evidence_quote()}</span
							>
							<textarea
								bind:value={currentObject.evidence_quote}
								rows="2"
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('evidence_quote')
								)}"
							></textarea>
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_object_id()}</span
							>
							<input
								bind:value={currentObject.object_id}
								placeholder={m.resolve_ambiguous_objects_unresolved()}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('object_id')
								)}"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_reconcile_status()}</span
							>
							<select
								bind:value={currentObject.reconcile_status}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('reconcile_status')
								)}"
							>
								{#each RECONCILE_STATUS_OPTIONS as opt}
									<option value={opt}>{opt}</option>
								{/each}
							</select>
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};"
								>{m.resolve_ambiguous_objects_reconcile_confidence()}</span
							>
							<input
								type="number"
								min="0"
								max="1"
								step="0.01"
								bind:value={currentObject.reconcile_confidence}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
									objDirty('reconcile_confidence')
								)}"
							/>
						</label>
						<div class="flex items-end justify-end">
							<button
								type="button"
								onclick={createNew}
								disabled={saving || creatingNode}
								style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary}; opacity:{saving ||
								creatingNode
									? 0.5
									: 1};"
							>
								{creatingNode
									? m.resolve_ambiguous_objects_creating()
									: m.resolve_ambiguous_objects_create_new()}
							</button>
						</div>
					</div>
				</div>

				<!-- Right column: Top Region (quick reference) + Bottom Region (editable candidates) -->
				<div class="flex min-h-0 flex-col gap-5">
					<div
						class="shrink-0 rounded-xl p-5"
						style="background:{cardBg}; border:1px solid {borderColor};"
					>
						<h2 style="font-size:14px; font-weight:600; color:{textPrimary}; margin-bottom:12px;">
							{m.resolve_ambiguous_objects_candidates()}
						</h2>
						{#if currentNodes.length === 0}
							<div style="font-size:13px; color:{textMuted};">
								{m.resolve_ambiguous_objects_no_candidate_object_nodes_found()}
							</div>
						{:else}
							<table style="width:100%; border-collapse:collapse; font-size:12px;">
								<thead>
									<tr>
										<th
											style="text-align:left; padding:6px 8px; color:{textMuted}; font-weight:500; border-bottom:1px solid {borderColor};"
											>{m.resolve_ambiguous_objects_canonical_name()}</th
										>
										<th
											style="text-align:left; padding:6px 8px; color:{textMuted}; font-weight:500; border-bottom:1px solid {borderColor};"
											>{m.resolve_ambiguous_objects_description()}</th
										>
										<th
											style="text-align:right; padding:6px 8px; color:{textMuted}; font-weight:500; border-bottom:1px solid {borderColor};"
											>{m.resolve_ambiguous_objects_action()}</th
										>
									</tr>
								</thead>
								<tbody>
									{#each currentNodes as node (node.object_id)}
										<tr>
											<td
												style="padding:6px 8px; color:{textPrimary}; border-bottom:1px solid {borderColor}; vertical-align:top;"
											>
												{node.canonical_name}
												{#if node.recommended}
													<span
														style="display:inline-block; margin-left:6px; font-size:10px; font-weight:600; padding:2px 6px; border-radius:4px; background:{accentTint}; color:{accent};"
														>{m.resolve_ambiguous_objects_recommended()}</span
													>
												{/if}
											</td>
											<td
												style="padding:6px 8px; color:{textSecondary}; border-bottom:1px solid {borderColor}; max-width:260px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; vertical-align:top;"
											>
												{node.description}
											</td>
											<td
												style="padding:6px 8px; border-bottom:1px solid {borderColor}; text-align:right; vertical-align:top;"
											>
												<button
													type="button"
													onclick={() => useCandidate(node)}
													style="font-size:11px; font-weight:500; padding:4px 10px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;"
												>
													{m.resolve_ambiguous_objects_use_this()}
												</button>
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
						{/if}
					</div>

					<div
						class="flex min-h-0 flex-1 flex-col rounded-xl p-5"
						style="background:{cardBg}; border:1px solid {borderColor};"
					>
						<div class="flex shrink-0 items-center justify-between" style="margin-bottom:12px;">
							<h2 style="font-size:14px; font-weight:600; color:{textPrimary};">
								{m.resolve_ambiguous_objects_related_object_nodes()}
							</h2>
							<button
								type="button"
								onclick={requestMerge}
								disabled={!canMerge}
								title={m.resolve_ambiguous_objects_merge_the_selected_nodes_into()}
								style="font-size:12px; font-weight:600; padding:6px 14px; border-radius:6px; border:none; cursor:{canMerge
									? 'pointer'
									: 'not-allowed'}; background:{accent}; color:white; opacity:{canMerge ? 1 : 0.5};"
							>
								{merging
									? m.resolve_ambiguous_objects_merging()
									: m.resolve_ambiguous_objects_merge()}
							</button>
						</div>
						{#if mergeError}
							<div class="mb-3" style="font-size:12px; color:#F87171;">
								{m.resolve_ambiguous_objects_merge_failed({ mergeError })}
							</div>
						{/if}
						{#if currentNodes.length === 0}
							<div style="font-size:13px; color:{textMuted};">
								{m.resolve_ambiguous_objects_no_candidate_object_nodes_found()}
							</div>
						{:else}
							<div
								class="related-nodes-scroll"
								style="flex:1 1 0; min-height:0; overflow-y:auto; padding-right:4px;"
							>
								{#each currentNodes as node, i (node.object_id)}
									{@const matches = nodeMatchDetails(node)}
									<div
										class="mb-3 rounded-lg p-4"
										style="border:1px solid {borderColor}; background:{pageBg};"
									>
										<div class="mb-3 flex items-center justify-between">
											<div class="flex items-center gap-2">
												<span style="font-size:12px; color:{textMuted};"
													>{m.resolve_ambiguous_objects_id_2({ id: node.id })}</span
												>
												<span style="font-size:12px; font-family:monospace; color:{textSecondary};"
													>{node.object_id}</span
												>
												{#if node.recommended}
													<span
														style="font-size:10px; font-weight:600; padding:2px 6px; border-radius:4px; background:{accentTint}; color:{accent};"
														>{m.resolve_ambiguous_objects_recommended()}</span
													>
												{/if}
											</div>
											<div class="flex items-center gap-3">
												<button
													type="button"
													onclick={() => findLinkedArtifactObjects(node)}
													disabled={findLoading || rebinding}
													style="font-size:12px; font-weight:600; padding:4px 10px; border-radius:6px; border:1px solid {borderColor}; cursor:{findLoading ||
													rebinding
														? 'not-allowed'
														: 'pointer'}; background:{cardBg}; color:{textPrimary}; opacity:{findLoading ||
													rebinding
														? 0.5
														: 1};"
												>
													{m.resolve_ambiguous_objects_find()}
												</button>
												<label
													class="flex items-center gap-1"
													style="font-size:11px; color:{textSecondary}; cursor:pointer;"
												>
													<input
														type="checkbox"
														checked={masterNodeId === node.object_id}
														onchange={(e) =>
															setMaster(
																node.object_id,
																(e.currentTarget as HTMLInputElement).checked
															)}
													/>
													{m.resolve_ambiguous_objects_master()}
												</label>
												<label
													class="flex items-center gap-1"
													style="font-size:11px; color:{masterNodeId === node.object_id
														? textMuted
														: textSecondary}; cursor:{masterNodeId === node.object_id
														? 'not-allowed'
														: 'pointer'};"
												>
													<input
														type="checkbox"
														checked={selectedNodeIds.has(node.object_id)}
														disabled={masterNodeId === node.object_id}
														onchange={(e) =>
															setSelected(
																node.object_id,
																(e.currentTarget as HTMLInputElement).checked
															)}
													/>
													{m.resolve_ambiguous_objects_select()}
												</label>
												<span style="font-size:11px; color:{textMuted};"
													>{m.resolve_ambiguous_objects_score({
														score: node.score.toFixed(2),
														method: node.method
													})}</span
												>
											</div>
										</div>
										<div
											class="mb-3"
											style="font-size:11px; color:{matches.hasMatch ? '#34D399' : textMuted};"
										>
											{#if matches.hasMatch}
												{m.resolve_ambiguous_objects_matches({
													matches: matchesSummaryText(matches)
												})}
											{:else}
												{m.resolve_ambiguous_objects_matches_none()}
											{/if}
										</div>
										<div class="grid grid-cols-2 gap-3">
											<label class="flex flex-col gap-1">
												<span style="font-size:11px; color:{textMuted};"
													>{m.resolve_ambiguous_objects_canonical_name()}</span
												>
												<input
													bind:value={currentNodes[i].canonical_name}
													style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {matchedFieldStyle(
														nodeDirty(i, 'canonical_name'),
														matches,
														'canonical_name'
													)}"
												/>
											</label>
											<label class="flex flex-col gap-1">
												<span style="font-size:11px; color:{textMuted};"
													>{m.resolve_ambiguous_objects_object_type()}</span
												>
												<input
													bind:value={currentNodes[i].object_type}
													style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {matchedFieldStyle(
														nodeDirty(i, 'object_type'),
														matches,
														'object_type'
													)}"
												/>
											</label>
											<label class="flex flex-col gap-1">
												<span style="font-size:11px; color:{textMuted};"
													>{m.resolve_ambiguous_objects_canonical_name_en()}</span
												>
												<input
													bind:value={currentNodes[i].canonical_name_en}
													style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {matchedFieldStyle(
														nodeDirty(i, 'canonical_name_en'),
														matches,
														'canonical_name_en'
													)}"
												/>
											</label>
											<label class="flex flex-col gap-1">
												<span style="font-size:11px; color:{textMuted};"
													>{m.resolve_ambiguous_objects_canonical_name_zh()}</span
												>
												<input
													bind:value={currentNodes[i].canonical_name_zh}
													style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {matchedFieldStyle(
														nodeDirty(i, 'canonical_name_zh'),
														matches,
														'canonical_name_zh'
													)}"
												/>
											</label>
											<label class="flex flex-col gap-1">
												<span style="font-size:11px; color:{textMuted};"
													>{m.resolve_ambiguous_objects_primary_language()}</span
												>
												<input
													bind:value={currentNodes[i].primary_language}
													style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
														nodeDirty(i, 'primary_language')
													)}"
												/>
											</label>
											<label class="flex flex-col gap-1">
												<span style="font-size:11px; color:{textMuted};"
													>{m.resolve_ambiguous_objects_aliases_comma_separated()}</span
												>
												<input
													value={aliasesText(node.aliases)}
													oninput={(e) => {
														currentNodes[i].aliases = parseAliasesText(
															(e.currentTarget as HTMLInputElement).value
														);
													}}
													style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {matchedFieldStyle(
														nodeDirty(i, 'aliases'),
														matches,
														'aliases'
													)}"
												/>
											</label>
											<label class="flex flex-col gap-1">
												<span style="font-size:11px; color:{textMuted};"
													>{m.resolve_ambiguous_objects_acronyms_comma_separated()}</span
												>
												<input
													value={aliasesText(node.acronyms)}
													oninput={(e) => {
														currentNodes[i].acronyms = parseAliasesText(
															(e.currentTarget as HTMLInputElement).value
														);
													}}
													style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {matchedFieldStyle(
														nodeDirty(i, 'acronyms'),
														matches,
														'acronyms'
													)}"
												/>
											</label>
											<label class="flex flex-col gap-1">
												<span style="font-size:11px; color:{textMuted};"
													>{m.resolve_ambiguous_objects_normalized_names_derived()}</span
												>
												<input
													value={aliasesText(node.normalized_names)}
													readonly
													style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; opacity:0.9; {matchedFieldStyle(
														false,
														matches,
														'normalized_names'
													)}"
												/>
											</label>
											<label class="col-span-2 flex flex-col gap-1">
												<span style="font-size:11px; color:{textMuted};"
													>{m.resolve_ambiguous_objects_description()}</span
												>
												<textarea
													bind:value={currentNodes[i].description}
													rows="2"
													style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(
														nodeDirty(i, 'description')
													)}"
												></textarea>
											</label>
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				</div>
			</div>
		{/if}
	</div>

	{#if navConfirm}
		<div
			class="overlay"
			role="presentation"
			tabindex="-1"
			onclick={stayOnNav}
			onkeydown={(e) => {
				if (e.key === 'Escape') stayOnNav();
			}}
		>
			<div
				class="modal"
				role="dialog"
				aria-modal="true"
				tabindex="0"
				onclick={(e) => e.stopPropagation()}
				onkeydown={(e) => e.stopPropagation()}
			>
				<p style="font-size:13px; color:{textPrimary}; margin-bottom:16px;">
					{navConfirm.kind === 'prev'
						? m.resolve_ambiguous_objects_save_changes_before_moving_to()
						: navConfirm.kind === 'next'
							? m.resolve_ambiguous_objects_save_changes_before_moving_to_2()
							: m.resolve_ambiguous_objects_save_changes_before_switching_records()}
				</p>
				<div class="flex justify-end gap-2">
					<button
						type="button"
						onclick={stayOnNav}
						style="font-size:12px; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary};"
						>{m.resolve_ambiguous_objects_stay()}</button
					>
					<button
						type="button"
						onclick={confirmNavDiscard}
						style="font-size:12px; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary};"
						>{m.resolve_ambiguous_objects_discard_continue()}</button
					>
					<button
						type="button"
						onclick={confirmNavSave}
						style="font-size:12px; font-weight:600; padding:6px 12px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;"
						>{m.resolve_ambiguous_objects_save_continue()}</button
					>
				</div>
			</div>
		</div>
	{/if}

	{#if cancelConfirm}
		<div
			class="overlay"
			role="presentation"
			tabindex="-1"
			onclick={dismissCancel}
			onkeydown={(e) => {
				if (e.key === 'Escape') dismissCancel();
			}}
		>
			<div
				class="modal"
				role="dialog"
				aria-modal="true"
				tabindex="0"
				onclick={(e) => e.stopPropagation()}
				onkeydown={(e) => e.stopPropagation()}
			>
				<p style="font-size:13px; color:{textPrimary}; margin-bottom:16px;">
					{m.resolve_ambiguous_objects_discard_your_edits_to_this()}
				</p>
				<div class="flex justify-end gap-2">
					<button
						type="button"
						onclick={dismissCancel}
						style="font-size:12px; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary};"
						>{m.resolve_ambiguous_objects_keep_editing()}</button
					>
					<button
						type="button"
						onclick={confirmCancel}
						style="font-size:12px; font-weight:600; padding:6px 12px; border-radius:6px; border:none; cursor:pointer; background:#DC2626; color:white;"
						>{m.resolve_ambiguous_objects_discard()}</button
					>
				</div>
			</div>
		</div>
	{/if}

	{#if mergeConfirm}
		<div
			class="overlay"
			role="presentation"
			tabindex="-1"
			onclick={() => (mergeConfirm = false)}
			onkeydown={(e) => {
				if (e.key === 'Escape') mergeConfirm = false;
			}}
		>
			<div
				class="modal"
				role="dialog"
				aria-modal="true"
				tabindex="0"
				onclick={(e) => e.stopPropagation()}
				onkeydown={(e) => e.stopPropagation()}
			>
				<p style="font-size:13px; color:{textPrimary}; margin-bottom:8px;">
					{m.resolve_ambiguous_objects_merge_selected_object_node_into({
						selectedNodeIdsCount: selectedNodeIds.size,
						plural: selectedNodeIds.size === 1 ? '' : 's'
					})}
				</p>
				<p style="font-size:12px; color:{textSecondary}; margin-bottom:16px;">
					{m.resolve_ambiguous_objects_their_artifact_object_mentions_will()}
					<span style="font-family:monospace;">{masterNodeId}</span
					>{m.resolve_ambiguous_objects_and_the_merged_nodes_will()}
				</p>
				<div class="flex justify-end gap-2">
					<button
						type="button"
						onclick={() => (mergeConfirm = false)}
						style="font-size:12px; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary};"
						>{m.resolve_ambiguous_objects_cancel()}</button
					>
					<button
						type="button"
						onclick={confirmMerge}
						style="font-size:12px; font-weight:600; padding:6px 12px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;"
						>{m.resolve_ambiguous_objects_merge()}</button
					>
				</div>
			</div>
		</div>
	{/if}

	{#if existsNotice}
		<div
			class="overlay"
			role="presentation"
			tabindex="-1"
			onclick={() => (existsNotice = '')}
			onkeydown={(e) => {
				if (e.key === 'Escape') existsNotice = '';
			}}
		>
			<div
				class="modal"
				role="dialog"
				aria-modal="true"
				tabindex="0"
				onclick={(e) => e.stopPropagation()}
				onkeydown={(e) => e.stopPropagation()}
			>
				<p style="font-size:13px; color:{textPrimary}; margin-bottom:16px;">{existsNotice}</p>
				<div class="flex justify-end">
					<button
						type="button"
						onclick={() => (existsNotice = '')}
						style="font-size:12px; font-weight:600; padding:6px 12px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;"
						>{m.resolve_ambiguous_objects_ok()}</button
					>
				</div>
			</div>
		</div>
	{/if}

	{#if rebindNotice}
		<div
			class="overlay"
			role="presentation"
			tabindex="-1"
			onclick={() => (rebindNotice = '')}
			onkeydown={(e) => {
				if (e.key === 'Escape') rebindNotice = '';
			}}
		>
			<div
				class="modal"
				role="dialog"
				aria-modal="true"
				tabindex="0"
				onclick={(e) => e.stopPropagation()}
				onkeydown={(e) => e.stopPropagation()}
			>
				<p style="font-size:13px; color:{textPrimary}; margin-bottom:16px;">{rebindNotice}</p>
				<div class="flex justify-end">
					<button
						type="button"
						onclick={() => (rebindNotice = '')}
						style="font-size:12px; font-weight:600; padding:6px 12px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;"
						>{m.resolve_ambiguous_objects_ok()}</button
					>
				</div>
			</div>
		</div>
	{/if}

	{#if helpOpen}
		<div
			class="overlay"
			role="presentation"
			tabindex="-1"
			onclick={() => (helpOpen = false)}
			onkeydown={(e) => {
				if (e.key === 'Escape') helpOpen = false;
			}}
		>
			<div
				class="modal"
				role="dialog"
				aria-modal="true"
				tabindex="0"
				onclick={(e) => e.stopPropagation()}
				onkeydown={(e) => e.stopPropagation()}
				style="width:min(560px, 100%);"
			>
				<h3 style="font-size:14px; font-weight:600; color:{textPrimary}; margin-bottom:10px;">
					{m.resolve_ambiguous_objects_about_this_page()}
				</h3>
				<div style="font-size:12px; color:{textSecondary}; line-height:1.7;">
					<p style="margin-bottom:10px;">
						{m.resolve_ambiguous_objects_help_items()}
					</p>
					<p style="margin-bottom:10px;">
						{m.resolve_ambiguous_objects_help_close()}
					</p>
					<p style="margin-bottom:10px;">
						{m.resolve_ambiguous_objects_the()}
						<strong>{m.resolve_ambiguous_objects_recommended()}</strong>
						{m.resolve_ambiguous_objects_badge_marks_the_same_deterministic()}
						<strong>{m.resolve_ambiguous_objects_use_this()}</strong>
						{m.resolve_ambiguous_objects_on_any_candidate_to_copy()}
						<code>ambiguous_resolved</code>.
					</p>
					<p style="margin-bottom:10px;">
						<strong>{m.resolve_ambiguous_objects_reconcile_status_2()}</strong>
						{m.resolve_ambiguous_objects_values()} <code>pending</code>
						{m.resolve_ambiguous_objects_not_yet_attempted()}
						<code>ambiguous</code>
						{m.resolve_ambiguous_objects_still_tied()} <code>ambiguous_resolved</code>
						{m.resolve_ambiguous_objects_tie_broken_by_a_human()} <code>matched</code>
						{m.resolve_ambiguous_objects_confident_automatic_match()}
						<code>new</code>
						{m.resolve_ambiguous_objects_a_new_object_node_was()} <code>rejected</code>
						{m.resolve_ambiguous_objects_no_valid_object_leave_object()}
					</p>
					<p>
						<strong>{m.resolve_ambiguous_objects_save()}</strong>
						{m.resolve_ambiguous_objects_help_save()}
					</p>
				</div>
				<div class="mt-4 flex justify-end">
					<button
						type="button"
						onclick={() => (helpOpen = false)}
						style="font-size:12px; font-weight:600; padding:6px 14px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;"
						>{m.resolve_ambiguous_objects_close()}</button
					>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1.5rem;
		background: rgba(2, 6, 23, 0.62);
		z-index: 30;
	}

	.modal {
		width: min(420px, 100%);
		border-radius: 12px;
		border: 1px solid rgba(148, 163, 184, 0.16);
		background: #1f2333;
		padding: 1.25rem;
	}
</style>
