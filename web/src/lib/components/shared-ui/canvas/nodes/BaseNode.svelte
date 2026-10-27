<!-- web/src/lib/components/shared-ui/canvas/nodes/BaseNode.svelte -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
  import { Handle, Position, NodeResizer } from '@xyflow/svelte';
  import CopyIcon   from '@lucide/svelte/icons/copy';
  import Trash2Icon from '@lucide/svelte/icons/trash-2';
  import PencilIcon from '@lucide/svelte/icons/pencil';
  import { getNodeActions } from '../node-actions';

  let {
    id,
    selected = false,
    label,
    icon         = '⬡',
    inputHandles = [],
    outputHandles= [],
    children,
    onEdit,
  }: {
    id:            string;
    selected?:     boolean;
    label:         string;
    icon?:         string;
    inputHandles:  string[];
    outputHandles: string[];
    children?:     import('svelte').Snippet;
    // Shown as an edit button when given: the node's class can be edited.
    onEdit?:       () => void;
  } = $props();

  // Duplicate and Delete change the canvas that holds the node, which provides them.
  const actions = getNodeActions();

  // A tool bar click must not reach Svelte Flow's node click, which would select the node again.
  function run(e: MouseEvent, fn: () => void) {
    e.stopPropagation();
    fn();
  }
</script>

<div
  class="relative rounded-lg overflow-visible"
  style="background:var(--cv-raised); border:2px solid {selected ? 'var(--cv-accent)' : 'var(--cv-border)'};
    box-shadow:{selected ? '0 0 0 3px rgba(99,102,241,0.2)' : 'none'};
    min-width:160px; min-height:60px; width:100%; height:100%;"
>
  <NodeResizer
    isVisible={selected}
    minWidth={160}
    minHeight={60}
    handleStyle="background:var(--cv-accent); border:1px solid var(--cv-raised); border-radius:2px; width:8px; height:8px;"
    lineStyle="border-color:#6366f140;"
  />
  <!-- Mini toolbar (only when selected) -->
  {#if selected && (actions || onEdit)}
    <div
      class="nodrag absolute flex gap-1"
      style="top:-30px; left:50%; transform:translateX(-50%); background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:6px; padding:3px 6px; white-space:nowrap; z-index:10;"
    >
      {#if onEdit}
        <button onclick={(e) => run(e, () => onEdit?.())} title={m.basenode_edit_class()} style="background:none; border:none; cursor:pointer; color:var(--cv-text-2); padding:1px 3px; border-radius:3px; display:flex; align-items:center;" aria-label={m.basenode_edit_class()}>
          <PencilIcon class="w-3 h-3" />
        </button>
      {/if}
      {#if actions}
        <button onclick={(e) => run(e, () => actions.duplicate(id))} title={m.basenode_duplicate()} style="background:none; border:none; cursor:pointer; color:var(--cv-text-2); padding:1px 3px; border-radius:3px; display:flex; align-items:center;" aria-label={m.basenode_duplicate()}>
          <CopyIcon class="w-3 h-3" />
        </button>
        <button onclick={(e) => run(e, () => actions.remove(id))} title={m.basenode_delete()} style="background:none; border:none; cursor:pointer; color:#ef4444; padding:1px 3px; border-radius:3px; display:flex; align-items:center;" aria-label={m.basenode_delete()}>
          <Trash2Icon class="w-3 h-3" />
        </button>
      {/if}
    </div>
  {/if}

  <!-- Input handles -->
  {#each inputHandles as handleId, i}
    <Handle
      type="target"
      position={Position.Left}
      id={`in-${handleId}`}
      style="top:{inputHandles.length === 1 ? '50%' : `${(i + 1) / (inputHandles.length + 1) * 100}%`}; background:var(--cv-accent-2); border:2px solid var(--cv-raised); width:14px; height:14px;"
    />
  {/each}

  <!-- Header -->
  <div style="padding:6px 10px; border-bottom:1px solid var(--cv-divider); display:flex; align-items:center; gap:6px; font-size:10px; color:{selected ? 'var(--cv-text)' : 'var(--cv-text-2)'};">
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
      style="top:{outputHandles.length === 1 ? '50%' : `${(i + 1) / (outputHandles.length + 1) * 100}%`}; background:var(--cv-accent); border:2px solid var(--cv-raised); width:14px; height:14px;"
    />
  {/each}
</div>
