<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import { canvasThemeVars } from './canvas-theme';
  let { tree, darkMode, onClose }: { tree: string; darkMode: boolean; onClose: () => void } = $props();
  let backdropPressed = false;
  function focus(node: HTMLElement) { node.focus(); }
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center select-text"
  style="{canvasThemeVars(darkMode)} background:var(--cv-backdrop);"
  onpointerdown={(e) => { backdropPressed = e.target === e.currentTarget; }}
  onclick={(e) => { if (backdropPressed && e.target === e.currentTarget) onClose(); backdropPressed = false; }}
  onkeydown={(e) => { if (e.key === 'Escape') onClose(); }}
  role="dialog" aria-modal="true" aria-labelledby="wpi-tree-title" tabindex="-1">
  <div class="rounded-xl overflow-hidden shadow-2xl" style="background:var(--cv-panel); border:1px solid var(--cv-divider); width:640px; max-width:calc(100vw - 16px); max-height:85vh; display:flex; flex-direction:column;">
    <div style="padding:16px 20px; border-bottom:1px solid var(--cv-divider); display:flex; justify-content:space-between; gap:12px;">
      <div id="wpi-tree-title" style="font-size:15px; font-weight:600; color:var(--cv-text);">{m.wpi_toolbar_view_tree()}</div>
      <button use:focus onclick={onClose} class="select-none" style="color:var(--cv-text-muted); font-size:16px; background:none; border:none; cursor:pointer;" aria-label={m.wpi_view_json_close()}>✕</button>
    </div>
    <pre style="margin:0; padding:16px 20px; overflow:auto; color:var(--cv-text); font:12px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace;">{tree}</pre>
  </div>
</div>
