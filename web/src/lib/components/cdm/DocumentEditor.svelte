<script lang="ts">
	import { onDestroy } from 'svelte';
	import BlockList from './BlockList.svelte';
	import type { Document } from './types.js';
	import {
		saveDocument,
		saveDocumentToNewVersion,
		publishDocument,
		renderDocument,
		getDocument,
		createDocument,
		listDocumentVersions,
		CdmApiError,
		CdmStaleVersionError,
		CdmFrozenError,
		CdmValidationError,
		CdmBlockConflictError,
		type CreateTarget,
		type DocumentVersionNode
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
		createTarget?: CreateTarget | null;
		onCreated?: (key: string) => void;
		previewPages?: string[] | null;
		previewVersion?: number | null;
		previewLoading?: boolean;
	} = $props();

	let doc = $state(structuredClone(initialDocument));
	let isNew = $derived(doc.document_key === '');
	let showCreateConfirm = $state(false);
	let showVersionsDialog = $state(false);
	let versionsLoading = $state(false);
	let versionsError = $state<string | null>(null);
	let versions = $state<DocumentVersionNode[]>([]);

	let savedSnapshot = $state(JSON.stringify(initialDocument));
	let dirty = $derived(JSON.stringify(doc) !== savedSnapshot);

	let saving = $state(false);
	let savingNewVersion = $state(false);
	let publishing = $state(false);

	let staleError = $state<CdmStaleVersionError | null>(null);
	let frozenMessage = $state<string | null>(null);
	let genericError = $state<string | null>(null);
	let attributions = $state<BlockAttribution[]>([]);
	let savedNote = $state<string | null>(null);

	let previewError = $state<string | null>(null);
	let livePreviewTimer: ReturnType<typeof setTimeout> | null = null;
	let livePreviewInFlight = $state(false);
	let hasGeneratedPreviewStructure = $derived(
		doc.blocks.length === 0 &&
			(!!doc.title.trim() ||
				!!doc.metadata.doc_type ||
				!!doc.metadata.rendering_type ||
				(doc.metadata.authors?.length ?? 0) > 0 ||
				!!doc.metadata.version)
	);

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

	function clearPreviewSurface() {
		previewPages = null;
		previewVersion = null;
		previewError = null;
	}

	function adoptSavedDocument(result: Document, note?: string) {
		doc.document_key = result.document_key;
		doc.content_version = result.content_version;
		doc.edit_version = result.edit_version;
		savedSnapshot = JSON.stringify(doc);
		if (note) {
			savedNote = note;
		}
		clearPreviewSurface();
	}

	function handleSaveError(e: unknown) {
		if (e instanceof CdmStaleVersionError) {
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
	}

	async function save() {
		if (frozenMessage || (!isNew && !dirty)) return;
		if (isNew) {
			showCreateConfirm = true;
			return;
		}
		saving = true;
		clearSaveFeedback();
		try {
			const result = await saveDocument(doc);
			adoptSavedDocument(result, `Saved current version v${result.content_version}.`);
		} catch (e) {
			handleSaveError(e);
		} finally {
			saving = false;
		}
	}

	async function saveToNewVersion() {
		if (frozenMessage || isNew || !dirty) return;
		savingNewVersion = true;
		clearSaveFeedback();
		try {
			const result = await saveDocumentToNewVersion(doc);
			adoptSavedDocument(result, `Saved to new version v${result.content_version}.`);
			if (showVersionsDialog) {
				await loadVersions();
			}
		} catch (e) {
			handleSaveError(e);
		} finally {
			savingNewVersion = false;
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
			adoptSavedDocument(
				created,
				`Created as ${created.document_key} (version ${created.content_version}).`
			);
			onCreated?.(created.document_key);
		} catch (e) {
			handleSaveError(e);
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
		if (frozenMessage || dirty || isNew) return;
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
				'This document is now published and read-only. Save to New Version is disabled until editable version branching is supported for published documents.';
		} catch (e) {
			handleSaveError(e);
		} finally {
			publishing = false;
		}
	}

	async function syncPreview(showSaveNote = true) {
		if (!doc.document_key) return;
		previewLoading = true;
		livePreviewInFlight = true;
		previewError = null;
		try {
			if (dirty) {
				clearSaveFeedback();
				const saved = await saveDocument(doc);
				adoptSavedDocument(
					saved,
					showSaveNote ? `Saved current version v${saved.content_version}.` : undefined
				);
			}
			const result = await renderDocument(doc.document_key);
			previewPages = result.pages;
			previewVersion = result.content_version;
		} catch (e) {
			if (
				e instanceof CdmStaleVersionError ||
				e instanceof CdmFrozenError ||
				e instanceof CdmValidationError ||
				e instanceof CdmBlockConflictError ||
				e instanceof CdmApiError
			) {
				handleSaveError(e);
				previewError = e.message;
			} else {
				previewError = String(e);
			}
		} finally {
			previewLoading = false;
			livePreviewInFlight = false;
		}
	}

	async function preview() {
		await syncPreview(true);
	}

	function clearLivePreviewTimer() {
		if (livePreviewTimer !== null) {
			clearTimeout(livePreviewTimer);
			livePreviewTimer = null;
		}
	}

	function scheduleLivePreview(delayMs: number) {
		if (isNew || !doc.document_key || frozenMessage) return;
		clearLivePreviewTimer();
		livePreviewTimer = setTimeout(() => {
			livePreviewTimer = null;
			void syncPreview(false);
		}, delayMs);
	}

	onDestroy(() => {
		clearLivePreviewTimer();
	});

	$effect(() => {
		const key = doc.document_key;
		const snapshot = JSON.stringify(doc);
		const isDirty = dirty;
		const isFrozen = !!frozenMessage;
		if (!key || isFrozen) {
			clearLivePreviewTimer();
			return;
		}
		if (!previewPages || previewPages.length === 0) {
			scheduleLivePreview(0);
			return;
		}
		if (isDirty) {
			scheduleLivePreview(500);
			return;
		}
		if (!livePreviewInFlight && previewVersion !== doc.content_version) {
			scheduleLivePreview(0);
		}
		void snapshot;
	});

	function formatDate(value: string) {
		return new Intl.DateTimeFormat(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		}).format(new Date(value));
	}

	function formatBytes(bytes: number) {
		return new Intl.NumberFormat(undefined).format(bytes) + ' bytes';
	}

	async function loadVersions() {
		if (!doc.document_key) return;
		versionsLoading = true;
		versionsError = null;
		try {
			const result = await listDocumentVersions(doc.document_key);
			versions = result.results;
		} catch (e) {
			versionsError = e instanceof CdmApiError ? e.message : String(e);
		} finally {
			versionsLoading = false;
		}
	}

	async function openVersions() {
		if (!doc.document_key) return;
		showVersionsDialog = true;
		await loadVersions();
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
		<div class="cdm-editor-meta">
			<span class="cdm-version-badge">
				{isNew ? 'not yet saved' : `v${doc.content_version}`}{dirty && !isNew
					? ' · unsaved changes'
					: ''}
			</span>
		</div>
		<div class="cdm-editor-actions">
			<button
				type="button"
				class="cdm-action-button"
				onclick={preview}
				disabled={previewLoading || isNew}
				title={isNew ? 'Save the document first' : undefined}
			>
				{previewLoading ? 'Rendering…' : 'Preview'}
			</button>
			<button
				type="button"
				class="cdm-action-button"
				onclick={save}
				disabled={saving || !!frozenMessage || (!isNew && !dirty)}
				title={!isNew && !dirty ? 'No unsaved changes' : undefined}
			>
				{saving ? 'Saving…' : isNew ? 'Save…' : 'Save'}
			</button>
			<button
				type="button"
				class="cdm-action-button"
				onclick={saveToNewVersion}
				disabled={savingNewVersion || !!frozenMessage || isNew || !dirty}
				title={isNew ? 'Save the document first' : !dirty ? 'No unsaved changes' : undefined}
			>
				{savingNewVersion ? 'Saving…' : 'Save to New Version'}
			</button>
			<button
				type="button"
				class="cdm-action-button"
				onclick={openVersions}
				disabled={isNew}
				title={isNew ? 'Save the document first' : undefined}
			>
				Versions
			</button>
			<button
				type="button"
				class="cdm-action-button cdm-action-button--primary"
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
				Someone else saved a newer edit state (revision {staleError.currentVersion}) while you were
				editing. Your local changes have not been discarded or sent.
			</p>
			<button type="button" class="cdm-inline-button" onclick={reloadDiscardingLocalChanges}>
				Discard my changes and reload
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
	<div class="cdm-overlay">
		<div class="cdm-dialog">
			<h2>Create this document?</h2>
			<p>
				This creates a new document titled <strong>{doc.title || '(untitled)'}</strong> in the
				knowledge store <strong>{createTarget?.ksName ?? '—'}</strong>.
			</p>
			<div class="cdm-dialog-actions">
				<button type="button" class="cdm-action-button" onclick={cancelCreate}>Cancel</button>
				<button
					type="button"
					class="cdm-action-button cdm-action-button--primary"
					onclick={confirmCreate}
				>
					Create
				</button>
			</div>
		</div>
	</div>
{/if}

{#if showVersionsDialog}
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div
		class="cdm-overlay"
		onclick={(e) => e.target === e.currentTarget && (showVersionsDialog = false)}
	>
		<div class="cdm-dialog cdm-dialog--versions">
			<div class="cdm-dialog-head">
				<div>
					<h2>Version Tree</h2>
					<p>Document versions for {doc.title || doc.document_key}</p>
				</div>
				<button
					type="button"
					class="cdm-action-button"
					onclick={() => (showVersionsDialog = false)}
				>
					Close
				</button>
			</div>

			{#if versionsLoading}
				<div class="cdm-versions-status">Loading versions…</div>
			{:else if versionsError}
				<div class="cdm-banner cdm-banner--error">{versionsError}</div>
			{:else}
				<div class="cdm-version-tree">
					{#each versions as version, index (version.content_version)}
						<div class="cdm-version-node-wrap">
							<div class="cdm-version-node" class:cdm-version-node--current={version.current}>
								<div class="cdm-version-node-head">
									<strong>Version {version.content_version}</strong>
									{#if version.current}
										<span class="cdm-version-current">Current</span>
									{/if}
								</div>
								<div class="cdm-version-node-meta">Created: {formatDate(version.create_time)}</div>
								<div class="cdm-version-node-meta">Modified: {formatDate(version.update_time)}</div>
								<div class="cdm-version-node-meta">Size: {formatBytes(version.size_bytes)}</div>
								{#if version.parent_content_version !== undefined}
									<div class="cdm-version-node-parent">
										Parent: v{version.parent_content_version}
									</div>
								{/if}
							</div>
							{#if index < versions.length - 1}
								<div class="cdm-version-connector"></div>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
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
		flex-wrap: wrap;
	}
	.cdm-title-input {
		flex: 1 1 320px;
		font-size: 1.1em;
		font-weight: 600;
		border: 1px solid rgba(127, 127, 127, 0.35);
		border-radius: 8px;
		padding: 8px 10px;
		background: rgba(255, 255, 255, 0.8);
	}
	.cdm-editor-meta {
		display: flex;
		align-items: center;
	}
	.cdm-version-badge {
		font-family: ui-monospace, monospace;
		font-size: 0.8em;
		color: var(--cdm-muted, #6b7280);
		white-space: nowrap;
	}
	.cdm-editor-actions {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}
	.cdm-action-button {
		appearance: none;
		border: 1px solid rgba(127, 127, 127, 0.35);
		border-radius: 8px;
		padding: 8px 12px;
		background: linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(240, 236, 224, 0.96));
		color: #241d16;
		font: inherit;
		font-size: 0.86rem;
		font-weight: 600;
		cursor: pointer;
		box-shadow:
			0 1px 0 rgba(255, 255, 255, 0.7) inset,
			0 1px 2px rgba(0, 0, 0, 0.08);
	}
	.cdm-action-button:hover:not(:disabled) {
		background: linear-gradient(180deg, rgba(255, 255, 255, 1), rgba(232, 224, 205, 1));
	}
	.cdm-action-button:disabled {
		opacity: 0.48;
		cursor: not-allowed;
		box-shadow: none;
	}
	.cdm-action-button--primary {
		background: linear-gradient(180deg, #1f5bd8, #1848ad);
		border-color: #143b8d;
		color: #fff;
	}
	.cdm-action-button--primary:hover:not(:disabled) {
		background: linear-gradient(180deg, #2563eb, #1d4ed8);
	}
	.cdm-inline-button {
		appearance: none;
		border: 1px solid currentColor;
		border-radius: 7px;
		padding: 6px 10px;
		background: transparent;
		color: inherit;
		font: inherit;
		font-size: 0.86rem;
		font-weight: 600;
		cursor: pointer;
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
	.cdm-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 50;
		padding: 24px;
	}
	.cdm-dialog {
		background: var(--cdm-surface, #fff);
		width: min(440px, 92vw);
		border-radius: 10px;
		padding: 20px;
		box-shadow: 0 24px 70px rgba(0, 0, 0, 0.28);
	}
	.cdm-dialog--versions {
		width: min(760px, 96vw);
		max-height: 84vh;
		overflow: auto;
	}
	.cdm-dialog-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 16px;
		margin-bottom: 18px;
	}
	.cdm-dialog h2 {
		margin: 0 0 6px;
		font-size: 1.05em;
	}
	.cdm-dialog p {
		margin: 0;
		font-size: 0.92em;
		line-height: 1.6;
		color: var(--cdm-muted, #6b7280);
	}
	.cdm-dialog-actions {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
		margin-top: 16px;
	}
	.cdm-versions-status {
		padding: 20px 4px;
		color: var(--cdm-muted, #6b7280);
	}
	.cdm-version-tree {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
	}
	.cdm-version-node-wrap {
		display: flex;
		flex-direction: column;
		align-items: center;
		width: 100%;
	}
	.cdm-version-node {
		width: min(100%, 520px);
		align-self: flex-start;
		border: 1px solid rgba(127, 127, 127, 0.35);
		border-radius: 12px;
		padding: 14px 16px;
		background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(244, 239, 228, 0.98));
		box-shadow: 0 8px 24px rgba(0, 0, 0, 0.06);
	}
	.cdm-version-node--current {
		border-color: #1d4ed8;
		box-shadow:
			0 0 0 2px rgba(29, 78, 216, 0.12),
			0 8px 24px rgba(0, 0, 0, 0.08);
	}
	.cdm-version-node-head {
		display: flex;
		align-items: center;
		gap: 10px;
		margin-bottom: 8px;
	}
	.cdm-version-current {
		border-radius: 999px;
		padding: 2px 8px;
		background: rgba(29, 78, 216, 0.12);
		color: #1d4ed8;
		font-size: 0.72rem;
		font-weight: 700;
	}
	.cdm-version-node-meta,
	.cdm-version-node-parent {
		font-size: 0.84rem;
		color: #54493d;
		margin-top: 4px;
	}
	.cdm-version-connector {
		width: 2px;
		height: 22px;
		background: rgba(127, 127, 127, 0.35);
		margin: 6px 0;
	}
</style>
