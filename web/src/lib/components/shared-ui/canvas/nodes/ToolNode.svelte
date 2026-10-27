<!-- web/src/lib/components/shared-ui/canvas/nodes/ToolNode.svelte -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'Tool')} icon="🔧"
  inputHandles={['args']} outputHandles={['result']}
>
  <div style="padding:6px 10px; font-size:9px; color:var(--cv-text-muted);">
    {data.tool_name ?? m.toolnode_unnamed_tool()}
  </div>
</BaseNode>
