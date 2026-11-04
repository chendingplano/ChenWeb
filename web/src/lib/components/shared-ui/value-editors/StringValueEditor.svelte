<script lang="ts">
 import { untrack } from 'svelte';
 import { getLocale } from '$lib/paraglide/runtime';
 import { m } from '$lib/paraglide/messages.js';
 import DraftField from './DraftField.svelte';
 import { jsonText } from './values';
 import { convertStringMode, emptyStringValue, losesTranslations, type StringMode } from './string-value';
 import type { ValueEditorProps } from './types';
 import type { Json } from '$lib/wpi/types';
 import './value-editors.css';
 let { id, label, value, options, check, onChange }: ValueEditorProps = $props();
 const languages = $derived(options.languages?.length ? options.languages : ['en', 'zh-cn']);
 const locale = $derived(getLocale());
 const map = $derived(value && typeof value === 'object' && !Array.isArray(value) ? value : null);
 let mode = $state<StringMode>(untrack(() => value && typeof value === 'object' && !Array.isArray(value) ? 'multilingual' : options.mode ?? 'single'));
 let menu: HTMLDetailsElement;
 $effect(() => {
  const isMap = !!value && typeof value === 'object' && !Array.isArray(value);
  untrack(() => {
   if (isMap && mode !== 'multilingual') mode = 'multilingual';
   else if (!isMap && mode === 'multilingual') mode = options.mode === 'list' ? 'list' : 'single';
  });
 });
 const parsed = (text: string) => ({ ok: true as const, value: emptyStringValue(text, options.emptyAsNull ?? true) });
 function setMode(next: StringMode) {
  const converted = convertStringMode(value, next, languages, locale);
  if (mode === 'multilingual' && next !== 'multilingual' && map && losesTranslations(map, languages, locale) && !window.confirm(m.wpi_string_discard_translations({ value: JSON.stringify(converted) }))) return;
  if (check?.(converted)) return;
  mode = next;
  onChange(converted);
  menu.open = false;
 }
 function translate(language: string, next: Json | undefined) {
  const updated = { ...(map ?? {}), [language]: next ?? null };
  if (!check?.(updated)) onChange(updated);
 }
</script>
<div class="ve-row">
 {#if mode === 'single'}
  <DraftField {id} {value} ariaLabel={label} format={(v) => typeof v === 'string' ? v : v == null ? '' : jsonText(v)} parse={parsed} invalidMessage={() => m.wpi_text_invalid()} {check} commitOnBlur={options.commitOnBlur} {onChange} />
 {:else if mode === 'list'}
  <select {id} class="ve-input" aria-label={label} value={typeof value === 'string' ? (options.choices ?? []).includes(value) ? `choice-${(options.choices ?? []).indexOf(value)}` : 'existing' : 'unset'} onchange={(e) => { const index = Number(e.currentTarget.value.slice(7)); if (e.currentTarget.value.startsWith('choice-')) onChange(emptyStringValue((options.choices ?? [])[index], options.emptyAsNull ?? true)); }}>
   {#if value == null}<option value="unset">{m.wpi_select_placeholder()}</option>{/if}
   {#if typeof value === 'string' && !(options.choices ?? []).includes(value)}<option value="existing">{value}</option>{/if}
   {#each options.choices ?? [] as choice, index}<option value={`choice-${index}`}>{choice}</option>{/each}
  </select>
 {:else}
  <div>
   {#each languages as language, index}
    <label for={index === 0 ? id : `${id}-${language}`} class="ve-sub">{language === 'en' ? m.wpi_text_en() : language === 'zh-cn' ? m.wpi_text_zh() : language}</label>
    <DraftField id={index === 0 ? id : `${id}-${language}`} value={map?.[language]} format={(v) => typeof v === 'string' ? v : ''} parse={parsed} invalidMessage={() => m.wpi_text_invalid()} check={check ? (next) => check({ ...(map ?? {}), [language]: next }) : undefined} commitOnBlur={options.commitOnBlur} onChange={(next) => translate(language, next)} />
   {/each}
  </div>
 {/if}
 <details class="mode-menu" bind:this={menu}>
  <summary aria-label={m.wpi_string_mode()}>…</summary>
  <div class="mode-items">
   <button type="button" aria-pressed={mode === 'single'} onclick={() => setMode('single')}>{m.wpi_string_single()}</button>
   <button type="button" aria-pressed={mode === 'multilingual'} onclick={() => setMode('multilingual')}>{m.wpi_string_multilingual()}</button>
   <button type="button" aria-pressed={mode === 'list'} onclick={() => setMode('list')}>{m.wpi_string_list()}</button>
  </div>
 </details>
</div>

<style>
 .mode-menu { position: relative; }
 .mode-menu summary { cursor: pointer; list-style: none; padding: 4px 8px; }
 .mode-items { position: absolute; z-index: 5; right: 0; display: grid; padding: 4px; background: var(--cv-panel); border: 1px solid var(--cv-border); }
 .mode-items button { text-align: left; padding: 5px 12px; white-space: nowrap; }
</style>
