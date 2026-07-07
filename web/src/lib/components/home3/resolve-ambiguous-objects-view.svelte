<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listAmbiguousObjects,
		getAmbiguousObjectDetail,
		updateArtifactObject,
		updateObjectNode,
		buildArtifactObjectPatch,
		buildObjectNodePatch,
		neighborAmbiguousId,
		RECONCILE_STATUS_OPTIONS,
		type AmbiguousObjectSummary,
		type ArtifactObjectDetail,
		type ObjectNodeCandidate
	} from './resolve-ambiguous-objects-client.js';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Design tokens (matches db-consistency-view.svelte / db-maint-log-view.svelte) ---
	let pageBg        = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg        = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let borderColor   = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent        = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint    = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');

	// --- Left panel state ---
	let rows        = $state<AmbiguousObjectSummary[]>([]);
	let listLoading = $state(false);
	let listError   = $state('');

	// --- Right panel state ---
	let selectedId     = $state<number | null>(null);
	let detailLoading  = $state(false);
	let detailError    = $state('');
	let snapshotObject = $state<ArtifactObjectDetail | null>(null);
	let currentObject  = $state<ArtifactObjectDetail | null>(null);
	let snapshotNodes  = $state<ObjectNodeCandidate[]>([]);
	let currentNodes   = $state<ObjectNodeCandidate[]>([]);

	let saving      = $state(false);
	let saveError   = $state('');
	let helpOpen    = $state(false);

	let navConfirm      = $state<{ kind: 'prev' | 'next' | 'switch' } | null>(null);
	let pendingSelectId = $state<number | null>(null);
	let cancelConfirm   = $state(false);

	let isDirty = $derived.by(() => {
		if (!snapshotObject || !currentObject) return false;
		if (JSON.stringify(snapshotObject) !== JSON.stringify(currentObject)) return true;
		return JSON.stringify(snapshotNodes) !== JSON.stringify(currentNodes);
	});

	// Derived (not inlined in the template) so `selectedId`'s `number | null`
	// type is narrowed once here, rather than needing a `number` at every
	// neighborAmbiguousId call site in the markup.
	let prevId = $derived.by(() =>
		selectedId === null ? null : neighborAmbiguousId(rows.map((r) => r.id), selectedId, -1)
	);
	let nextId = $derived.by(() =>
		selectedId === null ? null : neighborAmbiguousId(rows.map((r) => r.id), selectedId, 1)
	);

	function requestNav(kind: 'prev' | 'next' | 'switch', id: number) {
		if (isDirty) {
			navConfirm = { kind };
			pendingSelectId = id;
			return;
		}
		selectRow(id);
	}

	function goPrev() {
		if (prevId !== null) requestNav('prev', prevId);
	}

	function goNext() {
		if (nextId !== null) requestNav('next', nextId);
	}

	function clickCard(id: number) {
		if (id === selectedId) return;
		requestNav('switch', id);
	}

	async function confirmNavSave() {
		const target = pendingSelectId;
		navConfirm = null;
		pendingSelectId = null;
		await doSave();
		if (target !== null) await selectRow(target);
	}

	function confirmNavDiscard() {
		const target = pendingSelectId;
		navConfirm = null;
		pendingSelectId = null;
		if (target !== null) selectRow(target);
	}

	function stayOnNav() {
		navConfirm = null;
		pendingSelectId = null;
	}

	function requestCancel() {
		if (!isDirty) return;
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

	async function doSave() {
		if (!currentObject || !snapshotObject || selectedId === null) return;
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
			const resolved = currentObject.reconcile_status !== 'ambiguous';
			snapshotObject = { ...currentObject };
			snapshotNodes = currentNodes.map((c) => ({ ...c }));
			if (resolved) {
				const resolvedId = selectedId;
				rows = rows.filter((r) => r.id !== resolvedId);
				if (rows.length > 0) {
					await selectRow(rows[0].id);
				} else {
					selectedId = null;
					currentObject = null;
					snapshotObject = null;
					currentNodes = [];
					snapshotNodes = [];
				}
			}
		} catch (e) {
			saveError = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}

	async function loadList() {
		listLoading = true;
		listError = '';
		try {
			const res = await listAmbiguousObjects();
			rows = res.rows;
			if (rows.length > 0 && selectedId === null) {
				await selectRow(rows[0].id);
			}
		} catch (e) {
			listError = e instanceof Error ? e.message : String(e);
		} finally {
			listLoading = false;
		}
	}

	async function selectRow(id: number) {
		selectedId = id;
		detailLoading = true;
		detailError = '';
		saveError = '';
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

	function useCandidate(objectId: string) {
		if (!currentObject) return;
		currentObject.object_id = objectId;
	}

	function aliasesText(values: string[]): string {
		return values.join(', ');
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

<div class="flex" style="height:100%; min-height:100%; background:{pageBg};">
	<!-- Left panel -->
	<div
		class="flex-shrink-0 overflow-y-auto"
		style="width:320px; border-right:1px solid {borderColor};"
	>
		<div class="p-4" style="border-bottom:1px solid {borderColor};">
			<h1 style="font-size:16px; font-weight:600; color:{textPrimary}; margin-bottom:2px;">
				Resolve Ambiguous Objects
			</h1>
			<p style="font-size:12px; color:{textSecondary};">
				{rows.length} row{rows.length === 1 ? '' : 's'} at reconcile_status = ambiguous
			</p>
		</div>

		{#if listLoading}
			<div class="p-4" style="color:{textMuted}; font-size:13px;">Loading…</div>
		{:else if listError}
			<div class="p-4" style="color:#F87171; font-size:13px;">Error: {listError}</div>
		{:else if rows.length === 0}
			<div class="p-4" style="color:{textMuted}; font-size:13px;">
				No ambiguous objects — the queue is empty.
			</div>
		{:else}
			{#each rows as row (row.id)}
				<button
					type="button"
					onclick={() => clickCard(row.id)}
					class="w-full text-left p-3 cursor-pointer"
					style="
						border-bottom:1px solid {borderColor};
						background:{selectedId === row.id ? accentTint : 'transparent'};
					"
				>
					<div style="font-size:13px; font-weight:500; color:{selectedId === row.id ? accent : textPrimary};">
						{row.object_name}{row.object_name_en ? ` (${row.object_name_en})` : ''}
					</div>
					<div style="font-size:11px; color:{textMuted}; margin-top:2px;">
						{row.artifact_type} · confidence {row.confidence.toFixed(2)}
					</div>
				</button>
			{/each}
		{/if}
	</div>

	<!-- Right panel -->
	<div class="flex-1 overflow-y-auto p-6">
		<div class="flex items-center gap-2 mb-4">
			<button
				type="button"
				onclick={goPrev}
				disabled={prevId === null}
				style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary}; opacity:{prevId === null ? 0.5 : 1};"
			>
				Prev
			</button>
			<button
				type="button"
				onclick={goNext}
				disabled={nextId === null}
				style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary}; opacity:{nextId === null ? 0.5 : 1};"
			>
				Next
			</button>
			<button
				type="button"
				onclick={requestCancel}
				disabled={!isDirty}
				style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary}; opacity:{isDirty ? 1 : 0.5};"
			>
				Cancel
			</button>
			<button
				type="button"
				onclick={doSave}
				disabled={!isDirty || saving}
				style="font-size:12px; font-weight:600; padding:6px 14px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white; opacity:{!isDirty || saving ? 0.5 : 1};"
			>
				{saving ? 'Saving…' : 'Save'}
			</button>
			<button
				type="button"
				onclick={() => (helpOpen = true)}
				style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textSecondary}; margin-left:auto;"
			>
				Help
			</button>
		</div>
		{#if saveError}
			<div class="mb-4" style="font-size:12px; color:#F87171;">Save failed: {saveError}</div>
		{/if}
		{#if detailLoading}
			<div style="color:{textMuted}; font-size:13px;">Loading…</div>
		{:else if detailError}
			<div style="color:#F87171; font-size:13px;">Error: {detailError}</div>
		{:else if !currentObject}
			<div style="color:{textMuted}; font-size:13px;">Select a record on the left to resolve it.</div>
		{:else}
			<!-- Artifact Object block -->
			<div class="rounded-xl p-5 mb-5" style="background:{cardBg}; border:1px solid {borderColor};">
				<div class="flex items-center justify-between mb-4">
					<h2 style="font-size:14px; font-weight:600; color:{textPrimary};">Artifact Object</h2>
					<span style="font-size:11px; color:{textMuted};">
						id {currentObject.id} · {currentObject.artifact_type} · {currentObject.artifact_id}
					</span>
				</div>
				<div class="grid grid-cols-2 gap-3">
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Object Name</span>
						<input bind:value={currentObject.object_name} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Object Name (EN)</span>
						<input bind:value={currentObject.object_name_en} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Object Name (ZH)</span>
						<input bind:value={currentObject.object_name_zh} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Language</span>
						<input bind:value={currentObject.language} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Object Type</span>
						<input bind:value={currentObject.object_type} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Object Role</span>
						<input bind:value={currentObject.object_role} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1 col-span-2">
						<span style="font-size:11px; color:{textMuted};">Aliases (comma-separated)</span>
						<input
							value={aliasesText(currentObject.aliases)}
							oninput={(e) => { if (currentObject) currentObject.aliases = parseAliasesText((e.currentTarget as HTMLInputElement).value); }}
							style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"
						/>
					</label>
					<label class="flex flex-col gap-1 col-span-2">
						<span style="font-size:11px; color:{textMuted};">Acronyms (comma-separated)</span>
						<input
							value={aliasesText(currentObject.acronyms)}
							oninput={(e) => { if (currentObject) currentObject.acronyms = parseAliasesText((e.currentTarget as HTMLInputElement).value); }}
							style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"
						/>
					</label>
					<label class="flex flex-col gap-1 col-span-2">
						<span style="font-size:11px; color:{textMuted};">Description</span>
						<textarea bind:value={currentObject.description} rows="2" style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"></textarea>
					</label>
					<label class="flex flex-col gap-1 col-span-2">
						<span style="font-size:11px; color:{textMuted};">Evidence Quote</span>
						<textarea bind:value={currentObject.evidence_quote} rows="2" style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"></textarea>
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Object ID</span>
						<input bind:value={currentObject.object_id} placeholder="(unresolved)" style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Reconcile Status</span>
						<select bind:value={currentObject.reconcile_status} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;">
							{#each RECONCILE_STATUS_OPTIONS as opt}
								<option value={opt}>{opt}</option>
							{/each}
						</select>
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Reconcile Confidence</span>
						<input type="number" min="0" max="1" step="0.01" bind:value={currentObject.reconcile_confidence} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
				</div>
			</div>

			<!-- Related Object Nodes block -->
			<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
				<h2 style="font-size:14px; font-weight:600; color:{textPrimary}; margin-bottom:12px;">
					Related Object Nodes
				</h2>
				{#if currentNodes.length === 0}
					<div style="font-size:13px; color:{textMuted};">No candidate object nodes found for this artifact object.</div>
				{/if}
				{#each currentNodes as node, i (node.object_id)}
					<div class="rounded-lg p-4 mb-3" style="border:1px solid {borderColor}; background:{pageBg};">
						<div class="flex items-center justify-between mb-3">
							<div class="flex items-center gap-2">
								<span style="font-size:12px; font-family:monospace; color:{textSecondary};">{node.object_id}</span>
								{#if node.recommended}
									<span style="font-size:10px; font-weight:600; padding:2px 6px; border-radius:4px; background:{accentTint}; color:{accent};">Recommended</span>
								{/if}
							</div>
							<div class="flex items-center gap-3">
								<span style="font-size:11px; color:{textMuted};">score {node.score.toFixed(2)} · {node.method}</span>
								<button
									type="button"
									onclick={() => useCandidate(node.object_id)}
									style="font-size:11px; font-weight:500; padding:4px 10px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;"
								>
									Use this
								</button>
							</div>
						</div>
						<div class="grid grid-cols-2 gap-3">
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Canonical Name</span>
								<input bind:value={currentNodes[i].canonical_name} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Object Type</span>
								<input bind:value={currentNodes[i].object_type} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Canonical Name (EN)</span>
								<input bind:value={currentNodes[i].canonical_name_en} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Canonical Name (ZH)</span>
								<input bind:value={currentNodes[i].canonical_name_zh} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Primary Language</span>
								<input bind:value={currentNodes[i].primary_language} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Aliases (comma-separated)</span>
								<input
									value={aliasesText(node.aliases)}
									oninput={(e) => { currentNodes[i].aliases = parseAliasesText((e.currentTarget as HTMLInputElement).value); }}
									style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"
								/>
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Acronyms (comma-separated)</span>
								<input
									value={aliasesText(node.acronyms)}
									oninput={(e) => { currentNodes[i].acronyms = parseAliasesText((e.currentTarget as HTMLInputElement).value); }}
									style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"
								/>
							</label>
							<label class="flex flex-col gap-1 col-span-2">
								<span style="font-size:11px; color:{textMuted};">Description</span>
								<textarea bind:value={currentNodes[i].description} rows="2" style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"></textarea>
							</label>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>

	{#if navConfirm}
		<div
			class="overlay"
			role="presentation"
			tabindex="-1"
			onclick={stayOnNav}
			onkeydown={(e) => { if (e.key === 'Escape') stayOnNav(); }}
		>
			<div class="modal" role="dialog" aria-modal="true" tabindex="0" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
				<p style="font-size:13px; color:{textPrimary}; margin-bottom:16px;">
					{navConfirm.kind === 'prev'
						? 'Save changes before moving to the previous record?'
						: navConfirm.kind === 'next'
							? 'Save changes before moving to the next record?'
							: 'Save changes before switching records?'}
				</p>
				<div class="flex justify-end gap-2">
					<button type="button" onclick={stayOnNav} style="font-size:12px; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary};">Stay</button>
					<button type="button" onclick={confirmNavDiscard} style="font-size:12px; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary};">Discard &amp; Continue</button>
					<button type="button" onclick={confirmNavSave} style="font-size:12px; font-weight:600; padding:6px 12px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;">Save &amp; Continue</button>
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
			onkeydown={(e) => { if (e.key === 'Escape') dismissCancel(); }}
		>
			<div class="modal" role="dialog" aria-modal="true" tabindex="0" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
				<p style="font-size:13px; color:{textPrimary}; margin-bottom:16px;">Discard your edits to this record?</p>
				<div class="flex justify-end gap-2">
					<button type="button" onclick={dismissCancel} style="font-size:12px; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary};">Keep Editing</button>
					<button type="button" onclick={confirmCancel} style="font-size:12px; font-weight:600; padding:6px 12px; border-radius:6px; border:none; cursor:pointer; background:#DC2626; color:white;">Discard</button>
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
			onkeydown={(e) => { if (e.key === 'Escape') helpOpen = false; }}
		>
			<div class="modal" role="dialog" aria-modal="true" tabindex="0" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} style="width:min(560px, 100%);">
				<h3 style="font-size:14px; font-weight:600; color:{textPrimary}; margin-bottom:10px;">About This Page</h3>
				<div style="font-size:12px; color:{textSecondary}; line-height:1.7;">
					<p style="margin-bottom:10px;">
						A row is <strong>ambiguous</strong> when reconciliation found two or more equally-scored
						candidate object nodes and could not pick one automatically. This page lets you review
						the artifact object and its candidates side by side and resolve the tie by hand.
					</p>
					<p style="margin-bottom:10px;">
						The <strong>Recommended</strong> badge marks the same deterministic tie-break pick used by
						the automated backfill (most shared normalized names, falling back to the
						lexicographically smallest object_id). Click <strong>Use this</strong> on any candidate to
						copy its object_id into the Object ID field above.
					</p>
					<p style="margin-bottom:10px;">
						<strong>reconcile_status</strong> values: <code>pending</code> (not yet attempted),
						<code>ambiguous</code> (still tied), <code>ambiguous_resolved</code> (tie broken by a
						human or the automated tie-break), <code>matched</code> (confident automatic match),
						<code>new</code> (a new object node was created), <code>rejected</code> (no valid object
						— leave object_id empty).
					</p>
					<p>
						<strong>Save</strong> writes your edits to both the artifact object and any edited
						candidate nodes. Once you set an object_id and a non-<code>ambiguous</code> status, the
						row leaves this queue.
					</p>
				</div>
				<div class="flex justify-end mt-4">
					<button type="button" onclick={() => (helpOpen = false)} style="font-size:12px; font-weight:600; padding:6px 14px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;">Close</button>
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
