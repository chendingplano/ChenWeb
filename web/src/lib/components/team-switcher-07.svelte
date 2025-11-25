<script lang="ts">
	import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
	import * as Sidebar from "$lib/components/ui/sidebar/index.js";
	import { useSidebar } from "$lib/components/ui/sidebar/index.js";
	import ChevronsUpDownIcon from "@lucide/svelte/icons/chevrons-up-down";
    import type { Icon } from "@tabler/icons-svelte";
    import type { TeamMenuItem} from "$lib/types/menu";
	import PlusIcon from "@lucide/svelte/icons/plus";

	// This should be `Component` after @lucide/svelte updates types
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let { teams, onMenuSelect }: 
        { 
            teams: { name: string; url:string, logo: any; plan: string }[];
            onMenuSelect:(team:TeamMenuItem) => void;
        } = $props();

	const sidebar = useSidebar();

	let activeTeam = $state(teams[0]);
    function handleSelect(team:{name: string, url:string, logo:Icon; plan: string}) {
        const menu_item : TeamMenuItem = {
			id:			"team_" + team.name,
			type:		"team",
            title: 		team.name,
            url: 		team.url,
            icon: 		team.logo,
			plan:		team.plan
        }
        onMenuSelect(menu_item)
    }

	function handleDropdownMenuSelect(team: { name: string; url:string, logo: any; plan: string }) {
		// Call the parent’s callback if provided
        const menu_item : TeamMenuItem = {
            id:         `team_${team.name}`,
            title:      team.name,
            type:       "team",
			plan:		team.plan,
			url:		team.url,
        };
		onMenuSelect(menu_item);
	}
</script>

<Sidebar.Menu>
	<Sidebar.MenuItem>
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<Sidebar.MenuButton
						{...props}
						size="lg"
						class="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
						onclick={()=>handleSelect(activeTeam)}
					>
						<div
							class="bg-sidebar-primary text-sidebar-primary-foreground flex aspect-square size-8 items-center justify-center rounded-lg"
						>
							<activeTeam.logo class="size-4" />
						</div>
						<div class="grid flex-1 text-left text-sm leading-tight">
							<span class="truncate font-medium">
								{activeTeam.name}
							</span>
							<span class="truncate text-xs">{activeTeam.plan}</span>
						</div>
						<ChevronsUpDownIcon class="ml-auto" />
					</Sidebar.MenuButton>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content
				class="w-(--bits-dropdown-menu-anchor-width) min-w-56 rounded-lg"
				align="start"
				side={sidebar.isMobile ? "bottom" : "right"}
				sideOffset={4}
			>
				<DropdownMenu.Label class="text-muted-foreground text-xs">Teams</DropdownMenu.Label>
				{#each teams as team, index (team.name)}
					<DropdownMenu.Item onclick={() => handleDropdownMenuSelect(team)} class="gap-2 p-2">
						<div class="flex size-6 items-center justify-center rounded-md border">
							<team.logo class="size-3.5 shrink-0" />
						</div>
						{team.name}
						<DropdownMenu.Shortcut>⌘{index + 1}</DropdownMenu.Shortcut>
					</DropdownMenu.Item>
				{/each}
				<DropdownMenu.Separator />
				<DropdownMenu.Item 
					onclick={() => handleDropdownMenuSelect({name:"add team", url:"", logo:"", plan:""})} 
					class="gap-2 p-2">
					<div
						class="flex size-6 items-center justify-center rounded-md border bg-transparent"
					>
						<PlusIcon class="size-4" />
					</div>
					<div class="text-muted-foreground font-medium">Add team</div>
				</DropdownMenu.Item>
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	</Sidebar.MenuItem>
</Sidebar.Menu>
