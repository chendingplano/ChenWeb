<script lang="ts">
 import { onMount } from 'svelte';
 import { m } from '$lib/paraglide/messages.js';
 import { canvasThemeVars } from './canvas-theme';
 let { darkMode, onClose, onCreate }: { darkMode: boolean; onClose: () => void; onCreate: (id: string, name: string, description: string, keywords: string, tags: string) => Promise<boolean> } = $props();
 let dialog: HTMLDialogElement;
 let id = $state('');
 let busy = $state(false);
 let name = $state('');
 let description = $state('');
 let keywords = $state('');
 let tags = $state('');
 onMount(() => dialog.showModal());
</script>
<dialog bind:this={dialog} onclose={onClose} class="select-text" style={canvasThemeVars(darkMode)} aria-labelledby="create-page-title">
 <form onsubmit={async (event) => { event.preventDefault(); if (busy || !name.trim() || !id.trim()) return; busy = true; try { if (await onCreate(id.trim(), name, description, keywords, tags)) onClose(); } finally { busy = false; } }}>
  <h2 id="create-page-title">{m.wpi_create_page_title()}</h2>
  <label>{m.wpi_create_page_id()}<input bind:value={id} required maxlength="128" pattern="[a-z0-9][a-z0-9-]*" title={m.wpi_create_page_id_hint()} autofocus /></label>
  <label>{m.wpi_create_page_name()}<input bind:value={name} required maxlength="100" /></label>
  <label>{m.wpi_create_page_description()}<textarea bind:value={description} maxlength="1000"></textarea></label>
  <label>{m.wpi_create_page_keywords()}<input bind:value={keywords} placeholder={m.wpi_create_page_comma_separated()} /></label>
  <label>{m.wpi_create_page_tags()}<input bind:value={tags} placeholder={m.wpi_create_page_comma_separated()} /></label>
  <footer><button type="button" onclick={onClose} disabled={busy}>{m.wpi_create_page_cancel()}</button><button type="submit" disabled={busy || !name.trim() || !id.trim()}>{m.wpi_create_page_submit()}</button></footer>
 </form>
</dialog>
<style>
 dialog { margin:auto; width:460px; max-width:calc(100vw - 32px); border:1px solid var(--cv-divider); border-radius:12px; padding:20px; background:var(--cv-panel); color:var(--cv-text); }
 dialog::backdrop { background:#0008; }
 form { display:grid; gap:14px; } h2 { margin:0; font-size:16px; font-weight:600; }
 label { display:grid; gap:5px; font-size:12px; }
 input, textarea { width:100%; background:var(--cv-field); color:var(--cv-text); border:1px solid var(--cv-divider); border-radius:4px; padding:8px; }
 textarea { min-height:70px; resize:vertical; }
 footer { display:flex; justify-content:flex-end; gap:8px; }
 button { border:1px solid var(--cv-divider); background:var(--cv-raised); color:var(--cv-text); padding:7px 12px; border-radius:4px; cursor:pointer; }
 button:disabled { opacity:.45; cursor:default; }
</style>
