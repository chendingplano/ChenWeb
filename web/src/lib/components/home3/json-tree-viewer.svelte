<script lang="ts">
    import JsonTreeViewer from './json-tree-viewer.svelte';
    let { value, depth = 0, darkMode = true }: { value: unknown; depth?: number; darkMode?: boolean } = $props();

    let collapsed = $state(depth > 1);

    let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
    let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
    let keyColor = $derived(darkMode ? '#93C5FD' : '#2563EB');
    let stringColor = $derived(darkMode ? '#86EFAC' : '#16A34A');
    let numberColor = $derived(darkMode ? '#FCA5A5' : '#DC2626');
    let boolColor = $derived(darkMode ? '#FDBA74' : '#D97706');
    let nullColor = $derived(darkMode ? '#94A3B8' : '#9CA3AF');
    let bracketColor = $derived(darkMode ? '#94A3B8' : '#6B7280');
    let indentColor = $derived(darkMode ? '#2D3348' : '#E5E7EB');

    let isObject = $derived(value !== null && typeof value === 'object' && !Array.isArray(value));
    let isArray = $derived(Array.isArray(value));
    let isExpandable = $derived(isObject || isArray);
    let entries = $derived(isObject ? Object.entries(value as Record<string, unknown>) : isArray ? (value as unknown[]).map((v, i) => [String(i), v] as [string, unknown]) : []);
    let isEmpty = $derived(entries.length === 0);
    let openBracket = $derived(isArray ? '[' : '{');
    let closeBracket = $derived(isArray ? ']' : '}');
    let collapsedPreview = $derived(
        isEmpty
            ? (isArray ? '[]' : '{}')
            : isArray
                ? `[ ${entries.length} item${entries.length !== 1 ? 's' : ''} ]`
                : `{ ${entries.length} key${entries.length !== 1 ? 's' : ''} }`
    );

    function formatScalar(v: unknown): { text: string; color: string } {
        if (v === null) return { text: 'null', color: nullColor };
        if (typeof v === 'boolean') return { text: String(v), color: boolColor };
        if (typeof v === 'number') return { text: String(v), color: numberColor };
        if (typeof v === 'string') return { text: `"${v}"`, color: stringColor };
        return { text: String(v), color: textPrimary };
    }
</script>

{#if isExpandable}
    <span>
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <span
            onclick={() => { if (!isEmpty) collapsed = !collapsed; }}
            onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); if (!isEmpty) collapsed = !collapsed; } }}
            role="button"
            tabindex="0"
            style="cursor: {isEmpty ? 'default' : 'pointer'}; user-select: none; display: inline-flex; align-items: center; gap: 2px;"
        >
            {#if !isEmpty}
                <span style="display: inline-block; width: 14px; height: 14px; line-height: 14px; text-align: center; font-size: 0.7rem; color: {bracketColor}; transition: transform 0.15s; transform: {collapsed ? 'rotate(0deg)' : 'rotate(90deg)'}; vertical-align: middle;">▶</span>
            {:else}
                <span style="display: inline-block; width: 14px;"></span>
            {/if}
            {#if collapsed || isEmpty}
                <span style="color: {bracketColor};">{collapsedPreview}</span>
            {:else}
                <span style="color: {bracketColor};">{openBracket}</span>
            {/if}
        </span>
        {#if !collapsed && !isEmpty}
            <div style="margin-left: {depth === 0 ? 0 : 16}px; border-left: 1px solid {indentColor}; padding-left: 12px;">
                {#each entries as [k, v], i}
                    <div style="display: flex; align-items: flex-start; gap: 4px; padding: 1px 0; font-size: 0.82rem; line-height: 1.6;">
                        {#if !isArray}
                            <span style="color: {keyColor}; flex-shrink: 0; font-weight: 500;">"{k}"</span>
                            <span style="color: {bracketColor}; flex-shrink: 0;">:</span>
                        {/if}
                        {#if v !== null && typeof v === 'object'}
                            <JsonTreeViewer value={v} depth={depth + 1} {darkMode} />
                        {:else}
                            {@const fmt = formatScalar(v)}
                            <span style="color: {fmt.color}; word-break: break-all;">{fmt.text}</span>
                        {/if}
                        {#if i < entries.length - 1}
                            <span style="color: {bracketColor}; flex-shrink: 0;">,</span>
                        {/if}
                    </div>
                {/each}
            </div>
            <span style="color: {bracketColor};">{closeBracket}</span>
        {/if}
    </span>
{:else}
    {@const fmt = formatScalar(value)}
    <span style="color: {fmt.color}; font-size: 0.82rem; word-break: break-all;">{fmt.text}</span>
{/if}
