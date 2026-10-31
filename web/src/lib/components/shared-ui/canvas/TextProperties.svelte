<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { localized } from '$lib/wpi/text';
	import {
		textGroups,
		textChoices,
		textDefaults,
		validTextAttribute
	} from '$lib/wpi/text-attributes';
	import type { ElementAttribute } from '$lib/wpi/api';
	import type { Json, JsonObject } from '$lib/wpi/types';
	import { valueText, withAttribute } from './element-attributes';
	import {
		textAttributeLabel,
		textChoiceLabel,
		textGroupLabel,
		textGroupHelp
	} from './text-property-labels';
	let {
		attributes,
		attrs,
		pane,
		onChange
	}: {
		attributes: Record<string, ElementAttribute>;
		attrs: JsonObject;
		pane: string;
		onChange: (attrs: JsonObject) => void;
	} = $props();
	const definitions = $derived({
		width: {},
		height: {},
		min_width: {},
		max_width: {},
		...attributes
	} as Record<string, ElementAttribute>);
	const known = new Set(textGroups.flatMap(([, names]) => names));
	const groups = $derived([
		...textGroups,
		['other', Object.keys(definitions).filter((n) => !known.has(n))] as [string, string[]]
	]);
	function get(name: string): Json | undefined {
		return attrs[name] ?? definitions[name]?.default_value ?? textDefaults[name];
	}
	function exceedsMaximum(name: string): boolean {
		if (name !== 'width' && name !== 'height') return false;
		const value = get(name);
		const maximum = get(`max_${name}`);
		if (typeof value !== 'string' || typeof maximum !== 'string') return false;
		const length = /^(\d+(?:\.\d+)?)(px|rem|em|%)$/;
		const a = length.exec(value);
		const b = length.exec(maximum);
		return !!a && !!b && a[2] === b[2] && Number(a[1]) > Number(b[1]);
	}
	function reset(name: string) {
		const next = { ...attrs };
		delete next[name];
		onChange(next);
	}
	function set(name: string, value: Json) {
		onChange({ ...attrs, [name]: value });
	}
	let editing = $state<{ name: string; text: string } | null>(null);
	function visible(name: string): boolean {
		if (
			name.startsWith('label_') &&
			!localized(get('label') as string | Record<string, string> | undefined)
		)
			return false;
		if (name.startsWith('icon_') && !get('icon')) return false;
		const horizontal = ['left', 'right'].includes(String(get('label_placement') ?? 'top'));
		if (['label_horizontal_gap', 'label_vertical_align'].includes(name)) return horizontal;
		return true;
	}
	function translation(name: string, locale: string, value: string) {
		const current = get(name);
		const map: JsonObject =
			current && typeof current === 'object' && !Array.isArray(current)
				? { ...current }
				: { en: typeof current === 'string' ? current : '', 'zh-cn': '' };
		map[locale] = value;
		set(name, map);
	}
	const sourceConflict = $derived(get('text') != null && get('data_binding') != null);
</script>

<div class="text-properties select-text" data-text-properties>
	<p class="pane">{m.wpi_element_pane()}: {pane}</p>
	{#if sourceConflict}<p role="alert" class="error">{m.wpi_text_source_conflict()}</p>{/if}
	{#each groups as [group, names] (group)}
		{@const present = names.filter((name) => name in definitions && visible(name))}
		{#if present.length}
			<section aria-label={textGroupLabel(group)}>
				<h3>{textGroupLabel(group)}</h3>
				{#if textGroupHelp(group)}<p class="hint">{textGroupHelp(group)}</p>{/if}
				{#each present as name (name)}
					{@const def = definitions[name]}
					{@const value = get(name)}
					{@const fixed = def.instance_modifiable === false}
					{@const localizedField = ['text', 'label', 'help_tip'].includes(name)}
					{@const languageMap =
						value !== null && typeof value === 'object' && !Array.isArray(value)}
					{@const exceeds = editing?.name !== name && editing?.name !== `max_${name}` && exceedsMaximum(name)}
					{@const invalid = editing?.name !== name && ((value !== undefined && !validTextAttribute(name, value)) || exceeds)}
					<div
						class="attribute"
						role={localizedField ? 'group' : undefined}
						aria-labelledby={localizedField ? `text-prop-${name}-heading` : undefined}
					>
						<div class="heading">
							<label
								id={`text-prop-${name}-heading`}
								for={localizedField && !fixed ? `text-prop-${name}-format` : `text-prop-${name}`}
								>{textAttributeLabel(name)}{#if def.default_value === null}
									*{/if}</label
							>
							{#if !fixed && name in attrs}<button type="button" onclick={() => reset(name)}
									>{m.wpi_input_reset()}</button
								>{/if}
						</div>
						{#if fixed}
							<textarea
								id={`text-prop-${name}`}
								readonly
								rows="1"
								value={valueText(def.default_value)}
							></textarea>
							<p class="hint">{m.propertiespanel_fixed()}</p>
						{:else if localizedField}
							<select
								id={`text-prop-${name}-format`}
								aria-label={textAttributeLabel(name)}
								value={languageMap ? 'localized' : 'plain'}
								onchange={(e) => {
									if (e.currentTarget.value === 'localized')
										set(name, { en: typeof value === 'string' ? value : '', 'zh-cn': '' });
									else set(name, languageMap && typeof value.en === 'string' ? value.en : '');
								}}
							>
								<option value="plain">{m.wpi_text_plain()}</option><option value="localized"
									>{m.wpi_text_localized()}</option
								>
							</select>
							{#if languageMap}
								{#each ['en', 'zh-cn'] as lang}
									<label
										id={`text-prop-${name}-${lang}-heading`}
										for={`text-prop-${name}-${lang}`}
										class="hint">{lang === 'en' ? m.wpi_text_en() : m.wpi_text_zh()}</label
									>
									<textarea
										id={`text-prop-${name}-${lang}`}
										aria-labelledby={`text-prop-${name}-heading text-prop-${name}-${lang}-heading`}
										rows="2"
										value={String(value[lang] ?? '')}
										aria-invalid={invalid}
										oninput={(e) => translation(name, lang, e.currentTarget.value)}
									></textarea>
								{/each}
							{:else}
								<textarea
									id={`text-prop-${name}`}
									aria-labelledby={`text-prop-${name}-heading`}
									rows="2"
									value={typeof value === 'string' ? value : ''}
									aria-invalid={invalid}
									oninput={(e) => {
										if (e.currentTarget.value === '') reset(name);
										else set(name, e.currentTarget.value);
									}}
								></textarea>
							{/if}
							{#if name === 'text' || name === 'help_tip'}<button
									type="button"
									onclick={() => set(name, '')}
									>{name === 'text' ? m.wpi_text_empty() : m.wpi_text_empty_help()}</button
								>{/if}
						{:else if textChoices[name]}
							<select
								id={`text-prop-${name}`}
								value={String(value ?? '')}
								aria-invalid={invalid}
								onchange={(e) => {
									const raw = e.currentTarget.value;
									set(
										name,
										['font_weight', 'label_font_weight'].includes(name) && /^\d+$/.test(raw)
											? Number(raw)
											: raw
									);
								}}
							>
								{#if value !== undefined && !textChoices[name].includes(String(value))}<option
										value={String(value)}>{String(value)}</option
									>{/if}
								{#each textChoices[name] as choice}<option value={choice}
										>{textChoiceLabel(choice)}</option
									>{/each}
							</select>
						{:else}
							<input
								id={`text-prop-${name}`}
								type="text"
								value={editing?.name === name ? editing.text : valueText(value)}
								aria-invalid={invalid}
								onfocus={(e) => (editing = { name, text: e.currentTarget.value })}
								oninput={(e) => (editing = { name, text: e.currentTarget.value })}
								onblur={(e) => {
									editing = null;
									if (e.currentTarget.value !== valueText(get(name)))
										onChange(withAttribute(attrs, name, e.currentTarget.value));
								}}
							/>
						{/if}
						{#if invalid}<p class="error" role="alert">{exceeds ? m.wpi_text_exceeds_maximum() : m.wpi_text_invalid()}</p>{/if}
						{#if group === 'other'}<p class="hint" role="status">{m.wpi_text_unsupported()}</p>{/if}
						{#if def.help_tip}<p class="hint">{localized(def.help_tip)}</p>{/if}
						{#if !fixed}<p class="origin">
								{name in attrs
									? m.wpi_text_override()
									: def.default_value != null
										? m.wpi_text_class_default()
										: name in textDefaults
											? m.wpi_text_module_default()
											: m.wpi_text_optional()}
							</p>{/if}
					</div>
				{/each}
			</section>
		{/if}
	{/each}
</div>

<style>
	.text-properties {
		color: var(--cv-text);
		font-size: 12px;
		user-select: text;
	}
	h3 {
		font-weight: 600;
		margin: 14px 0 6px;
	}
	.attribute {
		margin: 10px 0;
	}
	.heading {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 4px;
		margin-bottom: 4px;
	}
	label {
		font-size: 12px;
	}
	input,
	textarea,
	select {
		width: 100%;
		box-sizing: border-box;
		border: 1px solid var(--cv-border);
		border-radius: 4px;
		padding: 5px 7px;
		background: var(--cv-surface);
		color: var(--cv-text);
		font-size: 12px;
	}
	textarea {
		resize: vertical;
	}
	button {
		font-size: 11px;
		padding: 4px;
		color: var(--cv-accent);
		cursor: pointer;
	}
	button:focus-visible,
	input:focus-visible,
	textarea:focus-visible,
	select:focus-visible {
		outline: 2px solid var(--cv-accent);
		outline-offset: 2px;
	}
	.hint,
	.origin,
	.pane {
		font-size: 11px;
		color: var(--cv-text-muted);
		margin-top: 3px;
	}
	.error {
		color: var(--destructive, #b91c1c);
		font-size: 12px;
	}
</style>
