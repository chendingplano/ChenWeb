<!-- Draws any entity's properties from its definition (OpenSpec
     wpi-property-editors, design D2, D5): a title, groups that fold, and for
     each property a label, Reset, its value editor with the help-tip icon,
     and any error, warning or note. Focus entering and leaving a property is
     reported, so the editor records one undo step per focus (D7). -->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import type { Json } from '$lib/wpi/types';
	import { commonSenseEditor } from '$lib/wpi/properties/common-sense';
	import type {
		EditorDef,
		GroupDef,
		PropertyContext,
		PropertyDef,
		PropertySource
	} from '$lib/wpi/properties/types';
	import { valueEditors } from './editors';
	import HelpTip from './HelpTip.svelte';
	import ColorModeSelect from '../value-editors/ColorModeSelect.svelte';

	let {
		title,
		def,
		source,
		context,
		idPrefix,
		onEditStart,
		onEditEnd,
		children
	}: {
		title: string;
		def: EditorDef;
		source: PropertySource;
		context: Omit<PropertyContext, 'get'>;
		idPrefix: string;
		onEditStart?: () => void;
		onEditEnd?: () => void;
		children?: Snippet;
	} = $props();

	const ctx: PropertyContext = $derived({ ...context, get: (name: string) => source.get(name) });
	const listed = $derived(new Set(def.groups.flatMap((g) => g.properties.map((p) => p.name))));
	const groups = $derived.by((): GroupDef[] => {
		const other = source.names.filter((n) => !listed.has(n));
		return other.length
			? [
					...def.groups,
					{
						id: 'other',
						label: m.wpi_input_group_other,
						help: def.otherHelp,
						properties: other.map((name) => ({ name }))
					}
				]
			: def.groups;
	});

	function shown(p: PropertyDef): boolean {
		return !p.visible || p.visible(ctx);
	}
	function readOnly(p: PropertyDef): boolean {
		return !!p.computed || !!p.readOnly || source.fixed(p.name);
	}
	function stored(p: PropertyDef): Json | undefined {
		return p.computed ? p.computed(ctx) : source.get(p.name);
	}
	function edited(p: PropertyDef): Json | undefined {
		const v = stored(p);
		return p.read && !p.computed ? p.read(v) : v;
	}
	function toStored(p: PropertyDef, v: Json | undefined): Json | undefined {
		return p.write ? p.write(v) : v;
	}
	function editorOf(p: PropertyDef) {
		if (readOnly(p)) return valueEditors['read-only'];
		return valueEditors[p.editor ?? commonSenseEditor(p.name, source.defaultValue(p.name))];
	}
	function isEditableColor(p: PropertyDef) {
		return (
			!readOnly(p) &&
			(p.editor ?? commonSenseEditor(p.name, source.defaultValue(p.name))) === 'color'
		);
	}
	function optionsOf(p: PropertyDef) {
		return typeof p.options === 'function' ? p.options(ctx) : (p.options ?? {});
	}
	function check(p: PropertyDef) {
		const validate = p.validate;
		return validate ? (v: Json) => validate(toStored(p, v) as Json, ctx) : undefined;
	}
	function warning(p: PropertyDef): string | null {
		const v = stored(p);
		return p.warn && v !== undefined && !readOnly(p) ? p.warn(v, ctx) : null;
	}
	function tip(p: PropertyDef): string {
		return source.helpTip(p.name) || p.helpTip?.() || '';
	}

	// One edit per property: entering another property ends the last one.
	let editing: string | null = null;
	function focusIn(event: FocusEvent) {
		const name =
			(event.target as HTMLElement).closest<HTMLElement>('[data-property]')?.dataset.property ??
			null;
		if (name === editing) return;
		if (editing !== null) onEditEnd?.();
		editing = name;
		if (name !== null) onEditStart?.();
	}
	function focusOut(event: FocusEvent) {
		const next = event.relatedTarget as HTMLElement | null;
		const name = next?.closest<HTMLElement>('[data-property]')?.dataset.property ?? null;
		if (name === editing) return;
		if (editing !== null) onEditEnd?.();
		editing = null;
	}
</script>

<section
	class="property-editor select-text"
	aria-label={title}
	onfocusin={focusIn}
	onfocusout={focusOut}
>
	<h2 class="title">{title}</h2>
	{@render children?.()}
	{#each groups as group (group.id)}
		{@const properties = group.properties.filter(shown)}
		{#if properties.length && (!group.visible || group.visible(ctx))}
			<details class="group" open>
				<summary>{group.label()}</summary>
				{#if group.help?.()}<p class="hint">{group.help()}</p>{/if}
				{#each properties as p (p.name)}
					{@const id = `${idPrefix}-${p.name}`}
					{@const Editor = editorOf(p)}
					{@const help = tip(p)}
					{@const warn = warning(p)}
					<div class="property" data-property={p.name}>
						<div class="heading">
							<label for={id}>
								{p.label?.() ?? p.name}{#if source.required(p.name)}<span
										class="required"
										title={m.propertiespanel_required()}
									>
										*</span
									>{/if}
							</label>
							{#if isEditableColor(p)}
								<ColorModeSelect
									{id}
									label={p.label?.() ?? p.name}
									value={edited(p)}
									theme={optionsOf(p).theme}
									onChange={(v) => source.set(p.name, toStored(p, v))}
								/>
							{/if}
							{#if source.resettable(p.name) && source.isSet(p.name) && !readOnly(p)}
								<button type="button" class="reset" onclick={() => source.set(p.name, undefined)}
									>{m.wpi_input_reset()}</button
								>
							{/if}
						</div>
						<div class="editor">
							<Editor
								{id}
								label={p.label?.() ?? p.name}
								value={edited(p)}
								options={{ ...optionsOf(p), colorModeInHeading: isEditableColor(p) }}
								check={check(p)}
								onChange={(v) => source.set(p.name, toStored(p, v))}
							/>
							{#if help}<HelpTip id={`${id}-tip`} tip={help} name={p.label?.() ?? p.name} />{/if}
						</div>
						{#if source.fixed(p.name)}<p class="hint">{m.propertiespanel_fixed()}</p>{/if}
						{#if warn}<p class="warning" role="status">{warn}</p>{/if}
						{#if p.note}<p class="hint">{p.note()}</p>{/if}
					</div>
				{/each}
			</details>
		{/if}
	{/each}
</section>

<style>
	.property-editor {
		padding: 0 12px 12px;
		color: var(--cv-text);
		user-select: text;
	}
	.title {
		position: sticky;
		top: 0;
		z-index: 3;
		margin: 0 -12px 8px;
		padding: 12px;
		border-bottom: 1px solid var(--cv-divider);
		background: var(--cv-panel);
		font-size: 14px;
		font-weight: 700;
		overflow-wrap: anywhere;
	}
	.group + .group {
		margin-top: 6px;
	}
	.group summary {
		padding: 8px;
		margin-bottom: 8px;
		border: 1px solid var(--cv-border);
		border-radius: 4px;
		background: var(--cv-group-header, var(--cv-surface));
		font-size: 11px;
		font-weight: 600;
		color: #101827;
		cursor: pointer;
		user-select: none;
	}
	.group summary:hover {
		background: color-mix(in srgb, var(--cv-group-header, var(--cv-surface)) 85%, white);
	}
	.property {
		margin: 0 0 12px;
	}
	.heading {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 4px;
		margin-bottom: 4px;
		padding-right: 12px;
	}
	.heading label {
		flex: 1;
		min-width: 0;
	}
	label {
		font-size: 10px;
		color: var(--cv-text-muted);
	}
	.required {
		color: var(--cv-accent);
	}
	.reset {
		border: 1px solid var(--cv-border);
		border-radius: 4px;
		background: var(--cv-surface);
		color: var(--cv-text);
		font-size: 10px;
		padding: 2px 6px;
		cursor: pointer;
	}
	.editor {
		position: relative;
	}
	.hint,
	.warning {
		font-size: 10px;
		margin: 3px 0 0;
	}
	.hint {
		color: var(--cv-text-muted);
	}
	.warning {
		color: var(--cv-warn, #b45309);
	}
</style>
