<!-- web/src/lib/components/shared-ui/canvas/PropertiesPanel.svelte -->
<script lang="ts">
  import type { FlowNode, NodeType } from '$lib/types/flow';

  let {
    node,
    nodeType,
    darkMode = true,
    onUpdate,
  }: {
    node:      FlowNode | null;
    nodeType:  NodeType | null;
    darkMode?: boolean;
    onUpdate:  (nodeId: string, data: Record<string, unknown>) => void;
  } = $props();

  // Map of attribute → UI control type
  const SELECT_ATTRS: Record<string, string[]> = {
    model:      ['gpt-4o', 'gpt-4o-mini', 'claude-opus-4-6', 'claude-sonnet-4-6', 'claude-haiku-4-5'],
    language:   ['typescript', 'python', 'go', 'rust', 'java', 'c++', 'bash'],
    file_type:  ['txt', 'csv', 'json', 'pdf', 'md', 'html'],
    media_type: ['image', 'audio', 'video'],
    method:     ['GET', 'POST', 'PUT', 'DELETE', 'PATCH'],
    operation:  ['status', 'clone', 'pull', 'push', 'commit', 'diff'],
  };
  const MASKED_ATTRS  = new Set(['auth_token']);
  const TEXTAREA_ATTRS = new Set(['system_prompt', 'content', 'tool_config', 'headers', 'rule_expression']);

  function update(key: string, value: unknown) {
    if (!node) return;
    onUpdate(node.id, { ...node.data, [key]: value });
  }
</script>

<aside
  class="flex flex-col h-full overflow-hidden flex-shrink-0"
  style="width:240px; background:#111827; border-left:1px solid #1e2a3a;"
>
  {#if !node || !nodeType}
    <div class="flex-1 flex items-center justify-center" style="color:#4b5563; font-size:12px;">
      Select a node to edit its properties
    </div>
  {:else}
    <div style="padding:12px 12px 0; border-bottom:1px solid #1e2a3a;">
      <div style="font-size:9px; color:#6366f1; letter-spacing:1px; margin-bottom:8px;">PROPERTIES</div>
      <!-- Node name -->
      <div style="margin-bottom:10px;">
        <label style="font-size:9px; color:#6b7280; display:block; margin-bottom:3px;">Name</label>
        <input
          type="text"
          maxlength="100"
          value={String(node.data.label ?? nodeType.label)}
          oninput={(e) => update('label', (e.target as HTMLInputElement).value)}
          style="width:100%; background:#1e2535; border:1px solid #374151; border-radius:4px; padding:4px 8px; font-size:10px; color:#e2e8f0; box-sizing:border-box;"
        />
      </div>
    </div>

    <div class="flex-1 overflow-y-auto" style="padding:10px 12px; scrollbar-width:thin; scrollbar-color:#374151 transparent;">
      <!-- Attributes -->
      {#each Object.entries(nodeType.defaultData) as [key]}
        <div style="margin-bottom:10px;">
          <label style="font-size:9px; color:#6b7280; display:block; margin-bottom:3px; text-transform:capitalize;">
            {key.replace(/_/g, ' ')}
          </label>
          {#if SELECT_ATTRS[key]}
            <select
              value={String(node.data[key] ?? nodeType.defaultData[key])}
              onchange={(e) => update(key, (e.target as HTMLSelectElement).value)}
              style="width:100%; background:#1e2535; border:1px solid #374151; border-radius:4px; padding:4px 8px; font-size:10px; color:#e2e8f0;"
            >
              {#each SELECT_ATTRS[key] as opt}
                <option value={opt}>{opt}</option>
              {/each}
            </select>
          {:else if key === 'temperature'}
            <div class="flex items-center gap-2">
              <input type="range" min="0" max="2" step="0.1"
                value={Number(node.data[key] ?? nodeType.defaultData[key])}
                oninput={(e) => update(key, parseFloat((e.target as HTMLInputElement).value))}
                style="flex:1;"
              />
              <span style="font-size:10px; color:#94a3b8; min-width:24px;">{node.data[key] ?? nodeType.defaultData[key]}</span>
            </div>
          {:else if TEXTAREA_ATTRS.has(key)}
            <textarea
              rows="4"
              value={String(node.data[key] ?? nodeType.defaultData[key] ?? '')}
              oninput={(e) => update(key, (e.target as HTMLTextAreaElement).value)}
              style="width:100%; background:#1e2535; border:1px solid #374151; border-radius:4px; padding:4px 8px; font-size:10px; color:#e2e8f0; resize:vertical; box-sizing:border-box;"
            ></textarea>
          {:else if MASKED_ATTRS.has(key)}
            <input type="password"
              value={String(node.data[key] ?? '')}
              oninput={(e) => update(key, (e.target as HTMLInputElement).value)}
              style="width:100%; background:#1e2535; border:1px solid #374151; border-radius:4px; padding:4px 8px; font-size:10px; color:#e2e8f0; box-sizing:border-box;"
            />
          {:else}
            <input type="text"
              value={String(node.data[key] ?? '')}
              oninput={(e) => update(key, (e.target as HTMLInputElement).value)}
              style="width:100%; background:#1e2535; border:1px solid #374151; border-radius:4px; padding:4px 8px; font-size:10px; color:#e2e8f0; box-sizing:border-box;"
            />
          {/if}
        </div>
      {/each}

      <!-- Connectors (read-only) -->
      <div style="margin-top:8px;">
        <div style="font-size:9px; color:#6b7280; margin-bottom:6px;">CONNECTORS</div>
        {#each nodeType.inputs as inp}
          <div style="background:#1e2535; border-radius:4px; padding:4px 8px; margin-bottom:3px; font-size:9px; color:#818cf8;">← in: {inp}</div>
        {/each}
        {#each nodeType.outputs as out}
          <div style="background:#1e2535; border-radius:4px; padding:4px 8px; margin-bottom:3px; font-size:9px; color:#6366f1;">→ out: {out}</div>
        {/each}
      </div>
    </div>
  {/if}
</aside>
