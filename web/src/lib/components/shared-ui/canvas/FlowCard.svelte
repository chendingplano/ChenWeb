<!-- web/src/lib/components/shared-ui/canvas/FlowCard.svelte -->
<script lang="ts">
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
  style="background: {selected ? '#1e2535' : '#161b27'}; border-color: {selected ? '#6366f1' : '#1e2a3a'}; padding:0;"
  onclick={() => onOpen(flow)}
  oncontextmenu={openContextMenu}
  aria-label="Open flow {flow.flow_name}"
>
  <!-- Thumbnail -->
  <div class="w-full flex items-center justify-center overflow-hidden"
    style="height:64px; background:#0d1117; border-bottom:1px solid #1e2a3a;">
    {#if flow.thumbnail_svg}
      {@html flow.thumbnail_svg}
    {:else}
      <span style="font-size:11px; color:#4b5563;">No preview</span>
    {/if}
  </div>
  <!-- Meta -->
  <div style="padding:8px 10px;">
    <div style="font-size:12px; font-weight:500; color:#e2e8f0; margin-bottom:3px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
      {flow.flow_name}
      {#if flow.is_default}<span style="font-size:9px; color:#fbbf24; margin-left:4px;">★ default</span>{/if}
    </div>
    <div style="font-size:10px; color:#6b7280;">
      {flow.flow_data?.nodes?.length ?? 0} nodes ·
      {flow.is_shared ? '🌐 Shared' : '🔒 Private'}
    </div>
  </div>
</button>

<!-- Context menu -->
{#if showMenu}
  <div
    class="fixed z-50 rounded-lg shadow-xl overflow-hidden"
    style="left:{menuX}px; top:{menuY}px; background:#1e2535; border:1px solid #374151; min-width:160px;"
    role="menu"
  >
    <button class="w-full text-left px-3 py-2 text-sm hover:bg-white/5" style="color:#e2e8f0; border:none; cursor:pointer;" onclick={() => { onOpen(flow); showMenu=false; }}>Open</button>
    {#if isOwner}
      <button class="w-full text-left px-3 py-2 text-sm hover:bg-white/5" style="color:#e2e8f0; border:none; cursor:pointer;" onclick={() => { onSetDefault(flow); showMenu=false; }}>Set as default</button>
      <button class="w-full text-left px-3 py-2 text-sm hover:bg-white/5" style="color:#e2e8f0; border:none; cursor:pointer;" onclick={() => { onSaveAsTemplate(flow); showMenu=false; }}>Save as template</button>
    {/if}
    <button class="w-full text-left px-3 py-2 text-sm hover:bg-white/5" style="color:#e2e8f0; border:none; cursor:pointer;" onclick={() => { onDuplicate(flow); showMenu=false; }}>Duplicate</button>
    {#if isOwner}
      <button class="w-full text-left px-3 py-2 text-sm hover:bg-white/5" style="color:#ef4444; border:none; cursor:pointer;" onclick={() => { onDelete(flow); showMenu=false; }}>Delete</button>
    {/if}
  </div>
{/if}
