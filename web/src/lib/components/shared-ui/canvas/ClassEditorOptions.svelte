<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { Json, JsonObject } from '$lib/wpi/types';
	import JsonEditor from '../value-editors/JsonEditor.svelte';
	import { object } from './class-editor';

	let {
		id,
		editor,
		options,
		onChange
	}: { id: string; editor: string; options: JsonObject; onChange: (value: JsonObject) => void } =
		$props();
	const lists = $derived(
		editor === 'length'
			? ['units', 'keywords']
			: [
						'value-picker',
						'ordered-multi-picker',
						'alignment-picker',
						'button-type-picker',
						'action-picker',
						'layouter-picker'
				  ].includes(editor)
				? ['choices']
				: []
	);
	const numbers = $derived(
		editor === 'number'
			? ['min', 'max']
			: editor === 'ordered-multi-picker'
				? ['minItems']
				: ['json', 'multiline-text'].includes(editor)
					? ['rows']
					: []
	);
	const toggles = $derived(
		editor === 'number'
			? ['integer']
			: editor === 'color'
				? ['theme']
				: editor === 'validation'
					? ['numberRules']
					: ['date-picker', 'time-picker'].includes(editor)
						? ['range']
						: ['text', 'multiline-text'].includes(editor)
							? ['commitOnBlur']
							: []
	);
	function label(key: string): string {
		const labels: Record<string, () => string> = {
			choices: m.wpi_class_choices,
			units: m.wpi_class_units,
			keywords: m.wpi_class_keywords,
			min: m.wpi_class_min,
			max: m.wpi_class_max,
			step: m.wpi_class_step,
			minItems: m.wpi_class_min_items,
			rows: m.wpi_class_rows,
			integer: m.wpi_class_integer,
			theme: m.wpi_class_theme,
			numberRules: m.wpi_class_number_rules,
			range: m.wpi_class_range,
			commitOnBlur: m.wpi_class_commit_blur
		};
		return labels[key]?.() ?? key;
	}
	function set(key: string, value: Json | undefined) {
		const next = { ...options };
		if (value === undefined) delete next[key];
		else next[key] = value;
		onChange(next);
	}
</script>

<fieldset class="options">
	<legend>{m.wpi_class_editor_options()}</legend>
	{#each lists as key (key)}
		<label for={`${id}-${key}`}>{label(key)}</label>
		<textarea
			id={`${id}-${key}`}
			rows="3"
			value={Array.isArray(options[key]) ? (options[key] as Json[]).join('\n') : ''}
			oninput={(e) =>
				set(
					key,
					e.currentTarget.value
						.split('\n')
						.map((v) => v.trim())
						.filter(Boolean)
				)}
		></textarea>
		<p>{m.wpi_class_one_per_line()}</p>
	{/each}
	{#each numbers as key (key)}
		<label for={`${id}-${key}`}>{label(key)}</label>
		<input
			id={`${id}-${key}`}
			type="number"
			step={key === 'rows' || key === 'minItems' ? '1' : 'any'}
			min={key === 'rows' ? 1 : key === 'minItems' ? 0 : undefined}
			value={typeof options[key] === 'number' ? (options[key] as number) : ''}
			onchange={(e) => {
				const v = e.currentTarget.value;
				if (e.currentTarget.validity.valid) set(key, v === '' ? undefined : Number(v));
			}}
		/>
	{/each}
	{#each toggles as key (key)}
		<label class="toggle"
			><input
				type="checkbox"
				checked={options[key] === true}
				onchange={(e) => set(key, e.currentTarget.checked)}
			/>{label(key)}</label
		>
	{/each}
	<details>
		<summary>{m.wpi_class_advanced_options()}</summary>
		<p>{m.wpi_class_options_json_hint({ editor })}</p>
		<JsonEditor
			id={`${id}-json`}
			label={m.wpi_class_editor_options()}
			value={options}
			options={{ rows: 4 }}
			check={(value) =>
				value !== null && typeof value === 'object' && !Array.isArray(value)
					? null
					: m.wpi_class_options_object()}
			onChange={(value) => onChange(object(value))}
		/>
	</details>
</fieldset>

<style>
	.options {
		min-width: 0;
		margin: 6px 0 0;
		padding: 10px;
		border: 1px solid var(--cv-divider);
		border-radius: 4px;
	}
	legend,
	label,
	summary {
		font-size: 11px;
		color: var(--cv-text-2);
	}
	label {
		display: block;
		margin: 7px 0 4px;
	}
	input:not([type='checkbox']),
	textarea {
		width: 100%;
		box-sizing: border-box;
		background: var(--cv-surface);
		border: 1px solid var(--cv-border);
		color: var(--cv-text);
		border-radius: 4px;
		padding: 5px 7px;
		font-size: 11px;
	}
	textarea {
		resize: vertical;
	}
	.toggle {
		display: flex;
		align-items: center;
		gap: 7px;
	}
	p {
		font-size: 10px;
		color: var(--cv-text-faint);
		margin: 3px 0 6px;
	}
	details {
		margin-top: 8px;
	}
	summary {
		cursor: pointer;
		margin-bottom: 6px;
	}
</style>
