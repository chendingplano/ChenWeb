<script lang="ts">
    import type { ComponentDef } from "./FormTypes";

    let {
        itemDef,
        formData = $bindable()
    }:
    {
        itemDef: ComponentDef;
        formData: { [key: string]: unknown };
    } = $props();

    // Internal state for Tooltip
    let showTooltip = $state(false);
    let tooltipPosition = $state({ x: 0, y: 0 });

    // Validation (exposed to parent)
    export function checkValue(): string {
        if (itemDef.required && !formData[itemDef.fieldName]) {
            return "Field '" + itemDef.label+ "' is required.";
        }

        return '';
    }

    // Tooltip Handlers
    function handleMouseEnter(e: MouseEvent) {
        showTooltip = true;
        updateTooltipPosition(e);
    }

    function handleMouseMove(e: MouseEvent) {
        updateTooltipPosition(e);
    }

    function handleMouseLeave() {
        showTooltip = false;
    }

    function updateTooltipPosition(e: MouseEvent) {
        tooltipPosition = { x: e.clientX + 10, y: e.clientY + 10 };
    }
</script>

<div class="form-group {itemDef.fullWidth ? 'full-width' : ''}">
    <label for={itemDef.fieldName}>
        {itemDef.label}{itemDef.required ? ' *' : ''}

        {#if itemDef.helpText}
            <span 
                class="help-icon"
                role="button"
                tabindex="0"
                onmouseenter={handleMouseEnter}
                onmouseleave={handleMouseLeave}
                onmousemove={handleMouseMove}
            >
                ?
            </span>
        {/if}
    </label>

    <textarea
        id={itemDef.fieldName}
        bind:value={formData[itemDef.fieldName]}
        placeholder={itemDef.placeholder}
        class:error={!!itemDef.error}
        rows={itemDef.rows || 4}
        onchange={() => {
            if (formData[itemDef.fieldName]) itemDef.error = '';
        }}
    ></textarea>

    {#if itemDef.error}
        <span class="error-message">{itemDef.error}</span>
    {/if}

    {#if itemDef.helpText}
        <div class="help-text">{itemDef.helpText}</div>
    {/if}

    {#if showTooltip && itemDef.helpText}
        <div 
            class="tooltip"
            style="left: {tooltipPosition.x}px; top: {tooltipPosition.y}px;"
        >
            {itemDef.helpText}
        </div>
    {/if}
</div>

<style>
    .form-group {
        display: flex;
        flex-direction: column;
        position: relative;
        margin-bottom: 1rem;
    }

    .form-group textarea {
        padding: 10px;
        border: 1px solid #ddd;
        border-radius: 4px;
        font-size: 0.9em;
        transition: border-color 0.3s;
    }

    .form-group label {
        font-weight: bold;
        margin-bottom: 8px;
        color: #555;
        font-size: 0.9em;
    }

    .help-icon {
        display: inline-block;
        background: #ddd;
        border-radius: 50%;
        width: 20px;
        height: 20px;
        text-align: center;
        font-size: 12px;
        line-height: 20px;
        margin-left: 8px;
        cursor: help;
    }

    textarea.error {
        border: 1px solid red;
    }

    textarea {
        padding: 8px;
        font-size: 1rem;
        resize: vertical; /* allow user to resize vertically */
    }

    .error-message {
        color: red;
        font-size: 0.85rem;
        margin-top: 4px;
    }

    .tooltip {
        position: fixed;
        background: #333;
        color: #fff;
        padding: 5px 10px;
        border-radius: 4px;
        font-size: 12px;
        pointer-events: none;
        z-index: 1000;
    }

    .help-text {
        color: #6c757d;
        font-size: 0.8em;
        margin-top: 5px;
        font-style: italic;
    }

    .full-width {
        grid-column: 1 / -1;
    }

    .form-group textarea:focus {
        outline: none;
        border-color: #007bff;
        box-shadow: 0 0 0 2px rgba(0, 123, 255, 0.25);
    }
</style>
