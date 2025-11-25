<script lang="ts">
	import * as Collapsible from "$lib/components/ui/collapsible/index.js";
	import * as Sidebar from "$lib/components/ui/sidebar/index.js";
	import ChevronRightIcon from "@lucide/svelte/icons/chevron-right";
    import type { Icon } from "@tabler/icons-svelte";
    import type {NavigationMenuItem} from "$lib/types/menu"

    // It assumes nav-main-01 is a two-level menu systems. The first level
    // has title, url, icon. It contains a number of menu items, which
    // are defined in its 'items', which has only two attributes: title
    // and url.
	let {
		items,
        onMenuSelect
	}: {
		items: {
			title: string;
			url: string;
			icon?: any;
			isActive?: boolean;
			items?: {
				title: string;
				url: string;
			}[];
		}[];
        onMenuSelect: (item:NavigationMenuItem) => void;
	} = $props();

	function handleSelect(item: { title: string; url: string; icon?: Icon }) {
		// Call the parent’s callback if provided
        const menu_item : NavigationMenuItem = {
            id: "",
            title: item.title,
            url: item.url,
            icon: item.icon,
            type: "navigation"
        }
        onMenuSelect(menu_item)
	}
</script>

<Sidebar.Group>
	<Sidebar.GroupLabel>Platform</Sidebar.GroupLabel>
	<Sidebar.Menu>
		{#each items as item (item.title)}
			<Collapsible.Root open={item.isActive} class="group/collapsible">
				{#snippet child({ props })}
					<Sidebar.MenuItem {...props}>
						<Collapsible.Trigger>
							{#snippet child({ props })}
								<Sidebar.MenuButton {...props} 
                                    tooltipContent={item.title}
                                    onclick={() => handleSelect(item)}>
									{#if item.icon}
										<item.icon />
									{/if}
									<span>{item.title}</span>
									<ChevronRightIcon
										class="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90"
									/>
								</Sidebar.MenuButton>
							{/snippet}
						</Collapsible.Trigger>
						<Collapsible.Content>
							<Sidebar.MenuSub>
								{#each item.items ?? [] as subItem (subItem.title)}
									<Sidebar.MenuSubItem>
										<Sidebar.MenuSubButton>
											{#snippet child({ props })}
												<a href={subItem.url} {...props}>
													<span>{subItem.title}</span>
												</a>
											{/snippet}
										</Sidebar.MenuSubButton>
									</Sidebar.MenuSubItem>
								{/each}
							</Sidebar.MenuSub>
						</Collapsible.Content>
					</Sidebar.MenuItem>
				{/snippet}
			</Collapsible.Root>
		{/each}
	</Sidebar.Menu>
</Sidebar.Group>
