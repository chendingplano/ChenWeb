<!-- web/src/lib/components/shared-ui/canvas/WpiJsonDialog.svelte -->
<!-- The WPI editor's View JSON: the descriptor JSON of the selected node or of the page, with editing and syntax checking. -->
<script lang="ts">
  import { untrack } from 'svelte';
  import { m } from '$lib/paraglide/messages.js';
  import type { JsonObject } from '$lib/wpi/types';
  import { parseJsonObject } from './wpi-json';
  import { canvasThemeVars } from './canvas-theme';

  let {
    title,
    note = '',
    json,
    darkMode = true,
    onClose,
    onApply,
  }: {
    title:     string;
    note?:     string;
    json:      JsonObject;
    darkMode?: boolean;
    onClose:   () => void;
    onApply:   (value: JsonObject) => boolean;
  } = $props();

  let editing = $state(false);
  let text = $state(JSON.stringify(untrack(() => json), null, 2));
  let status = $state('');
  let valid = $state(false);
  let backdropPressed = false;
  let gutter: HTMLPreElement;
  let lineNumbers = $derived(text.split('\n').map((_, index) => index + 1).join('\n'));

  function syncScroll(event: Event) {
    if (gutter) gutter.scrollTop = (event.currentTarget as HTMLElement).scrollTop;
  }

  function check() {
    try {
      JSON.parse(text);
      valid = true;
      status = m.wpi_json_valid();
    } catch (error) {
      valid = false;
      status = m.wpi_json_invalid({ error: error instanceof Error ? error.message : String(error) });
    }
  }

  function apply() {
    check();
    if (!valid) return;
    try {
      const value = parseJsonObject(text);
      if (!onApply(value)) { status = m.wpi_json_invalid_descriptor(); valid = false; return; }
      onClose();
    } catch {
      status = m.wpi_json_object_required();
      valid = false;
    }
  }

  function focus(node: HTMLElement) { node.focus(); }
</script>

<!-- Backdrop. It renders outside the editor's root, so it sets the --cv-* colors itself. -->
<div class="fixed inset-0 z-50 flex items-center justify-center select-text"
  style="{canvasThemeVars(darkMode)} background: var(--cv-backdrop);"
  onpointerdown={(e) => { backdropPressed = e.target === e.currentTarget; }}
  onclick={(e) => { if (backdropPressed && e.target === e.currentTarget) onClose(); backdropPressed = false; }}
  onkeydown={(e) => { if (e.key === 'Escape') onClose(); }}
  role="dialog" aria-modal="true" aria-labelledby="wpi-json-title"
  tabindex="-1"
>
  <div class="rounded-xl overflow-hidden shadow-2xl"
    style="background:var(--cv-panel); border:1px solid var(--cv-divider); display:flex; flex-direction:column;
      width:640px; max-width:calc(100vw - 16px); max-height:85vh;">
    <div style="padding:16px 20px; border-bottom:1px solid var(--cv-divider); display:flex; justify-content:space-between; align-items:flex-start; gap:12px;">
      <div>
        <div id="wpi-json-title" style="font-size:15px; font-weight:600; color:var(--cv-text);">{title}</div>
        {#if note}
          <div style="font-size:11px; color:var(--cv-text-muted); margin-top:2px;">{note}</div>
        {/if}
      </div>
      <button use:focus onclick={onClose} class="select-none" style="color:var(--cv-text-muted); font-size:16px; background:none; border:none; cursor:pointer;" aria-label={m.wpi_view_json_close()}>✕</button>
    </div>
    <div style="padding:10px 20px; display:flex; align-items:center; gap:8px; border-bottom:1px solid var(--cv-divider);">
      <button class="json-action" onclick={() => { editing = true; status = ''; }} disabled={editing}>{m.wpi_json_edit()}</button>
      <button class="json-action" onclick={check}>{m.wpi_json_check()}</button>
      {#if editing}<button class="json-action" onclick={apply}>{m.wpi_json_apply()}</button>{/if}
      <span role="status" style="font-size:12px; color:{valid ? 'var(--cv-text)' : '#ef7777'};">{status}</span>
    </div>
    <div class="json-content">
      <pre class="line-numbers" bind:this={gutter} aria-hidden="true">{lineNumbers}</pre>
      {#if editing}
        <textarea class="json-text" aria-label={m.wpi_json_edit()} bind:value={text} oninput={() => (status = '')} onscroll={syncScroll} spellcheck="false"></textarea>
      {:else}
        <pre class="json-text" onscroll={syncScroll}>{text}</pre>
      {/if}
    </div>
  </div>
</div>

<style>
  .json-content {
    display: flex;
    height: 60vh;
    min-height: 0;
    overflow: hidden;
    font: 11px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace;
  }
  .line-numbers, .json-text {
    box-sizing: border-box;
    margin: 0;
    padding: 16px 0;
    font: inherit;
    line-height: inherit;
  }
  .line-numbers {
    flex: none;
    overflow: hidden;
    padding-left: 16px;
    padding-right: 12px;
    text-align: right;
    color: var(--cv-text-muted);
    opacity: 0.6;
    user-select: none;
  }
  .json-text {
    flex: 1;
    min-width: 0;
    overflow: auto;
    padding-right: 20px;
    color: var(--cv-text);
    background: var(--cv-panel);
    border: 0;
    resize: none;
    white-space: pre;
    tab-size: 2;
  }
  .json-action {
    background: var(--cv-surface);
    color: var(--cv-text);
    border: 1px solid var(--cv-border);
    border-radius: 5px;
    padding: 4px 10px;
    cursor: pointer;
  }
  .json-action:hover:not(:disabled) { background: var(--cv-surface-hover); }
  .json-action:disabled { opacity: 0.6; cursor: default; }
</style>
