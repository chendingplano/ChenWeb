<!--
  CWB_SB1 Sidebar with Resizable Panes and Dynamic Content
  This Svelte component implements a sidebar layout with resizable panes.
  It dynamically loads different views based on menu selection and handles user logout.

  The sidebar implements a menu with various items. When a menu item is selected,
  the function handleMenuSelect is called back with the selected item's details.
  The main content area updates to show different components based on the selected menu item.
  The right pane displays details related to the selected item in the main content area.

  Note that the menu selection is not handled by the menu component itself, but rather
  by this parent component, which updates the activeView state accordingly. The reason
  is that the menu component is designed to be reusable and does not maintain its own state.
  When a menu item is selected, it emits an event that this parent component listens to,
  allowing it to control what content to display.

  In this implementation, a menu component should emit a 'select' event with the selected
  item's details. The handleMenuSelect function updates the activeView state based on
  the selected item's title. If the "Log Out" item is selected, it triggers the logout process.
  Note that 'handleMenuSelect' is passed as a prop to the AppSidebar component.
-->
<script lang="ts">
  import * as Sidebar from "$lib/components/ui/sidebar/index.js";
  import AppSidebar from "$lib/components/app-sidebar-01.svelte";
  import * as Resizable from "$lib/components/ui/resizable/index.js";
  import MenuContent from "$lib/components/MenuContent.svelte";
  import TableWithFilters from "$lib/components/table-with-filters.svelte";
  import Table03 from "$lib/components/table-03.svelte";
  import LoginPage from "$lib/components/login-01.svelte";
  import ProcessDetails from "$lib/components/process-details.svelte";
  import Dashboard01 from "$lib/components/dashboard01.svelte";
  import type { IProcessTableRow } from "$lib/components/common-types.js";
  import type { Icon } from "@tabler/icons-svelte";
  import type {Sidebar01MenuItem, NullMenuItem} from "$lib/types/menu"
  import { goto } from '$app/navigation';
	import TeamSwitcher from "$lib/components/team-switcher.svelte";
 
  const null_menu_item : NullMenuItem = {
    id:     'null',
    title:  'null',
    type:   'null'
  }

  let { children } = $props();
  let activeView = $state<Sidebar01MenuItem>(null_menu_item)
  let recordSelected = $state<IProcessTableRow | null>(null); 

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

  function handleMenuSelect(item: Sidebar01MenuItem) {
    console.log("Menu selected (CWB_SB1_014):", item.title);
    activeView = item;
    if (activeView.title == "Log Out") {
      console.log("Handle Logout (CWB_SB1_066):", activeView);
      handleLogout()
    }
  }

 function handleRowSelect(record: IProcessTableRow) {
    recordSelected = record; // ✅ Update selected record
    console.log("After assignment (CWB_SB1_025):", $state.snapshot(recordSelected));
 }

</script>
 
<Sidebar.Provider>
 <AppSidebar onMenuSelect={handleMenuSelect} />
 <main class="flex flex-1 overflow-hidden">
  <Sidebar.Trigger />
    <Resizable.PaneGroup
      direction="horizontal"
      class="w-full h-full"
    >
      <Resizable.Pane defaultSize={75}>
        {@render children?.()}
        <div class="flex h-full items-center justify-center p-6">
        {#if activeView.title === 'Documents'}
            <div class="pt-5 px-4 w-full h-[900px] overflow-hidden">
                <TableWithFilters 
                  class="h-full w-full"
                  on:rowSelect={(e) => handleRowSelect(e.detail)}/>
            </div>
        {:else if activeView.title === 'table-03'}
            <div class="pt-5 px-4 h-[800px] overflow-hidden">
                <Table03 class="h-full w-full"/>
            </div>
        {:else if activeView.title === 'Login'}
            <div class="pt-5 px-4 w-[200px] h-[500px] items-center justify-center">
                <LoginPage />
            </div>
        {:else if activeView.title === 'Dashboard' || !activeView}
            <div class="pt-5 px-4 w-full h-[900px] overflow-auto">
                <Dashboard01/>
            </div>
        {:else}
            <div class="flex h-full items-center justify-center">
                <span class="font-semibold text-muted-foreground">'{activeView.title}' menu not implemented yet!</span>
            </div>
        {/if}
        </div>
      </Resizable.Pane>  

      <Resizable.Handle withHandle />

      <Resizable.Pane 
        defaultSize={25}
        minSize={15}
        class="overflow-auto bg-muted">
        <div class="p-4 border-b h-full">
          {#if activeView.title === 'Documents' && recordSelected}
            <div class="pt-5 px-4 w-full h-[900px] overflow-auto">
              <!-- Pass selected record to ProcessDetails -->
              <ProcessDetails {recordSelected} />
            </div>
          {:else if activeView.title === 'Documents'}
            <p class="text-muted-foreground">Select a document to view details</p>
          {:else if activeView}
            <h2 class="text-lg font-semibold">Details</h2>
            <p class="text-sm text-muted-foreground mt-2">No details available for this view.</p>
          {:else}
            <h2 class="text-lg font-semibold">Details</h2>
          {/if}
        </div>
      </Resizable.Pane>
    </Resizable.PaneGroup>
 </main>
</Sidebar.Provider>