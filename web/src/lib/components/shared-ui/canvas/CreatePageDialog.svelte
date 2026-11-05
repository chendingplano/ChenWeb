<script lang="ts">
 import { onMount } from 'svelte';
 import { m } from '$lib/paraglide/messages.js';
 import { canvasThemeVars } from './canvas-theme';
 import { appSlug } from './new-page';
 let { darkMode, onClose, onCreate }: { darkMode: boolean; onClose: () => void; onCreate: (name: string, description: string, keywords: string, tags: string) => Promise<boolean> } = $props();
 let dialog: HTMLDialogElement;
 let name = $state('');
 let slug = $derived(appSlug(name));
 let busy = $state(false);
 let description = $state('');
 let keywords = $state('');
 let tags = $state('');
 onMount(() => dialog.showModal());
</script>
<dialog bind:this={dialog} onclose={onClose} class="select-text" style={canvasThemeVars(darkMode)} aria-labelledby="create-page-title">
 <form onsubmit={async (event) => { event.preventDefault(); if (busy || !slug) return; busy = true; try { if (await onCreate(name, description, keywords, tags)) onClose(); } finally { busy = false; } }}>
  <h2 id="create-page-title">{m.wpi_create_page_title()}</h2>
  <label>{m.wpi_create_page_name()}<input bind:value={name} required maxlength="100" /></label>
  {#if name.trim()}<p class="slug" class:bad={!slug}>{slug ? m.wpi_create_page_id({ id: slug }) : m.wpi_create_page_id_hint()}</p>{/if}
  <label>{m.wpi_create_page_description()}<textarea bind:value={description} maxlength="1000"></textarea></label>
  <label>{m.wpi_create_page_keywords()}<input bind:value={keywords} placeholder={m.wpi_create_page_comma_separated()} /></label>
  <label>{m.wpi_create_page_tags()}<input bind:value={tags} placeholder={m.wpi_create_page_comma_separated()} /></label>
  <footer><button type="button" onclick={onClose} disabled={busy}>{m.wpi_create_page_cancel()}</button><button type="submit" disabled={busy || !slug}>{m.wpi_create_page_submit()}</button></footer>
 </form>
</dialog>
<style>
 dialog { margin:auto; width:460px; max-width:calc(100vw - 32px); border:1px solid var(--cv-divider); border-radius:12px; padding:20px; background:var(--cv-panel); color:var(--cv-text); }
 dialog::backdrop { background:#0008; }
 form { display:grid; gap:14px; } h2 { margin:0; font-size:16px; font-weight:600; }
 label { display:grid; gap:5px; font-size:12px; }
 input, textarea { width:100%; background:var(--cv-field); color:var(--cv-text); border:1px solid var(--cv-divider); border-radius:4px; padding:8px; }
 .slug { margin:-8px 0 0; font-size:12px; color:var(--cv-muted, inherit); opacity:.8; } .slug.bad { color:#e5484d; opacity:1; }
 textarea { min-height:70px; resize:vertical; }
 footer { display:flex; justify-content:flex-end; gap:8px; }
 button { border:1px solid var(--cv-divider); background:var(--cv-raised); color:var(--cv-text); padding:7px 12px; border-radius:4px; cursor:pointer; }
 button:disabled { opacity:.45; cursor:default; }
</style>
