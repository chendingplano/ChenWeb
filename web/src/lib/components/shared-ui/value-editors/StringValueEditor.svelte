<script lang="ts">
 import { untrack } from 'svelte';
 import { m } from '$lib/paraglide/messages.js';
 import DraftField from './DraftField.svelte';
 import StringModeMenu from './StringModeMenu.svelte';
 import { jsonText } from './values';
 import { currentStringMode, emptyStringValue, stringLanguages, type StringMode } from './string-value';
 import type { ValueEditorProps } from './types';
 import type { Json } from '$lib/wpi/types';
 import './value-editors.css';
 let { id, label, value, options, check, onChange }: ValueEditorProps = $props();
 const languages = $derived(stringLanguages(options.languages));
 const map = $derived(value && typeof value === 'object' && !Array.isArray(value) ? value : null);
 // With modeInHeading the host draws the menu and passes the current mode as options.mode.
 let chosen = $state<StringMode | undefined>(untrack(() => options.mode));
 const mode = $derived(currentStringMode(value, options.modeInHeading ? options.mode : chosen));
 const parsed = (text: string) => ({ ok: true as const, value: emptyStringValue(text, options.emptyAsNull ?? true) });
 function translate(language: string, next: Json | undefined) {
  const updated = { ...(map ?? {}), [language]: next ?? null };
  if (!check?.(updated)) onChange(updated);
 }
</script>
<div class="ve-row">
 <div class="value">
  {#if mode === 'single'}
   <DraftField {id} {value} ariaLabel={label} format={(v) => typeof v === 'string' ? v : v == null ? '' : jsonText(v)} parse={parsed} invalidMessage={() => m.wpi_text_invalid()} {check} commitOnBlur={options.commitOnBlur} {onChange} />
  {:else if mode === 'list'}
   <select {id} class="ve-input" aria-label={label} value={typeof value === 'string' ? (options.choices ?? []).includes(value) ? `choice-${(options.choices ?? []).indexOf(value)}` : 'existing' : 'unset'} onchange={(e) => { const index = Number(e.currentTarget.value.slice(7)); if (e.currentTarget.value.startsWith('choice-')) onChange(emptyStringValue((options.choices ?? [])[index], options.emptyAsNull ?? true)); }}>
    {#if value == null}<option value="unset">{m.wpi_select_placeholder()}</option>{/if}
    {#if typeof value === 'string' && !(options.choices ?? []).includes(value)}<option value="existing">{value}</option>{/if}
    {#each options.choices ?? [] as choice, index}<option value={`choice-${index}`}>{choice}</option>{/each}
   </select>
  {:else}
   <div class="languages">
    {#each languages as language, index}
     {@const fieldId = index === 0 ? id : `${id}-${language}`}
     <label for={fieldId} class="language">{language === 'en' ? m.wpi_text_en() : language === 'zh-cn' ? m.wpi_text_zh() : language}</label>
     <div class="field"><DraftField id={fieldId} value={map?.[language]} format={(v) => typeof v === 'string' ? v : ''} parse={parsed} invalidMessage={() => m.wpi_text_invalid()} check={check ? (next) => check({ ...(map ?? {}), [language]: next }) : undefined} commitOnBlur={options.commitOnBlur} onChange={(next) => translate(language, next)} /></div>
    {/each}
   </div>
  {/if}
 </div>
 {#if !options.modeInHeading}<StringModeMenu {value} {mode} {languages} {check} onMode={(next) => (chosen = next)} {onChange} />{/if}
</div>

<style>
 .value { flex: 1; min-width: 0; }
 /* One row per language: its name, then its field. */
 .languages { display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: baseline; gap: 4px 6px; }
 .language { font-size: 10px; color: var(--cv-text-muted, inherit); }
 .field { min-width: 0; }
</style>
