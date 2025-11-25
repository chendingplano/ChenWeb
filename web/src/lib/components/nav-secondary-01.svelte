<script lang="ts">
	import * as Sidebar from "$lib/components/ui/sidebar/index.js";
	import type { WithoutChildren } from "$lib/utils.js";
	import type { ComponentProps } from "svelte";
	import type { Icon } from "@tabler/icons-svelte";
    import type {Sidebar01MenuItem,
                 NavigationMenuItem
    } from "$lib/types/menu"
	import NavDocuments from "./nav-documents.svelte";

	let {items, onMenuSelect, ...restProps }: { 
		items: { title: string; url: string; icon: Icon }[],
		onMenuSelect: (item: NavigationMenuItem) => void
	} & WithoutChildren<
		ComponentProps<typeof Sidebar.Group>
	> = $props();

	// ✅ Function to handle clicks
	function handleSelect(item: { title: string; url: string; icon?: Icon }) {
		// Call the parent’s callback if provided
        const menu_item : NavigationMenuItem = {
            id: "secondary_" + item.title,
            type: "navigation",
            title: item.title,
            url: item.url,
            icon: item.icon
        }
		onMenuSelect(menu_item);
	}
</script>

<Sidebar.Group {...restProps}>
	<Sidebar.GroupContent>
		<Sidebar.Menu>
			{#each items as item (item.title)}
				<Sidebar.MenuItem>
					<Sidebar.MenuButton
						tooltipContent={item.title}
						onclick={() => handleSelect(item)}
					>
						{#snippet child({ props })}
							<a href={item.url} {...props}>
								<item.icon />
								<span>{item.title}</span>
							</a>
						{/snippet}
					</Sidebar.MenuButton>
				</Sidebar.MenuItem>
			{/each}
		</Sidebar.Menu>
	</Sidebar.GroupContent>
</Sidebar.Group>
