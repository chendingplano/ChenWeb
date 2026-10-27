<!-- web/src/lib/components/shared-ui/canvas/WpiCanvasToolbar.svelte -->
<!-- Tool bar above the WPI editor canvas: create nodes, load a page, align and distribute the selection. -->
<script lang="ts" module>
  export type WpiNodeKind = 'page' | 'element' | 'action';
</script>

<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
  import type { AlignMode, DistributeAxis } from './align-nodes';
  import PlusIcon                 from '@lucide/svelte/icons/plus';
  import ChevronDownIcon          from '@lucide/svelte/icons/chevron-down';
  import FileIcon                 from '@lucide/svelte/icons/file';
  import SquareMousePointerIcon   from '@lucide/svelte/icons/square-mouse-pointer';
  import ZapIcon                  from '@lucide/svelte/icons/zap';
  import FolderOpenIcon           from '@lucide/svelte/icons/folder-open';
  import AlignStartVerticalIcon   from '@lucide/svelte/icons/align-start-vertical';
  import AlignCenterVerticalIcon  from '@lucide/svelte/icons/align-center-vertical';
  import AlignEndVerticalIcon     from '@lucide/svelte/icons/align-end-vertical';
  import AlignStartHorizontalIcon from '@lucide/svelte/icons/align-start-horizontal';
  import AlignCenterHorizontalIcon from '@lucide/svelte/icons/align-center-horizontal';
  import AlignEndHorizontalIcon   from '@lucide/svelte/icons/align-end-horizontal';
  import DistributeHorizontalIcon from '@lucide/svelte/icons/align-horizontal-distribute-center';
  import DistributeVerticalIcon   from '@lucide/svelte/icons/align-vertical-distribute-center';

  let {
    selectedCount,
    onCreate,
    onLoadPage,
    onAlign,
    onDistribute,
  }: {
    selectedCount: number;
    onCreate:      (kind: WpiNodeKind) => void;
    onLoadPage:    () => void;
    onAlign:       (mode: AlignMode) => void;
    onDistribute:  (axis: DistributeAxis) => void;
  } = $props();

  const alignTools = $derived([
    { mode: 'left'   as AlignMode, label: m.wpi_toolbar_align_left(),   Icon: AlignStartVerticalIcon },
    { mode: 'center' as AlignMode, label: m.wpi_toolbar_align_center(), Icon: AlignCenterVerticalIcon },
    { mode: 'right'  as AlignMode, label: m.wpi_toolbar_align_right(),  Icon: AlignEndVerticalIcon },
    { mode: 'top'    as AlignMode, label: m.wpi_toolbar_align_top(),    Icon: AlignStartHorizontalIcon },
    { mode: 'middle' as AlignMode, label: m.wpi_toolbar_align_middle(), Icon: AlignCenterHorizontalIcon },
    { mode: 'bottom' as AlignMode, label: m.wpi_toolbar_align_bottom(), Icon: AlignEndHorizontalIcon },
  ]);
  const distributeTools = $derived([
    { axis: 'horizontal' as DistributeAxis, label: m.wpi_toolbar_distribute_horizontal(), Icon: DistributeHorizontalIcon },
    { axis: 'vertical'   as DistributeAxis, label: m.wpi_toolbar_distribute_vertical(),   Icon: DistributeVerticalIcon },
  ]);

  // The editor is always dark, so the menu does not follow the site theme.
  const itemClass = 'text-[12px] text-[#e2e8f0] data-highlighted:bg-white/5 data-highlighted:text-[#e2e8f0]';
</script>

<div class="flex items-center gap-1 px-2 flex-shrink-0 overflow-x-auto"
  style="height:40px; background:#161b27; border-bottom:1px solid #1e2a3a;"
  role="toolbar" aria-label={m.wpi_toolbar_label()}>

  <!-- Create nodes -->
  <DropdownMenu.Root>
    <DropdownMenu.Trigger>
      {#snippet child({ props })}
        <button {...props} class="toolbar-btn" style="color:#e2e8f0; gap:4px;">
          <PlusIcon class="w-4 h-4" />
          <span style="font-size:11px;">{m.wpi_toolbar_create_nodes()}</span>
          <ChevronDownIcon class="w-3 h-3" />
        </button>
      {/snippet}
    </DropdownMenu.Trigger>
    <DropdownMenu.Content align="start" class="min-w-40 bg-[#1e2535] border-[#374151]">
      <DropdownMenu.Item class={itemClass} onclick={() => onCreate('page')}>
        <FileIcon class="w-4 h-4 text-[#94a3b8]" />{m.wpi_toolbar_create_page()}
      </DropdownMenu.Item>
      <DropdownMenu.Item class={itemClass} onclick={() => onCreate('element')}>
        <SquareMousePointerIcon class="w-4 h-4 text-[#94a3b8]" />{m.wpi_toolbar_create_element()}
      </DropdownMenu.Item>
      <DropdownMenu.Item class={itemClass} onclick={() => onCreate('action')}>
        <ZapIcon class="w-4 h-4 text-[#94a3b8]" />{m.wpi_toolbar_create_action()}
      </DropdownMenu.Item>
    </DropdownMenu.Content>
  </DropdownMenu.Root>

  <!-- Load page -->
  <button onclick={onLoadPage} class="toolbar-btn" style="gap:4px;">
    <FolderOpenIcon class="w-4 h-4" />
    <span style="font-size:11px;">{m.wpi_toolbar_load_page()}</span>
  </button>

  <div style="width:1px; height:20px; background:#374151; margin:0 4px;"></div>

  <!-- Align (two or more selected) -->
  {#each alignTools as tool (tool.mode)}
    <button onclick={() => onAlign(tool.mode)} disabled={selectedCount < 2}
      title={tool.label} aria-label={tool.label} class="toolbar-btn">
      <tool.Icon class="w-4 h-4" />
    </button>
  {/each}

  <div style="width:1px; height:20px; background:#374151; margin:0 4px;"></div>

  <!-- Distribute (three or more selected) -->
  {#each distributeTools as tool (tool.axis)}
    <button onclick={() => onDistribute(tool.axis)} disabled={selectedCount < 3}
      title={tool.label} aria-label={tool.label} class="toolbar-btn">
      <tool.Icon class="w-4 h-4" />
    </button>
  {/each}
</div>

<style>
  .toolbar-btn {
    display: flex; align-items: center; flex-shrink: 0;
    background: none; border: none; cursor: pointer;
    color: #94a3b8; border-radius: 4px; padding: 4px 6px;
    transition: background 0.1s;
  }
  .toolbar-btn:hover:not(:disabled) { background: rgba(255,255,255,0.05); }
  .toolbar-btn:disabled { opacity: 0.35; cursor: not-allowed; }
</style>
