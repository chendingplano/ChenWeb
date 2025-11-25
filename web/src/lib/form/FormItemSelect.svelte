<script lang="ts">
    import type {ComponentDef} from "./FormTypes"

    let { itemDef, formData = $bindable() } = $props<{
        itemDef: ComponentDef;
        formData: {[key: string]: unknown}
    }>();

    // Internal state for Tooltip (Abstraction: Parent doesn't need to know about this)
    let showTooltip = $state(false);
    let tooltipPosition = $state({ x: 0, y: 0 });

    // 1. Direct Binding: We bind directly to itemDef.value.
    // Because itemDef is an object reference passed from the parent,
    // updating properties on it updates the parent's state automatically.

    // 2. Validation Function (Exposed to Parent)
    export function checkValue(): string {
        if (itemDef.required && !itemDef.value) {
            return "Field '" + itemDef.label + "' is required.";
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
        // simple offset so it doesn't block the cursor
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

    <select 
        id={itemDef.id}
        bind:value={formData[itemDef.fieldName]}
        class:error={!!itemDef.error}
        onchange={() => {
            // Auto-clear error when user makes a selection
            if(itemDef.value) itemDef.error = ''; 
        }}
    >
        <option value="">Select an option</option>
        {#each itemDef.options as option}
            <option value={option}>{option}</option>
        {/each}
    </select>

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
    }

    .form-group select {
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

    select.error {
        border: 1px solid red;
    }

    .help-text {
        color: #6c757d;
        font-size: 0.8em;
        margin-top: 5px;
        font-style: italic;
    }

    .error-message {
        color: red;
        font-size: 0.85rem;
        margin-top: 4px;
    }

    .tooltip {
        position: fixed; /* Fixed ensures x/y works relative to viewport */
        background: #333;
        color: #fff;
        padding: 5px 10px;
        border-radius: 4px;
        font-size: 12px;
        pointer-events: none;
        z-index: 1000;
    }

    .form-group select:focus {
        outline: none;
        border-color: #007bff;
        box-shadow: 0 0 0 2px rgba(0, 123, 255, 0.25);
    }
</style>