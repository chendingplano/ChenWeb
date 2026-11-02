<!-- Text that is either one string for every language or separate English
     and Chinese versions. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import DraftField from './DraftField.svelte';
	import { isLanguageMap, parseText, withLanguages, withTranslation } from './values';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, label, value, options, check, onChange }: ValueEditorProps = $props();
	const map = $derived(isLanguageMap(value) ? value : null);
	const languages = [
		{ code: 'en', label: () => m.wpi_text_en() },
		{ code: 'zh-cn', label: () => m.wpi_text_zh() }
	];
</script>

<select
	{id}
	class="ve-input"
	value={map ? 'localized' : 'plain'}
	onchange={(e) => onChange(withLanguages(value, e.currentTarget.value === 'localized'))}
>
	<option value="plain">{m.wpi_text_plain()}</option>
	<option value="localized">{m.wpi_text_localized()}</option>
</select>
{#if map}
	{#each languages as lang (lang.code)}
		<label class="ve-sub" for={`${id}-${lang.code}`}>{lang.label()}</label>
		<DraftField
			id={`${id}-${lang.code}`}
			value={map[lang.code] ?? ''}
			multiline
			rows={options.rows ?? 1}
			format={(v) => (typeof v === 'string' ? v : '')}
			parse={(text) => ({ ok: true, value: text })}
			invalidMessage={() => m.wpi_text_invalid()}
			check={check ? (text) => check(withTranslation(map, lang.code, String(text))) : undefined}
			onChange={(text) => onChange(withTranslation(map, lang.code, String(text ?? '')))}
		/>
	{/each}
{:else}
	<div style="margin-top:4px">
		<DraftField
			id={`${id}-text`}
			{value}
			ariaLabel={label}
			multiline
			rows={options.rows ?? 1}
			format={(v) => (typeof v === 'string' ? v : '')}
			parse={parseText}
			invalidMessage={() => m.wpi_text_invalid()}
			{check}
			commitOnBlur={options.commitOnBlur}
			{onChange}
		/>
	</div>
{/if}
{#if options.emptyAction}
	<button type="button" class="ve-button" style="margin-top:4px" onclick={() => onChange('')}>{options.emptyAction()}</button>
{/if}
