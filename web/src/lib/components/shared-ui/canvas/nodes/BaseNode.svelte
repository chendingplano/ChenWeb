<!-- web/src/lib/components/shared-ui/canvas/nodes/BaseNode.svelte -->
<script lang="ts">
  import { Handle, Position } from '@xyflow/svelte';
  import CopyIcon   from '@lucide/svelte/icons/copy';
  import Trash2Icon from '@lucide/svelte/icons/trash-2';
  import SlidersIcon from '@lucide/svelte/icons/sliders-horizontal';

  let {
    id,
    selected = false,
    label,
    icon         = '⬡',
    inputHandles = [],
    outputHandles= [],
    children,
    onConfigure,
    onDuplicate,
    onDelete,
  }: {
    id:            string;
    selected?:     boolean;
    label:         string;
    icon?:         string;
    inputHandles:  string[];
    outputHandles: string[];
    children?:     import('svelte').Snippet;
    onConfigure:   () => void;
    onDuplicate:   () => void;
    onDelete:      () => void;
  } = $props();
</script>

<div
  class="relative rounded-lg overflow-visible"
  style="background:#161b27; border:2px solid {selected ? '#6366f1' : '#374151'};
    box-shadow:{selected ? '0 0 0 3px rgba(99,102,241,0.2)' : 'none'};
    min-width:160px;"
>
  <!-- Mini toolbar (only when selected) -->
  {#if selected}
    <div
      class="absolute flex gap-1"
      style="top:-30px; left:50%; transform:translateX(-50%); background:#1e2535; border:1px solid #374151; border-radius:6px; padding:3px 6px; white-space:nowrap; z-index:10;"
    >
      <button onclick={onConfigure} title="Configure" style="background:none; border:none; cursor:pointer; color:#94a3b8; padding:1px 3px; border-radius:3px; display:flex; align-items:center;" aria-label="Configure">
        <SlidersIcon class="w-3 h-3" />
      </button>
      <button onclick={onDuplicate} title="Duplicate" style="background:none; border:none; cursor:pointer; color:#94a3b8; padding:1px 3px; border-radius:3px; display:flex; align-items:center;" aria-label="Duplicate">
        <CopyIcon class="w-3 h-3" />
      </button>
      <button onclick={onDelete} title="Delete" style="background:none; border:none; cursor:pointer; color:#ef4444; padding:1px 3px; border-radius:3px; display:flex; align-items:center;" aria-label="Delete">
        <Trash2Icon class="w-3 h-3" />
      </button>
    </div>
  {/if}

  <!-- Input handles -->
  {#each inputHandles as handleId, i}
    <Handle
      type="target"
      position={Position.Left}
      id={`in-${handleId}`}
      style="top:{inputHandles.length === 1 ? '50%' : `${(i + 1) / (inputHandles.length + 1) * 100}%`}; background:#818cf8; border:2px solid #161b27; width:14px; height:14px;"
    />
  {/each}

  <!-- Header -->
  <div style="padding:6px 10px; border-bottom:1px solid #1e2a3a; display:flex; align-items:center; gap:6px; font-size:10px; color:{selected ? '#e2e8f0' : '#94a3b8'};">
    <span style="font-size:13px;">{icon}</span>
    <span style="font-weight:500; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">{label}</span>
  </div>

  <!-- Body (slot for node-specific content) -->
  {#if children}
    {@render children()}
  {/if}

  <!-- Output handles -->
  {#each outputHandles as handleId, i}
    <Handle
      type="source"
      position={Position.Right}
      id={`out-${handleId}`}
      style="top:{outputHandles.length === 1 ? '50%' : `${(i + 1) / (outputHandles.length + 1) * 100}%`}; background:#6366f1; border:2px solid #161b27; width:14px; height:14px;"
    />
  {/each}
</div>
