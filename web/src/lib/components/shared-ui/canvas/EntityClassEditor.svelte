<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { getEntityClass, updateEntityClass, WpiError, type ElementClassRow } from '$lib/wpi/api';
	import type { JsonObject } from '$lib/wpi/types';
	import ClassAttributeTree from './ClassAttributeTree.svelte';
	import WpiJsonDialog from './WpiJsonDialog.svelte';
	import type { ClassTarget } from './class-editor';

	let {
		target,
		active,
		onSaved,
		onDirty,
		darkMode = true
	}: {
		target: ClassTarget | null;
		active: boolean;
		onSaved: () => void;
		onDirty?: (dirty: boolean) => void;
		darkMode?: boolean;
	} = $props();
	type Draft = {
		row: JsonObject;
		baseline: string;
		status: ElementClassRow['status'];
		version: number;
		loading: boolean;
		saving: boolean;
		error: string;
		warnings: string[];
	};
	let drafts = $state<Record<string, Draft>>({});
	let showJson = $state(false);
	export function viewJson(): boolean {
		if (!target || !draft || draft.loading || draft.saving || !Object.keys(draft.row).length)
			return false;
		showJson = true;
		return true;
	}
	const key = $derived(target ? `${target.kind}/${target.id}` : '');
	const draft = $derived(drafts[key]);
	const dirty = $derived(!!draft && JSON.stringify(draft.row) !== draft.baseline);
	const anyDirty = $derived(
		Object.values(drafts).some((d) => !d.loading && JSON.stringify(d.row) !== d.baseline)
	);
	$effect(() => onDirty?.(anyDirty));

	async function load(selection: ClassTarget, cacheKey: string) {
		drafts[cacheKey] = {
			row: {},
			baseline: '{}',
			status: 'active',
			version: 0,
			loading: true,
			saving: false,
			error: '',
			warnings: []
		};
		try {
			const result = await getEntityClass(selection.kind, selection.id);
			drafts[cacheKey] = {
				...drafts[cacheKey],
				row: result.class,
				baseline: JSON.stringify(result.class),
				status: result.status,
				version: result.version,
				loading: false
			};
		} catch (e) {
			drafts[cacheKey] = {
				...drafts[cacheKey],
				loading: false,
				error: m.wpi_class_load_failed({ detail: e instanceof Error ? e.message : String(e) })
			};
		}
	}
	$effect(() => {
		if (active && target && !drafts[key]) void load({ ...target }, key);
	});

	async function save() {
		if (!target || !draft || draft.loading || draft.saving || !dirty) return;
		const selection = { ...target },
			cacheKey = key,
			row = JSON.parse(JSON.stringify(draft.row)) as JsonObject;
		drafts[cacheKey].saving = true;
		drafts[cacheKey].error = '';
		try {
			const result = await updateEntityClass(selection.kind, selection.id, row);
			drafts[cacheKey].baseline = JSON.stringify(row);
			drafts[cacheKey].version = result.version;
			drafts[cacheKey].status = result.status;
			drafts[cacheKey].warnings = result.warnings.map((i) => `${i.path}: ${i.message}`);
			onSaved();
		} catch (e) {
			drafts[cacheKey].error =
				e instanceof WpiError && e.issues.length
					? e.issues.map((i) => `${i.path}: ${i.message}`).join('\n')
					: m.wpi_class_save_failed({ detail: e instanceof Error ? e.message : String(e) });
		} finally {
			drafts[cacheKey].saving = false;
		}
	}
	function unload(event: BeforeUnloadEvent) {
		if (anyDirty) event.preventDefault();
	}
</script>

<svelte:window onbeforeunload={unload} />

<div class="class-editor select-text">
	{#if !target}
		<p class="empty">{m.wpi_class_no_class()}</p>
	{:else if !draft || draft.loading}
		<p class="empty" role="status">{m.wpi_class_loading()}</p>
	{:else}
		<div class="heading">
			<h2>{m.wpi_class_title({ id: target.id })}</h2>
			<span class="state"
				>{draft.status === 'draft' ? m.wpi_class_draft() : m.wpi_class_active()}</span
			>
		</div>
		{#if Object.keys(draft.row).length}
			<dl class="metadata">
				<div>
					<dt>{m.wpi_class_record_version()}</dt>
					<dd data-class-version>{draft.version}</dd>
				</div>
				<div>
					<dt>{m.wpi_class_schema_version()}</dt>
					<dd data-class-schema-version>{String(draft.row.schema_version ?? '')}</dd>
				</div>
			</dl>
		{/if}
		<p class="hint">{m.wpi_class_saved_copies_hint()}</p>
		{#if draft.error}<p class="error" role="alert">{draft.error}</p>{/if}
		{#if !Object.keys(draft.row).length}
			<button type="button" onclick={() => target && load({ ...target }, key)}
				>{m.wpi_class_retry()}</button
			>
		{:else}
			<div class="actions">
				<button type="button" class="save" disabled={!dirty || draft.saving} onclick={save}
					>{draft.saving ? m.wpi_class_saving() : m.wpi_class_save()}</button
				>
				<button
					type="button"
					disabled={!dirty || draft.saving}
					onclick={() => {
						if (confirm(m.wpi_class_discard_confirm())) {
							draft.row = JSON.parse(draft.baseline);
							draft.error = '';
						}
					}}>{m.wpi_class_discard()}</button
				>
				{#if dirty}<span class="state">{m.wpi_class_unsaved()}</span>{/if}
			</div>
			{#each draft.warnings as warning}<p class="hint" role="status">{warning}</p>{/each}
			{#key key}
				<ClassAttributeTree
					kind={target.kind}
					row={draft.row}
					disabled={draft.saving}
					onChange={(row) => {
						draft.row = row;
						draft.error = '';
						draft.warnings = [];
					}}
				/>
			{/key}
		{/if}
	{/if}
</div>

{#if showJson && target && draft}
	<WpiJsonDialog
		{darkMode}
		title={m.wpi_class_json_title({ id: target.id })}
		note={m.wpi_class_json_note()}
		json={draft.row}
		onApply={(row) => {
			if (row.descriptor_id !== target.id || row.schema_version !== draft.row.schema_version)
				return false;
			draft.row = row;
			draft.error = '';
			draft.warnings = [];
			return true;
		}}
		onClose={() => (showJson = false)}
	/>
{/if}

<style>
	.class-editor {
		padding: 12px;
		color: var(--cv-text);
	}
	.heading {
		display: flex;
		align-items: baseline;
		gap: 8px;
	}
	h2 {
		margin: 0;
		font-size: 14px;
		overflow-wrap: anywhere;
	}
	.state {
		color: var(--cv-text-muted);
		font-size: 10px;
	}
	.metadata {
		display: flex;
		flex-wrap: wrap;
		gap: 8px 20px;
		margin: 8px 0;
		font-size: 11px;
	}
	.metadata div {
		display: flex;
		gap: 6px;
	}
	.metadata dt {
		color: var(--cv-text-muted);
	}
	.metadata dd {
		margin: 0;
		color: var(--cv-text);
	}
	.hint,
	.empty {
		color: var(--cv-text-muted);
		font-size: 11px;
		line-height: 1.5;
	}
	.actions {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 6px;
		margin: 12px 0;
	}
	button {
		padding: 5px 9px;
		font-size: 11px;
		background: var(--cv-surface);
		color: var(--cv-text);
		border: 1px solid var(--cv-border);
		border-radius: 4px;
		cursor: pointer;
	}
	button.save {
		border-color: var(--cv-accent);
		color: var(--cv-accent);
	}
	button:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.error {
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		color: var(--destructive, #ef4444);
		font-size: 11px;
	}
</style>
