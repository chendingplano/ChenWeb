<!-- The value-editor code module (KnowledgeStore code_modules/interpreter-value-editor.md):
     the value editor its class fixes in editor, under its label, bound to an input-doc field. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { untrack } from 'svelte';
	import { valueEditors } from '$lib/components/shared-ui/property-editor/editors';
	import HelpTip from '$lib/components/shared-ui/property-editor/HelpTip.svelte';
	import type { ValueEditorOptions } from '$lib/components/shared-ui/value-editors/types';
	import ErrorBox from '../ErrorBox.svelte';
	import { listActionClasses, listPageLayouters } from '../api';
	import { drawingEditor } from '../properties/editor-ids';
	import { getRuntime } from '../runtime.svelte';
	import { fieldErrorText, localized } from '../text';
	import { validTextAttribute } from '../text-attributes';
	import type { Json } from '../types';
	import { bindingPointer, runValidators, type ValidationError } from '../validators';
	import { elementValidationTexts } from '../validators/messages';
	import type { ElementProps } from './index';

	let { instanceId, element }: ElementProps = $props();
	const rt = getRuntime();
	const id = $props.id();
	const d = $derived(element.desc);
	const editor = $derived(d.editor ?? '');
	const drawing = $derived(drawingEditor(editor));
	const View = $derived(
		drawing && valueEditors[d.binding_mode === 'read-only' && drawing !== 'length' ? 'read-only' : drawing]
	);
	// Without a binding the value stays on the page; an unset value is written as null.
	let unbound = $state<Json | undefined>(
		untrack(() =>
			drawing === 'color' && d.editor_options?.theme === true && !d.data_binding
				? { light: '#ffffff', dark: '#000000' }
				: undefined
		)
	);
	const value = $derived(
		d.data_binding ? (rt.read(instanceId, d.data_binding) ?? undefined) : unbound
	);
	const errors = $derived(rt.errorsFor(instanceId, d.data_binding));

	// action-picker and layouter-picker offer the stored classes unless the options name choices.
	let loaded = $state<string[] | null>(null);
	let loadFailed = $state(false);
	const loads = $derived(
		(editor === 'action-picker' || editor === 'layouter-picker') &&
			!Array.isArray(d.editor_options?.choices)
	);
	$effect(() => {
		if (!loads) return;
		let current = true;
		const request =
			editor === 'action-picker'
				? listActionClasses()
				: listPageLayouters().then((rows) => rows.map((r) => String(r.descriptor_id)));
		request.then(
			(choices) => current && ((loaded = choices), (loadFailed = false)),
			() => current && (loadFailed = true)
		);
		return () => (current = false);
	});
	const options = $derived<ValueEditorOptions>({
		...((d.editor_options ?? {}) as ValueEditorOptions),
		...(loads && loaded ? { choices: loaded } : {}),
		...(drawing === 'length' ? { readOnly: d.binding_mode === 'read-only', useDefault: value == null, displayType: d.editor_options?.displayType === true } : {})
	});

	function check(next: Json): string | null {
		return editor === 'font-picker' && !validTextAttribute('font_family', next)
			? m.wpi_value_editor_font_invalid()
			: null;
	}
	// Validators run after the editor applies a value and before a save
	// (OpenSpec wpi-primitive-validators, D8).
	let validatorErrors = $state<ValidationError[]>([]);
	const validatorTexts = $derived(
		elementValidationTexts(validatorErrors, rt.validationErrorsFor(instanceId, d.data_binding), d.validators)
	);
	function validate(next: Json | undefined): boolean {
		validatorErrors = d.binding_mode === 'read-only' ? [] : runValidators(d.validators, next, bindingPointer(d.data_binding));
		return validatorErrors.length === 0;
	}
	$effect(() =>
		rt.registerInput(instanceId, id, (focus) => {
			const valid = validate(value);
			if (!valid && focus) document.getElementById(id)?.focus();
			return valid;
		})
	);
	function change(next: Json | undefined) {
		if (d.data_binding) rt.write(instanceId, d.data_binding, next ?? null);
		else unbound = next;
		validate(d.data_binding ? (next ?? null) : next);
	}
</script>

{#if View}
	<div class="value-editor" class:length-editor={drawing === 'length'} class:color-editor={drawing === 'color' && d.binding_mode !== 'read-only'}>
		<div class="heading">
			<label class="caption" for={id}>{localized(d.label)}</label>
		</div>
		<div class="editor" class:has-options={!!options.helpTip}>
			<View {id} label={localized(d.label)} {value} {options} {check} onChange={change} />
			{#if options.helpTip}<HelpTip id={`${id}-tip`} tip={localized(options.helpTip)} name={localized(d.label)} />{/if}
		</div>
		{#if loadFailed}<p class="error">{m.wpi_value_editor_choices_failed()}</p>{/if}
		{#each errors as code (code)}<p class="error">{fieldErrorText(code)}</p>{/each}
		{#each validatorTexts as text (text)}<p class="error">{text}</p>{/each}
	</div>
{:else}
	<ErrorBox
		title={m.wpi_error_title({ descriptor: element.pageId })}
		message={m.wpi_value_editor_not_built({ editor })}
	/>
{/if}

<style>
	.value-editor {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
		width: 100%;
	}
	.length-editor, .color-editor { display: grid; grid-template-columns: minmax(60px, 1fr) minmax(100px, 2fr); align-items: start; }
	.length-editor .error, .color-editor .error { grid-column: 1 / -1; }
	.editor { position: relative; min-width: 0; }
	.editor.has-options { padding-right: 15px; }
	.heading {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 4px;
		min-width: 0;
	}
	.caption {
		font-size: 14px;
		font-weight: 500;
	}
	.error {
		margin: 0;
		font-size: 12px;
		color: #dc2626;
	}
</style>
