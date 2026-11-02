<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { JsonObject } from '$lib/wpi/types';
	import {
		attributeTree,
		CLASS_KEYS,
		definitionAt,
		PENDING_EDITORS,
		renameAttribute,
		setAttributeDefault,
		setAttributeDefinition,
		VALUE_EDITOR_IDS,
		type AttributeDefinition,
		type AttributeNode,
		type ClassKind
	} from './class-editor';
	import { parseDefault } from './element-class-form';
	import { valueText } from './element-attributes';
	import LocalizedTextEditor from '../value-editors/LocalizedTextEditor.svelte';
	import ClassEditorOptions from './ClassEditorOptions.svelte';

	let {
		kind,
		row,
		disabled = false,
		onChange
	}: {
		kind: ClassKind;
		row: JsonObject;
		disabled?: boolean;
		onChange: (row: JsonObject) => void;
	} = $props();
	const tree = $derived(attributeTree(kind, row));
	let open = $state<Record<string, boolean>>({});
	let errors = $state<Record<string, string>>({});
	const error = (e: unknown) =>
		e instanceof Error && e.message === 'fixed_default'
			? m.wpi_class_fixed_default()
			: m.wpi_class_invalid_name();
	function change(path: string, fn: () => JsonObject) {
		try {
			onChange(fn());
			errors = { ...errors, [path]: '' };
			return true;
		} catch (e) {
			errors = { ...errors, [path]: error(e) };
			return false;
		}
	}
	function meta(path: string, patch: AttributeDefinition) {
		return change(path, () => setAttributeDefinition(kind, row, path, patch));
	}
	function rename(node: AttributeNode, name: string) {
		const nextPath = node.path.slice(0, node.path.lastIndexOf('/') + 1) + name.trim();
		change(node.path, () => {
			const next = renameAttribute(kind, row, node.path, name.trim());
			open = { ...open, [nextPath]: true };
			return next;
		});
	}
</script>

{#snippet branches(nodes: AttributeNode[], parent?: AttributeNode)}
	<ul class:children={!!parent}>
		{#each nodes as node (node.path)}
			{@const definition = definitionAt(kind, row, node.path)}
			{@const editor = definition.value_editor?.id ?? ''}
			{@const identity = node.path === '/descriptor_id' || node.path === '/schema_version'}
			{@const builtin = !parent && CLASS_KEYS.has(node.name)}
			<li>
				<button
					type="button"
					class="attribute"
					aria-expanded={!!open[node.path]}
					aria-controls={`class-form-${node.path}`}
					onclick={() => (open = { ...open, [node.path]: !open[node.path] })}
				>
					<span class="chevron" aria-hidden="true">{open[node.path] ? '▾' : '▸'}</span>
					<span class="name">{node.name}</span>
					<span class="scope"
						>{!definition.scope
							? m.wpi_class_not_configured()
							: definition.scope === 'class'
								? m.wpi_class_scope_class()
								: m.wpi_class_scope_instance()}</span
					>
				</button>
				{#if open[node.path]}
					<fieldset id={`class-form-${node.path}`} class="inline-form" {disabled}>
						<label for={`class-name-${node.path}`}>{m.wpi_class_attribute_name()}</label>
						<input
							id={`class-name-${node.path}`}
							type="text"
							value={node.name}
							readonly={builtin || Array.isArray(parent?.value)}
							onchange={(e) => rename(node, e.currentTarget.value)}
						/>
						<label for={`class-description-${node.path}`}
							>{m.wpi_class_attribute_description()}</label
						>
						<LocalizedTextEditor
							id={`class-description-${node.path}`}
							label={m.wpi_class_attribute_description()}
							value={definition.description}
							options={{}}
							onChange={(value) =>
								meta(node.path, {
									description: (value as AttributeDefinition['description']) ?? ''
								})}
						/>
						<label for={`class-scope-${node.path}`}>{m.wpi_class_attribute_scope()}</label>
						<select
							id={`class-scope-${node.path}`}
							value={definition.scope ?? ''}
							disabled={CLASS_KEYS.has(node.path.slice(1).split('/')[0]) &&
								definition.scope === 'class'}
							onchange={(e) => {
								if (!meta(node.path, { scope: e.currentTarget.value as 'class' | 'instance' })) {
									e.currentTarget.value = definition.scope ?? 'instance';
								}
							}}
						>
							<option value="" disabled>{m.wpi_class_not_configured()}</option>
							<option value="class">{m.wpi_class_scope_class()}</option>
							<option value="instance" disabled={CLASS_KEYS.has(node.path.slice(1).split('/')[0])}
								>{m.wpi_class_scope_instance()}</option
							>
						</select>
						{#if parent || kind !== 'element'}<p class="hint">
								{m.wpi_class_scope_metadata_hint()}
							</p>{/if}
						<label for={`class-default-${node.path}`}>{m.wpi_class_default_value()}</label>
						<textarea
							id={`class-default-${node.path}`}
							rows={node.children.length ? 3 : 1}
							value={valueText(node.value)}
							readonly={identity}
							onchange={(e) =>
								change(node.path, () =>
									setAttributeDefault(
										kind,
										row,
										node.path,
										e.currentTarget.value.trim() === ''
											? undefined
											: parseDefault(e.currentTarget.value)
									)
								)}
						></textarea>
						<p class="hint">{m.wpi_class_default_hint()}</p>
						<label for={`class-editor-${node.path}`}>{m.wpi_class_value_editor()}</label>
						<select
							id={`class-editor-${node.path}`}
							value={editor}
							onchange={(e) =>
								meta(node.path, { value_editor: { id: e.currentTarget.value, options: {} } })}
						>
							<option value="" disabled>{m.wpi_class_not_configured()}</option>
							{#each VALUE_EDITOR_IDS as id}<option value={id}
									>{PENDING_EDITORS.has(id) ? m.wpi_class_pending_editor({ id }) : id}</option
								>{/each}
						</select>
						{#if PENDING_EDITORS.has(editor)}<p class="hint">{m.wpi_class_editor_pending()}</p>{/if}
						{#if ['table-picker', 'field-picker', 'sort-order-editor'].includes(editor)}<p
								class="hint"
							>
								{m.wpi_class_nested_editor_hint()}
							</p>{/if}
						{#if editor}<ClassEditorOptions
								id={`class-options-${node.path}`}
								{editor}
								options={definition.value_editor?.options ?? {}}
								onChange={(options) => meta(node.path, { value_editor: { id: editor, options } })}
							/>{/if}
						{#if errors[node.path]}<p class="error" role="alert">{errors[node.path]}</p>{/if}
					</fieldset>
					{#if node.children.length}{@render branches(node.children, node)}{/if}
				{/if}
			</li>
		{/each}
	</ul>
{/snippet}

<div class="attribute-tree select-text">{@render branches(tree)}</div>

<style>
	ul {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	ul.children {
		margin-left: 14px;
		padding-left: 8px;
		border-left: 1px solid var(--cv-divider);
	}
	li {
		margin-bottom: 4px;
	}
	.attribute {
		display: flex;
		align-items: center;
		gap: 6px;
		width: 100%;
		padding: 8px;
		text-align: left;
		color: var(--cv-text);
		background: var(--cv-surface);
		border: 1px solid var(--cv-border);
		border-radius: 4px;
		cursor: pointer;
	}
	.attribute:hover,
	.attribute[aria-expanded='true'] {
		border-color: var(--cv-accent);
	}
	.name {
		font-family: ui-monospace, monospace;
		font-size: 11px;
		overflow-wrap: anywhere;
	}
	.scope {
		margin-left: auto;
		font-size: 9px;
		color: var(--cv-text-muted);
	}
	.chevron {
		color: var(--cv-text-muted);
	}
	.inline-form {
		min-width: 0;
		margin: 0 0 8px;
		padding: 6px 10px 10px;
		border: 1px solid var(--cv-divider);
		border-top: 0;
		border-radius: 0 0 4px 4px;
	}
	label {
		display: block;
		margin: 8px 0 4px;
		color: var(--cv-text-2);
		font-size: 11px;
	}
	input,
	textarea,
	select {
		box-sizing: border-box;
		width: 100%;
		padding: 5px 7px;
		font-size: 11px;
		color: var(--cv-text);
		background: var(--cv-surface);
		border: 1px solid var(--cv-border);
		border-radius: 4px;
	}
	textarea {
		resize: vertical;
		font-family: ui-monospace, monospace;
	}
	[readonly] {
		color: var(--cv-text-muted);
	}
	.hint {
		font-size: 10px;
		color: var(--cv-text-faint);
		margin: 4px 0;
	}
	.error {
		font-size: 11px;
		color: var(--destructive, #ef4444);
	}
</style>
