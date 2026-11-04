<script lang="ts">
 import { onMount } from 'svelte';
 import { m } from '$lib/paraglide/messages.js';
 import { canvasThemeVars } from './canvas-theme';
 let { darkMode, name, onSave, onDiscard, onCancel }: { darkMode: boolean; name: string; onSave: () => void; onDiscard: () => void; onCancel: () => void } = $props();
 let dialog: HTMLDialogElement;
 onMount(() => dialog.showModal());
</script>
<dialog bind:this={dialog} oncancel={(event) => { event.preventDefault(); onCancel(); }} class="select-text" style={canvasThemeVars(darkMode)} aria-labelledby="close-unsaved-title">
 <h2 id="close-unsaved-title">{m.wpi_close_unsaved_title({ name })}</h2>
 <p>{m.wpi_close_unsaved_body()}</p>
 <footer>
  <button type="button" onclick={onCancel}>{m.wpi_create_page_cancel()}</button>
  <button type="button" onclick={onDiscard}>{m.wpi_close_unsaved_discard()}</button>
  <button type="button" class="primary" onclick={onSave}>{m.wpi_close_unsaved_save()}</button>
 </footer>
</dialog>
<style>
 dialog { margin:auto; width:420px; max-width:calc(100vw - 32px); border:1px solid var(--cv-divider); border-radius:12px; padding:20px; background:var(--cv-panel); color:var(--cv-text); }
 dialog::backdrop { background:#0008; }
 h2 { margin:0 0 8px; font-size:16px; font-weight:600; } p { margin:0 0 16px; font-size:13px; }
 footer { display:flex; justify-content:flex-end; gap:8px; }
 button { border:1px solid var(--cv-divider); background:var(--cv-raised); color:var(--cv-text); padding:7px 12px; border-radius:4px; cursor:pointer; }
 button.primary { background:var(--cv-accent); color:#fff; border-color:var(--cv-accent); }
</style>
