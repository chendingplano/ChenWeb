<!-- web/src/lib/components/shared-ui/canvas/nodes/AiAssistantNode.svelte -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'AI Assistant')} icon="🤖"
  inputHandles={['text','context']} outputHandles={['response']}
>
  <div style="padding:6px 10px; font-size:9px; color:var(--cv-text-muted);">
    {data.model ?? ''}
    {#if data.system_prompt}<span style="margin-left:4px;color:var(--cv-text-faint);">{m.aiassistantnode_prompt_set()}</span>{/if}
  </div>
</BaseNode>
