<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listAmbiguousObjects,
		getAmbiguousObjectDetail,
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
					onclick={() => selectRow(row.id)}
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
</div>
