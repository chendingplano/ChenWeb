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
	import StringModeMenu from '../value-editors/StringModeMenu.svelte';
	import StringChoicesEditor from '../value-editors/StringChoicesEditor.svelte';
	import { currentStringMode, stringLanguages, type StringMode } from '../value-editors/string-value';

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
		if (readOnly(p) && p.editor !== 'length') return valueEditors['read-only'];
		return valueEditors[p.editor ?? commonSenseEditor(p.name, source.defaultValue(p.name))];
	}
	function isEditableColor(p: PropertyDef) {
		return (
			!readOnly(p) &&
			(p.editor ?? commonSenseEditor(p.name, source.defaultValue(p.name))) === 'color'
		);
	}
	function isEditableString(p: PropertyDef) {
		return (
			!readOnly(p) &&
			(p.editor ?? commonSenseEditor(p.name, source.defaultValue(p.name))) === 'string-editor'
		);
	}
	// The Single or List mode chosen in a string property's heading menu.
	let stringModes = $state<Record<string, StringMode>>({});
	function stringModeOf(p: PropertyDef) {
		return currentStringMode(edited(p), stringModes[p.name] ?? optionsOf(p).mode);
	}
	// A string property's List values are stored beside it, as <name>_options.choices.
	function choicesOf(p: PropertyDef): Json | undefined {
		return source.get(`${p.name}_options.choices`);
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
	let root: HTMLElement;
	let stringMenuOpen = $state(false);
	function openValidators() {
		stringMenuOpen = false;
		const group = root.querySelector<HTMLDetailsElement>('[data-group="validators"]');
		if (group) {
			group.open = true;
			group.scrollIntoView({ block: 'nearest' });
			group.querySelector<HTMLElement>('summary')?.focus();
		}
	}
	function closeStringMenuOutside(event: PointerEvent) {
		if (stringMenuOpen && !(event.target as Element).closest('.string-menu')) stringMenuOpen = false;
	}
	function setStringMode(mode: string) {
		source.set('editor_options.mode', mode);
		stringMenuOpen = false;
	}
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

<svelte:window onpointerdown={closeStringMenuOutside} />
<section
	bind:this={root}
	class="property-editor select-text"
	aria-label={title}
	onfocusin={focusIn}
	onfocusout={focusOut}
>
	<h2 class="title">{title}</h2>
	{#if ctx.get('editor') === 'string-editor'}
		<div class="string-menu">
			<button type="button" class="reset" aria-label={m.wpi_string_mode()} aria-expanded={stringMenuOpen} onclick={() => stringMenuOpen = !stringMenuOpen}>{m.wpi_string_options()}</button>
			{#if stringMenuOpen}
				<div class="string-menu-items">
					<button type="button" disabled={source.fixed('editor_options.mode')} onclick={() => setStringMode('single')}>{m.wpi_string_single()}</button>
					<button type="button" disabled={source.fixed('editor_options.mode')} onclick={() => setStringMode('multilingual')}>{m.wpi_string_multilingual()}</button>
					<button type="button" disabled={source.fixed('editor_options.mode')} onclick={() => setStringMode('list')}>{m.wpi_string_list()}</button>
					<button type="button" onclick={openValidators}>{m.wpi_vld_group_validators()}</button>
				</div>
			{/if}
		</div>
	{/if}
	{@render children?.()}
	{#each groups as group (group.id)}
		{@const properties = group.properties.filter(shown)}
		{#if properties.length && (!group.visible || group.visible(ctx))}
			<details class="group" data-group={group.id} open>
				<summary>{group.label()}</summary>
				{#if group.help?.()}<p class="hint">{group.help()}</p>{/if}
				{#each properties as p (p.name)}
					{@const id = `${idPrefix}-${p.name}`}
					{@const Editor = editorOf(p)}
					{@const help = tip(p)}
					{@const warn = warning(p)}
					<div class="property" class:length-property={p.editor === 'length' || p.name === 'editor_options.displayType'} data-property={p.name}>
						<div class="heading" class:string-heading={isEditableString(p)}>
							<label for={id}>
								{p.label?.() ?? p.name}{#if source.required(p.name)}<span
										class="required"
										title={m.propertiespanel_required()}
									>
										*</span
									>{/if}
							</label>
							{#if isEditableString(p)}
								<StringModeMenu
									heading
									value={edited(p)}
									mode={stringModeOf(p)}
									languages={stringLanguages(optionsOf(p).languages)}
									check={check(p)}
									onMode={(mode) => (stringModes[p.name] = mode)}
									onChange={(v) => source.set(p.name, toStored(p, v))}
								/>
							{/if}
							{#if isEditableColor(p)}
								<ColorModeSelect
									{id}
									label={p.label?.() ?? p.name}
									value={edited(p)}
									theme={optionsOf(p).theme}
									onChange={(v) => source.set(p.name, toStored(p, v))}
								/>
							{/if}
							{#if Editor !== valueEditors.length && source.resettable(p.name) && source.isSet(p.name) && !readOnly(p)}
								<button type="button" class="reset" onclick={() => source.set(p.name, undefined)}
									>{m.wpi_input_reset()}</button
								>
							{/if}
						</div>
						<div class="editor" class:display-type-editor={p.name === 'editor_options.displayType'} class:indented={isEditableString(p)}>
							<Editor
								{id}
								label={p.label?.() ?? p.name}
								value={edited(p)}
								options={{ ...optionsOf(p), colorModeInHeading: isEditableColor(p), ...(isEditableString(p) ? { modeInHeading: true, mode: stringModeOf(p), choices: (choicesOf(p) as string[] | undefined) ?? [] } : {}), ...(p.editor === 'length' ? { defaultValue: typeof source.defaultValue(p.name) === 'string' ? source.defaultValue(p.name) as string : undefined, useDefault: !source.isSet(p.name), readOnly: readOnly(p) } : {}) }}
								check={check(p)}
								onChange={(v) => source.set(p.name, toStored(p, v))}
							/>
							{#if help}<HelpTip id={`${id}-tip`} tip={help} name={p.label?.() ?? p.name} />{/if}
							{#if isEditableString(p) && stringModeOf(p) === 'list'}
								<div class="hint">{m.wpi_string_choices()}</div>
								<StringChoicesEditor
									id={`${id}-choices`}
									value={choicesOf(p)}
									onChange={(v) => source.set(`${p.name}_options.choices`, v)}
								/>
							{/if}
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
	.string-menu { position: relative; display: flex; justify-content: end; }
	.string-menu-items { position: absolute; z-index: 4; top: 100%; display: grid; padding: 4px; background: var(--cv-panel); border: 1px solid var(--cv-border); }
	.string-menu-items button { text-align: left; padding: 5px 12px; white-space: nowrap; font-size: 10px; }
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
	/* A string property's value and its List values are indented under its caption. */
	.editor.indented { margin-left: 16px; }
	.length-property { display: grid; grid-template-columns: minmax(60px, 1fr) minmax(100px, 2fr); align-items: start; gap: 4px; }
	.length-property .heading { margin: 0; padding: 4px 0 0; }
	.length-property .hint, .length-property .warning { grid-column: 1 / -1; }
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
	/* The string mode menu sits right after the caption; Reset stays at the end. */
	.string-heading {
		justify-content: flex-start;
	}
	.string-heading label {
		flex: 0 1 auto;
		line-height: 1.4;
	}
	.string-heading .reset {
		margin-left: auto;
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
	.display-type-editor {
		display: flex;
		align-items: center;
		box-sizing: border-box;
		min-height: 26px;
		padding: 4px 8px;
		border: 1px solid var(--cv-border, #d1d5db);
		border-radius: 4px;
		background: var(--cv-surface, transparent);
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
