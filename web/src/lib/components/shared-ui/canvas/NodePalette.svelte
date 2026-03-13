<!-- web/src/lib/components/shared-ui/canvas/NodePalette.svelte -->
<script lang="ts">
  import type { NodeType } from '$lib/types/flow';

  let {
    nodeTypes  = [],
    darkMode   = true,
    onDragStart,
  }: {
    nodeTypes:   NodeType[];
    darkMode?:   boolean;
    onDragStart: (event: DragEvent, nodeType: NodeType) => void;
  } = $props();

  let search = $state('');

  const categories = $derived(
    [...new Set(nodeTypes.map(n => n.category))]
  );

  function filteredByCategory(cat: string) {
    return nodeTypes.filter(n =>
      n.category === cat &&
      n.label.toLowerCase().includes(search.toLowerCase())
    );
  }
</script>

<aside
  class="flex flex-col h-full overflow-hidden flex-shrink-0"
  style="width:200px; background:#111827; border-right:1px solid #1e2a3a;"
>
  <div style="padding:8px; border-bottom:1px solid #1e2a3a;">
    <div style="font-size:9px; color:#6366f1; letter-spacing:1px; margin-bottom:6px;">NODE PALETTE</div>
    <input
      bind:value={search}
      placeholder="Search nodes..."
      style="width:100%; background:#1e2535; border:1px solid #374151; border-radius:4px; padding:4px 8px; font-size:10px; color:#94a3b8; box-sizing:border-box;"
    />
  </div>

  <div class="flex-1 overflow-y-auto" style="padding:8px; scrollbar-width:thin; scrollbar-color:#374151 transparent;">
    {#each categories as cat}
      {@const items = filteredByCategory(cat)}
      {#if items.length > 0}
        <div style="font-size:8px; color:#4b5563; letter-spacing:1px; margin:8px 0 4px 4px;">{cat.toUpperCase()}</div>
        {#each items as nodeType (nodeType.id)}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            draggable="true"
            ondragstart={(e) => onDragStart(e, nodeType)}
            class="flex items-center gap-2 rounded px-2 py-1.5 mb-1 cursor-grab hover:bg-white/5 transition-colors select-none"
            style="background:#1e2535; border:1px solid transparent; font-size:10px; color:#e2e8f0;"
            title="Drag onto canvas"
          >
            <span style="font-size:14px; flex-shrink:0;">{getIcon(nodeType.icon)}</span>
            {nodeType.label}
          </div>
        {/each}
      {/if}
    {/each}
  </div>
</aside>

<script module lang="ts">
  // Simple icon emoji map — replace with lucide icons if desired
  const ICON_MAP: Record<string, string> = {
    Bot: '🤖', Terminal: '💻', Type: '📝', FileText: '📄',
    File: '📁', Image: '🎬', Wrench: '🔧', Plug: '🔌',
    Globe: '📡', Filter: '📋', GitBranch: '🗂',
  };
  export function getIcon(name: string) { return ICON_MAP[name] ?? '⬡'; }
</script>
