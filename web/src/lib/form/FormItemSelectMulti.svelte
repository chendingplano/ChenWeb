<script lang="ts">
  import type { ComponentDef } from "./FormTypes";

  let {
    itemDef,
    formData = $bindable()
  } = $props<{
    itemDef: ComponentDef;
    formData: { [key: string]: unknown };
  }>();

  // Ensure formData[fieldName] is always an array
  const selectedValues = $derived(
    Array.isArray(formData[itemDef.fieldName])
      ? formData[itemDef.fieldName]
      : []
  );

  // Internal tooltip state
  let showTooltip = $state(false);
  let tooltipPosition = $state({ x: 0, y: 0 });

  // Validation: at least one option must be selected if required
  export function checkValue(): boolean {
    itemDef.error = '';

    if (itemDef.required && selectedValues.length === 0) {
      itemDef.error = 'This field is required';
      return false;
    }

    return true;
  }

  // Auto-clear error when user selects something
  function handleChange() {
    if (selectedValues.length > 0) {
      itemDef.error = '';
    }
  }

  // Tooltip handlers
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

  // Update formData directly when selection changes
  // Svelte's bind:value with `multiple` expects an array
  function handleSelectChange(e: Event) {
    e.preventDefault()
    const select = e.target as HTMLSelectElement;
    const values = Array.from(select.selectedOptions).map(opt => opt.value);
    formData[itemDef.fieldName] = values;
    handleChange();
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
    id={itemDef.fieldName}
    multiple
    onchange={handleSelectChange}
    class:error={!!itemDef.error}
  >
    <!-- Optional: show placeholder if nothing selected -->
    {#if selectedValues.length === 0}
      <option value="" disabled selected>
        {itemDef.placeholder || "Select one or more options"}
      </option>
    {/if}

    {#each itemDef.options as option}
      <option
        value={option}
        selected={selectedValues.includes(option)}
      >
        {option}
      </option>
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
        min-height: 40px;
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
        margin-left: auto;
        cursor: help;
        justify-content: end;
    }

    .help-text {
        color: #6c757d;
        font-size: 0.8em;
        margin-top: 5px;
        font-style: italic;
    }

    select.error {
        border: 1px solid red;
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

    .form-group select:focus {
        outline: none;
        border-color: #007bff;
        box-shadow: 0 0 0 2px rgba(0, 123, 255, 0.25);
    }
</style>