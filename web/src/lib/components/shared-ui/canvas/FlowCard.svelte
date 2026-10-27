<!-- web/src/lib/components/shared-ui/canvas/FlowCard.svelte -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
  import type { Flow } from '$lib/types/flow';

  let {
    flow,
    selected   = false,
    currentUserId,
    onOpen,
    onSetDefault,
    onSaveAsTemplate,
    onDuplicate,
    onDelete,
  }: {
    flow: Flow;
    selected?: boolean;
    currentUserId: number;
    onOpen:          (f: Flow) => void;
    onSetDefault:    (f: Flow) => void;
    onSaveAsTemplate:(f: Flow) => void;
    onDuplicate:     (f: Flow) => void;
    onDelete:        (f: Flow) => void;
  } = $props();

  const isOwner = $derived(flow.user_id === currentUserId);

  let showMenu = $state(false);
  let menuX    = $state(0);
  let menuY    = $state(0);

  function openContextMenu(e: MouseEvent) {
    e.preventDefault();
    menuX = e.clientX;
    menuY = e.clientY;
    showMenu = true;
  }
</script>

<svelte:window onclick={() => { showMenu = false; }} />

<!-- Card -->
<button
  class="w-full text-left rounded-lg border transition-all duration-150 cursor-pointer overflow-hidden"
  style="background: {selected ? 'var(--cv-surface)' : 'var(--cv-raised)'}; border-color: {selected ? 'var(--cv-accent)' : 'var(--cv-divider)'}; padding:0;"
  onclick={() => onOpen(flow)}
  oncontextmenu={openContextMenu}
  aria-label={m.flowcard_open_flow({ flow_name: flow.flow_name })}
>
  <!-- Thumbnail -->
  <div class="w-full flex items-center justify-center overflow-hidden"
    style="height:64px; background:var(--cv-canvas); border-bottom:1px solid var(--cv-divider);">
    {#if flow.thumbnail_svg}
      {@html flow.thumbnail_svg}
    {:else}
      <span style="font-size:11px; color:var(--cv-text-faint);">{m.flowcard_no_preview()}</span>
    {/if}
  </div>
  <!-- Meta -->
  <div style="padding:8px 10px;">
    <div style="font-size:12px; font-weight:500; color:var(--cv-text); margin-bottom:3px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
      {flow.flow_name}
      {#if flow.is_default}<span style="font-size:9px; color:var(--cv-warn); margin-left:4px;">{m.flowcard_default()}</span>{/if}
    </div>
    <div style="font-size:10px; color:var(--cv-text-muted);">
      {m.flowcard_nodes({ nodesCount: flow.flow_data?.nodes?.length ?? 0, value: flow.is_shared ? m.flowcard_shared() : m.flowcard_private() })}
    </div>
  </div>
</button>

<!-- Context menu -->
{#if showMenu}
  <div
    class="fixed z-50 rounded-lg shadow-xl overflow-hidden"
    style="left:{menuX}px; top:{menuY}px; background:var(--cv-surface); border:1px solid var(--cv-border); min-width:160px;"
    role="menu"
  >
    <button class="w-full text-left px-3 py-2 text-sm hover:bg-(--cv-hover)" style="color:var(--cv-text); border:none; cursor:pointer;" onclick={() => { onOpen(flow); showMenu=false; }}>{m.flowcard_open()}</button>
    {#if isOwner}
      <button class="w-full text-left px-3 py-2 text-sm hover:bg-(--cv-hover)" style="color:var(--cv-text); border:none; cursor:pointer;" onclick={() => { onSetDefault(flow); showMenu=false; }}>{m.flowcard_set_as_default()}</button>
      <button class="w-full text-left px-3 py-2 text-sm hover:bg-(--cv-hover)" style="color:var(--cv-text); border:none; cursor:pointer;" onclick={() => { onSaveAsTemplate(flow); showMenu=false; }}>{m.flowcard_save_as_template()}</button>
    {/if}
    <button class="w-full text-left px-3 py-2 text-sm hover:bg-(--cv-hover)" style="color:var(--cv-text); border:none; cursor:pointer;" onclick={() => { onDuplicate(flow); showMenu=false; }}>{m.flowcard_duplicate()}</button>
    {#if isOwner}
      <button class="w-full text-left px-3 py-2 text-sm hover:bg-(--cv-hover)" style="color:#ef4444; border:none; cursor:pointer;" onclick={() => { onDelete(flow); showMenu=false; }}>{m.flowcard_delete()}</button>
    {/if}
  </div>
{/if}
