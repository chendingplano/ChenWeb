<script lang="ts">
	// Task group 7: wires BlockList's local-only editing to the CDM HTTP API
	// -- save, publish, and on-demand preview -- closing the authoring loop
	// design.md calls out as this change's actual goal (create -> edit ->
	// save -> publish -> preview).
	//
	// initialDocument is a one-time seed, not a bindable prop: this component
	// owns the document from here on (its own content_version moves forward
	// on every successful save/publish), and the caller's job ends at
	// loading it. structuredClone matches the existing project convention
	// (inputs-mgmt-view.svelte, kb-input-metadata.js) for taking a private
	// mutable copy of data that came from elsewhere.
	import BlockList from './BlockList.svelte';
	import type { Document } from './types.js';
	import {
		saveDocument,
		publishDocument,
		renderDocument,
		getDocument,
		CdmApiError,
		CdmStaleVersionError,
		CdmFrozenError,
		CdmValidationError,
		CdmBlockConflictError
	} from './cdm-client.js';
	import { attributeToBlocks, type BlockAttribution } from './document-editor-ops.js';

	let { initialDocument }: { initialDocument: Document } = $props();

	let doc = $state(structuredClone(initialDocument));

	// dirty compares against a JSON snapshot taken at load/save time rather
	// than a dedicated dirty flag flipped by every mutation: BlockList (and
	// everything under it) mutates `doc.blocks` through many different
	// entry points -- block-ops, table-ops, list-ops, InlineEditor's
	// onUpdate, plain oninput handlers -- and none of them report back to
	// this component today. A snapshot compare is correct regardless of
	// which of those paths changed something, at the cost of a stringify on
	// every access; documents at this MVP's scale make that cost
	// unmeasurable.
	let savedSnapshot = $state(JSON.stringify(initialDocument));
	let dirty = $derived(JSON.stringify(doc) !== savedSnapshot);

	let saving = $state(false);
	let publishing = $state(false);
	let previewing = $state(false);

	let staleError = $state<CdmStaleVersionError | null>(null);
	// Once set, this document is published (D8): every mutating BlockList
	// control is hidden (editable={!frozenMessage} below) and Save/Publish
	// are disabled. There is deliberately no action offered to open a new
	// version -- design.md D4 defers that; the requirement here is only
	// that the state is explained, not dead-ended.
	let frozenMessage = $state<string | null>(null);
	let genericError = $state<string | null>(null);
	let attributions = $state<BlockAttribution[]>([]);
	let savedNote = $state<string | null>(null);

	let previewPages = $state<string[] | null>(null);
	let previewVersion = $state<number | null>(null);
	let previewError = $state<string | null>(null);

	// Violations/conflicts with no resolvable block id (e.g. a block missing
	// its own id, which per extractBlockId's doc comment has nothing to
	// quote) are shown as a plain list rather than silently dropped.
	let unattributed = $derived(attributions.filter((a) => a.blockId === null).map((a) => a.message));
	let blockErrors = $derived.by(() => {
		const map = new Map<string, string[]>();
		for (const a of attributions) {
			if (a.blockId === null) continue;
			const list = map.get(a.blockId) ?? [];
			list.push(a.message);
			map.set(a.blockId, list);
		}
		return map;
	});

	function clearSaveFeedback() {
		staleError = null;
		genericError = null;
		attributions = [];
		savedNote = null;
	}

	async function save() {
		if (frozenMessage) return;
		saving = true;
		clearSaveFeedback();
		try {
			const result = await saveDocument(doc);
			// Only content_version is adopted from the response, not the whole
			// document: doc.blocks here is exactly what was just sent (already
			// validated), and wholesale-replacing it risks clobbering an edit
			// made during the request's round trip.
			doc.content_version = result.content_version;
			savedSnapshot = JSON.stringify(doc);
			savedNote = `Saved as version ${result.content_version}.`;
		} catch (e) {
			if (e instanceof CdmStaleVersionError) {
				// Task 7.2: the author's local content is untouched here --
				// only reloadDiscardingLocalChanges, an explicit separate
				// action below, replaces it.
				staleError = e;
			} else if (e instanceof CdmFrozenError) {
				frozenMessage = e.message;
			} else if (e instanceof CdmValidationError) {
				attributions = attributeToBlocks(e.violations);
				genericError = 'This document has validation problems (see below).';
			} else if (e instanceof CdmBlockConflictError) {
				attributions = attributeToBlocks([e.message]);
			} else if (e instanceof CdmApiError) {
				genericError = e.message;
			} else {
				genericError = String(e);
			}
		} finally {
			saving = false;
		}
	}

	async function reloadDiscardingLocalChanges() {
		const fresh = await getDocument(doc.document_key);
		doc = fresh;
		savedSnapshot = JSON.stringify(fresh);
		clearSaveFeedback();
	}

	async function publish() {
		if (frozenMessage || dirty) return;
		if (!window.confirm('Publishing freezes this document: it becomes read-only. Continue?')) {
			return;
		}
		publishing = true;
		genericError = null;
		try {
			const result = await publishDocument(doc.document_key);
			doc.content_version = result.content_version;
			savedSnapshot = JSON.stringify(doc);
			frozenMessage =
				'This document is now published and read-only. Opening a new version to keep editing is not supported yet.';
		} catch (e) {
			if (e instanceof CdmStaleVersionError) {
				staleError = e;
			} else if (e instanceof CdmFrozenError) {
				frozenMessage = e.message;
			} else if (e instanceof CdmApiError) {
				genericError = e.message;
			} else {
				genericError = String(e);
			}
		} finally {
			publishing = false;
		}
	}

	async function preview() {
		// Task 7.6: an explicit action only -- never called from an $effect
		// tracking `doc`, which would fire on every keystroke. Renders
		// whatever content_version is currently saved server-side (design
		// D9); if `dirty`, that is deliberately not what's on screen, and the
		// panel below says so rather than silently showing stale content as
		// if it were current.
		previewing = true;
		previewError = null;
		try {
			const result = await renderDocument(doc.document_key);
			previewPages = result.pages;
			previewVersion = result.content_version;
		} catch (e) {
			previewError = e instanceof CdmApiError ? e.message : String(e);
		} finally {
			previewing = false;
		}
	}

	function closePreview() {
		previewPages = null;
		previewVersion = null;
	}
</script>

<div class="cdm-document-editor">
	<header class="cdm-editor-header">
		<input
			class="cdm-title-input"
			type="text"
			bind:value={doc.title}
			disabled={!!frozenMessage}
			placeholder="Document title"
		/>
		<span class="cdm-version-badge">v{doc.content_version}{dirty ? ' · unsaved changes' : ''}</span>
		<div class="cdm-editor-actions">
			<button type="button" onclick={preview} disabled={previewing}>
				{previewing ? 'Rendering…' : 'Preview'}
			</button>
			<button type="button" onclick={save} disabled={saving || !!frozenMessage}>
				{saving ? 'Saving…' : 'Save'}
			</button>
			<button
				type="button"
				onclick={publish}
				disabled={publishing || !!frozenMessage || dirty}
				title={dirty ? 'Save your changes before publishing' : undefined}
			>
				{publishing ? 'Publishing…' : 'Publish'}
			</button>
		</div>
	</header>

	{#if savedNote}
		<div class="cdm-banner cdm-banner--ok">{savedNote}</div>
	{/if}

	{#if staleError}
		<div class="cdm-banner cdm-banner--warn">
			<p>
				Someone else saved a newer version (v{staleError.currentVersion}) while you were editing.
				Your changes here have not been discarded or sent.
			</p>
			<button type="button" onclick={reloadDiscardingLocalChanges}>
				Discard my changes and reload v{staleError.currentVersion}
			</button>
		</div>
	{/if}

	{#if frozenMessage}
		<div class="cdm-banner cdm-banner--frozen">{frozenMessage}</div>
	{/if}

	{#if genericError}
		<div class="cdm-banner cdm-banner--error">{genericError}</div>
	{/if}

	{#if unattributed.length}
		<div class="cdm-banner cdm-banner--error">
			{#each unattributed as msg (msg)}
				<p>{msg}</p>
			{/each}
		</div>
	{/if}

	<BlockList bind:blocks={doc.blocks} editable={!frozenMessage} {blockErrors} />

	{#if previewError}
		<div class="cdm-banner cdm-banner--error">{previewError}</div>
	{/if}

	{#if previewPages}
		<div class="cdm-preview-overlay">
			<div class="cdm-preview-panel">
				<div class="cdm-preview-panel-header">
					<span>
						Preview — version {previewVersion}
						{#if dirty}(unsaved changes exist; save to update this preview){/if}
					</span>
					<button type="button" onclick={closePreview}>Close</button>
				</div>
				<div class="cdm-preview-pages">
					{#each previewPages as pageSvg, i (i)}
						<div class="cdm-preview-page">{@html pageSvg}</div>
					{/each}
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.cdm-document-editor {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.cdm-editor-header {
		display: flex;
		align-items: center;
		gap: 10px;
		position: sticky;
		top: 0;
		background: var(--cdm-surface, #fff);
		padding: 8px 0;
		z-index: 5;
	}
	.cdm-title-input {
		flex: 1 1 auto;
		font-size: 1.1em;
		font-weight: 600;
		border: 1px solid rgba(127, 127, 127, 0.35);
		border-radius: 6px;
		padding: 4px 8px;
	}
	.cdm-version-badge {
		font-family: ui-monospace, monospace;
		font-size: 0.8em;
		color: var(--cdm-muted, #6b7280);
		white-space: nowrap;
	}
	.cdm-editor-actions {
		display: flex;
		gap: 6px;
	}
	.cdm-banner {
		border-radius: 6px;
		padding: 8px 12px;
		font-size: 0.9em;
	}
	.cdm-banner p {
		margin: 0 0 6px;
	}
	.cdm-banner--ok {
		background: rgba(5, 150, 105, 0.1);
		border: 1px solid rgba(5, 150, 105, 0.4);
	}
	.cdm-banner--warn {
		background: rgba(217, 119, 6, 0.1);
		border: 1px solid rgba(217, 119, 6, 0.4);
	}
	.cdm-banner--frozen {
		background: rgba(107, 114, 128, 0.12);
		border: 1px solid rgba(107, 114, 128, 0.4);
	}
	.cdm-banner--error {
		background: rgba(220, 38, 38, 0.08);
		border: 1px solid rgba(220, 38, 38, 0.4);
		color: #b91c1c;
	}
	.cdm-preview-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 50;
	}
	.cdm-preview-panel {
		background: var(--cdm-surface, #fff);
		width: min(900px, 92vw);
		height: min(85vh, 1100px);
		border-radius: 8px;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.cdm-preview-panel-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 10px 14px;
		border-bottom: 1px solid rgba(127, 127, 127, 0.3);
		font-size: 0.85em;
	}
	.cdm-preview-pages {
		flex: 1 1 auto;
		overflow-y: auto;
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 16px;
		align-items: center;
	}
	.cdm-preview-page :global(svg) {
		max-width: 100%;
		box-shadow: 0 1px 6px rgba(0, 0, 0, 0.25);
	}
</style>
