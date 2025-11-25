<script lang="ts">
  import type { ComponentDef } from "./FormTypes";

  let {
    itemDef,
    formData = $bindable()
  } = $props<{
    itemDef: ComponentDef;
    formData: { [key: string]: unknown };
  }>();

  // Ensure current selections is always an array
  const currentSelections = $derived(
    Array.isArray(formData[itemDef.fieldName])
      ? formData[itemDef.fieldName]
      : []
  );

  // Local tooltip state
  let showTooltip = $state(false);
  let tooltipPosition = $state({ x: 0, y: 0 });

  // Toggle a single option
  function toggleOption(option: string) {
    const selections = currentSelections as string[]
    const newSelections = selections.includes(option)
      ? selections.filter(val => val !== option)
      : [...selections, option];
    
    formData[itemDef.fieldName] = newSelections;

    // Auto-clear error if something is selected
    if (newSelections.length > 0) {
    }
  }

  // Validation: at least one must be selected if required
  export function checkValue(): string {
    if (itemDef.required && currentSelections.length === 0) {
      return "Field '" + itemDef.label + "' is required.";
    }
    return "";
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
</script>

<div class="form-group">
  <label for={itemDef.fieldName} class="group-label">
    {itemDef.label}{itemDef.required ? ' *' : ''}

    {#if itemDef.helpText}
      <span
        class="help-icon"
        role="button"
        tabindex="0"
        onmouseenter={handleMouseEnter}
        onmouseleave={handleMouseLeave}
        onmousemove={handleMouseMove}
        aria-label="Show help"
      >
        ?
      </span>
    {/if}
  </label>

  <div class="checkbox-group">
    {#each itemDef.options as option}
      <label class="checkbox-item">
        <input
          type="checkbox"
          checked={currentSelections.includes(option)}
          onchange={() => toggleOption(option)}
        />
        <span class="checkbox-label">{option}</span>
      </label>
    {/each}
  </div>

  {#if itemDef.error}
    <span class="error-message">{itemDef.error}</span>
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
    gap: 8px;
  }

  .group-label {
    font-weight: 600;
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .checkbox-group {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(100px, 1fr));
    gap: 8px 16px;
    padding: 8px 0;
  }

  @media (max-width: 480px) {
    .checkbox-group {
        grid-template-columns: 1fr;
    }
  }

  .checkbox-item {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    user-select: none;
  }

  .checkbox-item input[type="checkbox"] {
    margin: 0;
    cursor: pointer;
  }

  .checkbox-label {
    font-size: 0.95em;
    cursor: pointer;
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
</style>