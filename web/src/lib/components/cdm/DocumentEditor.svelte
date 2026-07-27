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
	//
	// initialDocument.document_key === '' is a second, deliberate case: a
	// brand-new document the host (CdmEditorShell) built purely in memory
	// for "New Document," never sent to the server. Nothing above this
	// component has called createDocument yet -- Save is what does that, and
	// only after the author confirms it in the create dialog below, naming
	// the knowledge store it commits to (design D2's "creation allocates a
	// document_key and writes kb.inputs immediately" still holds; what moved
	// is *when* the UI decides to call it, not what the API does).
	import BlockList from './BlockList.svelte';
	import type { Document } from './types.js';
	import {
		saveDocument,
		publishDocument,
		renderDocument,
		getDocument,
		createDocument,
		CdmApiError,
		CdmStaleVersionError,
		CdmFrozenError,
		CdmValidationError,
		CdmBlockConflictError,
		type CreateTarget
	} from './cdm-client.js';
	import { attributeToBlocks, type BlockAttribution } from './document-editor-ops.js';

	let {
		initialDocument,
		createTarget = null,
		onCreated,
		previewPages = $bindable(null),
		previewVersion = $bindable(null),
		previewLoading = $bindable(false)
	}: {
		initialDocument: Document;
		// Required to save a new (key-less) document; unused once it has a
		// document_key. Null-checked defensively in confirmCreate, but every
		// real host always supplies one before letting Save be clicked, since
		// CdmEditorShell never opens "New Document" without an active store.
		createTarget?: CreateTarget | null;
		// Fires once, right after a brand-new document's first save succeeds
		// and it has a real document_key. Lets the host (e.g. refresh its
		// document list) react without polling doc.document_key itself.
		onCreated?: (key: string) => void;
		// Bindable rather than a callback: the host (CdmEditorShell) docks
		// these in a persistent pane instead of this component's own overlay
		// (removed below), and $bindable is the same "child mutates, parent
		// owns the value" idiom BlockList already uses for `blocks`.
		previewPages?: string[] | null;
		previewVersion?: number | null;
		previewLoading?: boolean;
	} = $props();

	let doc = $state(structuredClone(initialDocument));
	let isNew = $derived(doc.document_key === '');
	let showCreateConfirm = $state(false);

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

	let previewError = $state<string | null>(null);
	let hasGeneratedPreviewStructure = $derived(
		doc.blocks.length === 0 &&
			(!!doc.title.trim() ||
				!!doc.metadata.doc_type ||
				!!doc.metadata.rendering_type ||
				(doc.metadata.authors?.length ?? 0) > 0 ||
				!!doc.metadata.version)
	);

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

	// Save on a brand-new document does not call the API directly -- it opens
	// the confirm dialog below, which is what actually calls createDocument
	// (confirmCreate). Every existing-document save still goes straight
	// through, unchanged.
	async function save() {
		if (frozenMessage) return;
		if (isNew) {
			showCreateConfirm = true;
			return;
		}
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

	function cancelCreate() {
		showCreateConfirm = false;
	}

	async function confirmCreate() {
		if (!createTarget) {
			genericError = 'No knowledge store is active — cannot create a document.';
			showCreateConfirm = false;
			return;
		}
		showCreateConfirm = false;
		saving = true;
		clearSaveFeedback();
		try {
			const created = await createDocument(doc, {
				tenantId: createTarget.tenantId,
				ksStoreId: createTarget.ksStoreId
			});
			// Adopt the server-allocated key/version the same conservative way
			// save() adopts content_version: doc.blocks/title stay exactly what
			// was just sent, since they are already validated and a full
			// replacement risks clobbering an in-flight edit.
			doc.document_key = created.document_key;
			doc.content_version = created.content_version;
			savedSnapshot = JSON.stringify(doc);
			savedNote = `Created as ${created.document_key} (version ${created.content_version}).`;
			onCreated?.(created.document_key);
		} catch (e) {
			if (e instanceof CdmValidationError) {
				attributions = attributeToBlocks(e.violations);
				genericError = 'This document has validation problems (see below).';
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
		//
		// Guarded defensively even though the Preview button is disabled for
		// isNew below -- renderDocument needs a real document_key and has none
		// to call with yet.
		if (!doc.document_key) return;
		previewLoading = true;
		previewError = null;
		try {
			const result = await renderDocument(doc.document_key);
			previewPages = result.pages;
			previewVersion = result.content_version;
		} catch (e) {
			previewError = e instanceof CdmApiError ? e.message : String(e);
		} finally {
			previewLoading = false;
		}
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
		<span class="cdm-version-badge">
			{isNew ? 'not yet saved' : `v${doc.content_version}`}{dirty && !isNew
				? ' · unsaved changes'
				: ''}
		</span>
		<div class="cdm-editor-actions">
			<button
				type="button"
				onclick={preview}
				disabled={previewLoading || isNew}
				title={isNew ? 'Save the document first' : undefined}
			>
				{previewLoading ? 'Rendering…' : 'Preview'}
			</button>
			<button type="button" onclick={save} disabled={saving || !!frozenMessage}>
				{saving ? 'Saving…' : isNew ? 'Save…' : 'Save'}
			</button>
			<button
				type="button"
				onclick={publish}
				disabled={publishing || !!frozenMessage || dirty || isNew}
				title={isNew
					? 'Save the document first'
					: dirty
						? 'Save your changes before publishing'
						: undefined}
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

	<BlockList
		bind:blocks={doc.blocks}
		editable={!frozenMessage}
		{blockErrors}
		emptyMessage={hasGeneratedPreviewStructure
			? 'This document has no editable blocks yet. The preview may still show generated title and outline pages.'
			: 'This document has no blocks yet.'}
	/>

	{#if previewError}
		<div class="cdm-banner cdm-banner--error">{previewError}</div>
	{/if}
</div>

{#if showCreateConfirm}
	<div class="cdm-confirm-overlay">
		<div class="cdm-confirm-panel">
			<h2>Create this document?</h2>
			<p>
				This creates a new document titled <strong>{doc.title || '(untitled)'}</strong> in the
				knowledge store
				<strong>{createTarget?.ksName ?? '—'}</strong>.
			</p>
			<div class="cdm-confirm-actions">
				<button type="button" onclick={cancelCreate}>Cancel</button>
				<button type="button" class="cdm-confirm-primary" onclick={confirmCreate}> Create </button>
			</div>
		</div>
	</div>
{/if}

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
	.cdm-confirm-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 50;
	}
	.cdm-confirm-panel {
		background: var(--cdm-surface, #fff);
		width: min(440px, 92vw);
		border-radius: 8px;
		padding: 20px;
	}
	.cdm-confirm-panel h2 {
		margin: 0 0 10px;
		font-size: 1.05em;
	}
	.cdm-confirm-panel p {
		margin: 0 0 16px;
		font-size: 0.92em;
		line-height: 1.6;
	}
	.cdm-confirm-actions {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
	}
	.cdm-confirm-primary {
		font-weight: 600;
	}
</style>
