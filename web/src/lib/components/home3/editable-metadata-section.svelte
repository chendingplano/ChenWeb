<script lang="ts">
	import { safePrettyJson } from './kb-input-metadata.js';

	type EditorKind = 'text' | 'textarea' | 'datetime' | 'array' | 'json';
	type MetadataRow = {
		label: string;
		key: string;
		value: string;
		rawValue: unknown;
		editable: boolean;
		editor?: EditorKind;
		editKey?: string;
		wide?: boolean;
		pathLike?: boolean;
	};

	let {
		title,
		rows = [],
		emptyText = 'No data.',
		canEdit = false,
		onSave
	}: {
		title: string;
		rows?: MetadataRow[];
		emptyText?: string;
		canEdit?: boolean;
		onSave?: ((row: MetadataRow, draft: string, editor: EditorKind) => Promise<void>) | null;
	} = $props();

	let editingKey = $state<string | null>(null);
	let editingEditor = $state<EditorKind>('text');
	let editingDraft = $state('');
	let editingError = $state('');
	let editingSaving = $state(false);

	function isEditingRow(row: MetadataRow): boolean {
		return editingKey === (row.editKey ?? row.key);
	}

	function editorDraftFromValue(editor: EditorKind, rawValue: unknown): string {
		switch (editor) {
			case 'json':
				return safePrettyJson(rawValue);
			case 'array':
				return Array.isArray(rawValue) ? rawValue.map((v) => String(v)).join('\n') : '';
			case 'datetime':
				return typeof rawValue === 'string' ? rawValue : '';
			case 'textarea':
			case 'text':
			default:
				return rawValue == null ? '' : String(rawValue);
		}
	}

	function startEdit(row: MetadataRow) {
		const editor = row.editor ?? 'text';
		editingKey = row.editKey ?? row.key;
		editingEditor = editor;
		editingDraft = editorDraftFromValue(editor, row.rawValue);
		editingError = '';
	}

	function cancelEdit() {
		if (editingSaving) return;
		editingKey = null;
		editingDraft = '';
		editingError = '';
	}

	async function saveEdit(row: MetadataRow) {
		if (!onSave || editingSaving) return;
		editingSaving = true;
		editingError = '';
		try {
			await onSave(row, editingDraft, editingEditor);
			cancelEdit();
		} catch (error) {
			editingError = error instanceof Error ? error.message : 'Failed to save changes.';
		} finally {
			editingSaving = false;
		}
	}
</script>

<div class="metadata-section">
	<div class="metadata-section-title">{title}</div>
	{#if rows.length === 0}
		<div class="metadata-empty">{emptyText}</div>
	{:else}
		<div class="metadata-fields">
			{#each rows as row ((row.editKey ?? row.key))}
				<div class="metadata-row" class:metadata-row-wide={row.wide}>
					<span class="metadata-key" class:metadata-key-path={row.pathLike} title={row.label}>
						{row.label}
					</span>
					<div class="metadata-val-wrap">
						{#if isEditingRow(row)}
							<div class="metadata-editor">
								{#if editingEditor === 'textarea' || editingEditor === 'json' || editingEditor === 'array'}
									<textarea
										class="metadata-input metadata-input-textarea"
										rows={editingEditor === 'json' ? 8 : editingEditor === 'array' ? 5 : 4}
										bind:value={editingDraft}
									></textarea>
								{:else if editingEditor === 'datetime'}
									<input class="metadata-input" type="datetime-local" bind:value={editingDraft} />
								{:else}
									<input class="metadata-input" type="text" bind:value={editingDraft} />
								{/if}
								{#if editingError}
									<div class="metadata-edit-error">{editingError}</div>
								{/if}
								<div class="metadata-editor-actions">
									<button
										class="metadata-editor-btn metadata-editor-btn-primary"
										type="button"
										onclick={() => saveEdit(row)}
										disabled={editingSaving}
									>
										{editingSaving ? 'Saving…' : 'Save'}
									</button>
									<button
										class="metadata-editor-btn"
										type="button"
										onclick={cancelEdit}
										disabled={editingSaving}
									>
										Cancel
									</button>
								</div>
							</div>
						{:else if canEdit && row.editable && onSave}
							<div class="metadata-display">
								<button
									type="button"
									class="metadata-edit-trigger"
									title={`${row.value}\n(Double click to edit)`}
									ondblclick={() => startEdit(row)}
								>
									<span class="metadata-edit-text">{row.value}</span>
								</button>
								<button
									type="button"
									class="metadata-edit-icon-btn"
									title="Edit field"
									aria-label={`Edit ${row.label}`}
									onclick={() => startEdit(row)}
								>
									✎
								</button>
							</div>
						{:else}
							<span class="metadata-val" title={row.value}>{row.value}</span>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.metadata-section {
		border: 1px solid var(--ink-line-soft);
		background: var(--panel-bg-alt);
		padding: 10px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.metadata-section-title {
		font-family: var(--font-mono, monospace);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--text-muted);
	}

	.metadata-empty {
		font-size: 12px;
		color: var(--text-muted);
	}

	.metadata-fields {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.metadata-row {
		display: grid;
		grid-template-columns: 130px 1fr;
		gap: 8px;
		align-items: start;
	}

	.metadata-row-wide {
		grid-template-columns: minmax(132px, 180px) minmax(0, 1fr);
	}

	.metadata-key {
		font-family: var(--font-mono, monospace);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-muted);
	}

	.metadata-key-path {
		text-transform: none;
		letter-spacing: 0.02em;
		word-break: break-word;
	}

	.metadata-val-wrap {
		min-width: 0;
	}

	.metadata-val {
		font-size: 12px;
		color: var(--text-primary);
		word-break: break-word;
	}

	.metadata-display {
		display: flex;
		align-items: flex-start;
		gap: 6px;
		min-width: 0;
	}

	.metadata-edit-trigger {
		flex: 1;
		min-width: 0;
		padding: 0;
		border: 0;
		background: transparent;
		color: inherit;
		text-align: left;
		cursor: pointer;
	}

	.metadata-edit-text {
		display: inline-block;
		font-size: 12px;
		color: var(--text-primary);
		word-break: break-word;
	}

	.metadata-edit-icon-btn {
		flex: 0 0 auto;
		height: 24px;
		min-width: 24px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg);
		color: var(--text-secondary);
		cursor: pointer;
	}

	.metadata-edit-icon-btn:hover,
	.metadata-edit-icon-btn:focus-visible,
	.metadata-edit-trigger:hover .metadata-edit-text,
	.metadata-edit-trigger:focus-visible .metadata-edit-text {
		color: var(--brass);
		outline: none;
	}

	.metadata-editor {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.metadata-input {
		width: 100%;
		min-width: 0;
		padding: 8px 10px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg);
		color: var(--text-primary);
		font-size: 12px;
	}

	.metadata-input-textarea {
		resize: vertical;
		font-family: var(--font-mono, monospace);
		line-height: 1.4;
	}

	.metadata-edit-error {
		font-size: 11px;
		color: #c8553d;
	}

	.metadata-editor-actions {
		display: flex;
		gap: 6px;
	}

	.metadata-editor-btn {
		height: 28px;
		padding: 0 10px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg);
		color: var(--text-primary);
		cursor: pointer;
	}

	.metadata-editor-btn-primary {
		border-color: var(--brass);
		color: var(--brass);
	}

	.metadata-editor-btn:disabled {
		opacity: 0.7;
		cursor: default;
	}
</style>
