<!-- web/src/lib/components/shared-ui/canvas/WpiJsonDialog.svelte -->
<!-- The WPI editor's View JSON: the descriptor JSON of the selected node or of the page, read-only. -->
<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import type { JsonObject } from '$lib/wpi/types';
  import { canvasThemeVars } from './canvas-theme';

  let {
    title,
    note = '',
    json,
    darkMode = true,
    onClose,
  }: {
    title:     string;
    note?:     string;
    json:      JsonObject;
    darkMode?: boolean;
    onClose:   () => void;
  } = $props();

  function focus(node: HTMLElement) { node.focus(); }
</script>

<!-- Backdrop. It renders outside the editor's root, so it sets the --cv-* colors itself. -->
<div class="fixed inset-0 z-50 flex items-center justify-center select-text"
  style="{canvasThemeVars(darkMode)} background: var(--cv-backdrop);"
  onclick={(e) => { if (e.target === e.currentTarget) onClose(); }}
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
      <button use:focus onclick={onClose} style="color:var(--cv-text-muted); font-size:16px; background:none; border:none; cursor:pointer;" aria-label={m.wpi_view_json_close()}>✕</button>
    </div>
    <pre style="flex:1; overflow:auto; margin:0; padding:16px 20px; font-size:11px; line-height:1.5; color:var(--cv-text);
      font-family:ui-monospace, SFMono-Regular, Menlo, monospace;">{JSON.stringify(json, null, 2)}</pre>
  </div>
</div>
