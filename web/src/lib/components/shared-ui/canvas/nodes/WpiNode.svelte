<!-- web/src/lib/components/shared-ui/canvas/nodes/WpiNode.svelte -->
<!-- One component for the WPI editor's page, element and action nodes; the node type picks the icon. -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, type, data, selected }: { id: string; type: string; data: Record<string,any>; selected?: boolean } = $props();
  const ICONS: Record<string, string> = { 'wpi-page': '📄', 'wpi-element': '🧩', 'wpi-action': '⚡' };
  // An element is placed on a page, not wired to other nodes, so it has no connection points.
  const wired = $derived(type !== 'wpi-element');
</script>
<BaseNode {id} {selected} label={String(data.label ?? '')} icon={ICONS[type] ?? '⬡'}
  inputHandles={wired ? ['in'] : []} outputHandles={wired ? ['out'] : []}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
/>
