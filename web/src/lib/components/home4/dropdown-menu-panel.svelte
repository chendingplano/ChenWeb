<script lang="ts">
	let {
		darkMode,
		items,
		activeChildId,
		onSelect,
		onMouseEnter,
		onMouseLeave
	}: {
		darkMode: boolean;
		items: Array<{ id: string; label: string; icon?: any }>;
		activeChildId: string | undefined;
		onSelect: (childId: string, childTitle: string) => void;
		onMouseEnter: () => void;
		onMouseLeave: () => void;
	} = $props();

	// --- Panel geometry / style constants ---
	const DROPDOWN_RADIUS  = '12px';                        // border-radius for panel
	const DROPDOWN_SHADOW  = '0 8px 24px rgba(0,0,0,0.12)'; // panel shadow
	const DROPDOWN_MIN_W   = 200;                           // min-width in px
	const ITEM_PADDING     = '10px 16px';                   // item padding
	const HOVER_TINT_LIGHT = 'rgba(99,102,241,0.08)';       // item hover bg light
	const HOVER_TINT_DARK  = 'rgba(129,140,248,0.10)';      // item hover bg dark

	// --- Colour tokens (all $derived from darkMode) ---
	let cardBg      = $derived(darkMode ? '#1E2535' : '#ffffff');       // card / panel background
	let border      = $derived(darkMode ? '#2D3348' : '#E4E6EB');       // border / divider lines
	let textMain    = $derived(darkMode ? '#E2E8F0' : '#111827');       // primary text
	let textMuted   = $derived(darkMode ? '#64748b' : '#94a3b8');       // muted text
	let accent      = $derived(darkMode ? '#818CF8' : '#6366F1');       // primary accent (indigo)
	let accentTint  = $derived(darkMode ? HOVER_TINT_DARK : HOVER_TINT_LIGHT); // active item bg
	let hoverTint   = $derived(darkMode ? HOVER_TINT_DARK : HOVER_TINT_LIGHT); // hover bg

	// Suppress unused constant warnings
	void DROPDOWN_MIN_W;
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="overflow-hidden"
	style="
		background:{cardBg};
		border:1px solid {border};
		border-radius:{DROPDOWN_RADIUS};
		box-shadow:{DROPDOWN_SHADOW};
		min-width:{DROPDOWN_MIN_W}px;
	"
	onmouseenter={onMouseEnter}
	onmouseleave={onMouseLeave}
	role="menu"
>
	{#each items as item}
		<button
			role="menuitem"
			onclick={() => onSelect(item.id, item.label)}
			class="w-full flex items-center gap-3 text-left transition-colors duration-100 cursor-pointer"
			style="
				padding:{ITEM_PADDING};
				background:{activeChildId === item.id ? accentTint : 'transparent'};
				color:{activeChildId === item.id ? accent : textMain};
			"
			onmouseenter={(e) => {
				if (activeChildId !== item.id) {
					(e.currentTarget as HTMLElement).style.background = hoverTint;
				}
			}}
			onmouseleave={(e) => {
				if (activeChildId !== item.id) {
					(e.currentTarget as HTMLElement).style.background = 'transparent';
				}
			}}
		>
			{#if item.icon}
				<item.icon class="w-4 h-4 flex-shrink-0" style="color:{activeChildId === item.id ? accent : textMuted};" />
			{/if}
			<span class="text-sm">{item.label}</span>
		</button>
	{/each}
</div>
