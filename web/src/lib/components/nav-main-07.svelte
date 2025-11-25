<script lang="ts">
	import * as Collapsible from "$lib/components/ui/collapsible/index.js";
	import * as Sidebar from "$lib/components/ui/sidebar/index.js";
	import ChevronRightIcon from "@lucide/svelte/icons/chevron-right";
	import type {Sidebar07MenuItem, NavigationMenuItem} from "$lib/types/menu"

	type itemDef = {
		title: string;
		url: string;
		icon?: any;
		isActive?: boolean;
		items?: {
			title: string;
			url: string;
		}[];
	}

	let {
		items,
		onMenuSelect,
	}: {
		items: itemDef[];
		onMenuSelect: (menu_item: Sidebar07MenuItem) => void;
	} = $props();

    function handleSelect(item: itemDef, sub_menu:{title: string, url: string}) {
        const menu_item : NavigationMenuItem = {
			id:			item.title + "_" + sub_menu.title,
			type:		"navigation",
            title: 		item.title + "_" + sub_menu.title,
            url: 		sub_menu.url,
            icon: 		item.icon,
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
								<Sidebar.MenuButton {...props} tooltipContent={item.title}>
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
										<Sidebar.MenuSubButton onclick={() => handleSelect(item, subItem)}>
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
