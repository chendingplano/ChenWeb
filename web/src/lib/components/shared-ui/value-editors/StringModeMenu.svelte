<!-- The string editor's … menu: Single, Multilingual or List. Choosing one
     converts the value. The Properties panel draws it in the property heading,
     after the caption (interpreter-value-editor.md §5.2.3). -->
<script lang="ts">
 import { getLocale } from '$lib/paraglide/runtime';
 import { m } from '$lib/paraglide/messages.js';
 import { convertStringMode, losesTranslations, type StringMode } from './string-value';
 import type { Json } from '$lib/wpi/types';
 let { value, mode, languages, heading = false, check, onMode, onChange }: {
  value: Json | undefined;
  mode: StringMode;
  languages: string[];
  // Drawn after a caption, so the items open to the right.
  heading?: boolean;
  check?: (value: Json) => string | null;
  // Single and List store the same string, so the host keeps which one was chosen.
  onMode: (mode: Exclude<StringMode, 'multilingual'>) => void;
  onChange: (value: Json | undefined) => void;
 } = $props();
 let menu: HTMLDetailsElement;
 function setMode(next: StringMode) {
  const locale = getLocale();
  const converted = convertStringMode(value, next, languages, locale);
  const map = value && typeof value === 'object' && !Array.isArray(value) ? value : null;
  if (mode === 'multilingual' && next !== 'multilingual' && map && losesTranslations(map, languages, locale) && !window.confirm(m.wpi_string_discard_translations({ value: JSON.stringify(converted) }))) return;
  if (check?.(converted)) return;
  if (next !== 'multilingual') onMode(next);
  onChange(converted);
  menu.open = false;
 }
</script>
<details class="mode-menu" class:heading bind:this={menu}>
 <summary aria-label={m.wpi_string_mode()}>…</summary>
 <div class="mode-items">
  <button type="button" aria-pressed={mode === 'single'} onclick={() => setMode('single')}>{m.wpi_string_single()}</button>
  <button type="button" aria-pressed={mode === 'multilingual'} onclick={() => setMode('multilingual')}>{m.wpi_string_multilingual()}</button>
  <button type="button" aria-pressed={mode === 'list'} onclick={() => setMode('list')}>{m.wpi_string_list()}</button>
 </div>
</details>

<style>
 .mode-menu { position: relative; }
 .mode-menu summary { cursor: pointer; list-style: none; padding: 4px 8px; }
 .heading summary { padding: 0 6px; line-height: 1; }
 .mode-items { position: absolute; z-index: 5; right: 0; display: grid; padding: 4px; background: var(--cv-panel); border: 1px solid var(--cv-border); }
 .heading .mode-items { right: auto; left: 0; }
 .mode-items button { text-align: left; padding: 5px 12px; white-space: nowrap; }
</style>
