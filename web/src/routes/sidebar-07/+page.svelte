<script lang="ts">
	import AppSidebar from "$lib/components/app-sidebar-07.svelte";
	import * as Breadcrumb from "$lib/components/ui/breadcrumb/index.js";
	import { Separator } from "$lib/components/ui/separator/index.js";
	import * as Sidebar from "$lib/components/ui/sidebar/index.js";
    import { goto } from '$app/navigation';
	import type {Sidebar07MenuItem, NullMenuItem} from "$lib/types/menu"

	const null_menu_item : NullMenuItem = {
    	id:     'null',
    	title:  'null',
    	type:   'null'
    }

  	let { children } = $props();
  	let activeView = $state<Sidebar07MenuItem>(null_menu_item)
  	// let recordSelected = $state<IProcessTableRow | null>(null); 

    async function handleLogout() {
    try {
      const res = await fetch('/auth/logout', {
        method: 'DELETE',
        credentials: 'same-origin' // Important: sends cookies
      });

      if (res.ok) {
        console.log('Logged out successfully');
        // Redirect to login page
        goto('/login');
      } else {
        console.error('Logout failed');
        // Optionally show error
      }
    } catch (err) {
      console.error('Network error during logout:', err);
      // Still redirect to login for safety
      goto('/login');
    }
    }

    function handleMenuSelect(item: Sidebar07MenuItem) {
        console.log("Menu selected (CWB_SB7_042):", item.title);
        activeView = item;
        if (activeView.title == "user_Log out") {
          console.log("Handle Logout (CWB_SB1_066):", activeView);
          handleLogout()
        }
    }
</script>

<Sidebar.Provider>
	<AppSidebar onMenuSelect={handleMenuSelect}/>
	<Sidebar.Inset>
		<header
			class="group-has-data-[collapsible=icon]/sidebar-wrapper:h-12 flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear"
		>
			<div class="flex items-center gap-2 px-4">
				<Sidebar.Trigger class="-ml-1" />
				<Separator orientation="vertical" class="mr-2 data-[orientation=vertical]:h-4" />
				<Breadcrumb.Root>
					<Breadcrumb.List>
						<Breadcrumb.Item class="hidden md:block">
							<Breadcrumb.Link href="##">Building Your Application AAA</Breadcrumb.Link>
						</Breadcrumb.Item>
						<Breadcrumb.Separator class="hidden md:block" />
						<Breadcrumb.Item>
							<Breadcrumb.Page>Data Fetching</Breadcrumb.Page>
						</Breadcrumb.Item>
					</Breadcrumb.List>
				</Breadcrumb.Root>
			</div>
		</header>
		<div class="flex flex-1 flex-col gap-4 p-4 pt-0">
			<div class="grid auto-rows-min gap-4 md:grid-cols-3">
				<div class="bg-muted/50 aspect-video rounded-xl">Round box 1</div>
				<div class="bg-muted/50 aspect-video rounded-xl">Round box 2</div>
				<div class="bg-muted/50 aspect-video rounded-xl">Round box 3</div>
			</div>
			<div class="bg-muted/50 min-h-[100vh] flex-1 rounded-xl md:min-h-min">
				{#if activeView.title === 'Documents'}
            	<div class="pt-5 px-4 w-full h-[900px] overflow-hidden">
                	<!--<TableWithFilters 
                  class="h-full w-full"
                  on:rowSelect={(e) => handleRowSelect(e.detail)}/>
				-->
				  Menu 'Documents' not implemented yet!
            	</div>
        		{:else if activeView.title === 'History'}
            		<div class="pt-5 px-4 h-[800px] overflow-hidden">
						History not implemented yet!
            		</div>
        		{:else}
            		<div class="flex h-full items-center justify-center">
                		<span class="font-semibold text-muted-foreground">'{activeView.title}' menu not implemented yet!</span>
            		</div>
        		{/if}
			</div>
		</div>
	</Sidebar.Inset>
</Sidebar.Provider>
