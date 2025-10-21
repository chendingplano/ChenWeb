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
  import { goto } from '$app/navigation';
 
  let { children } = $props();
  let activeView = $state<string | null>(null)
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

  function handleMenuSelect(value: CustomEvent<string>) {
    console.log("Menu selected (MID_003_014):", value.detail);
    activeView = value.detail.title;
    console.log("Menu selected (MID_003_015):", activeView);
    if (activeView == "Log Out") {
      handleLogout()
    }
  }

 function handleRowSelect(record: IProcessTableRow) {
    recordSelected = record; // ✅ Update selected record
    console.log("After assignment (MID_003_025):", $state.snapshot(recordSelected));
 }

</script>
 
<Sidebar.Provider>
 <AppSidebar on:select={handleMenuSelect} />
 <main class="flex flex-1 overflow-hidden">
  <Sidebar.Trigger />
    <Resizable.PaneGroup
      direction="horizontal"
      class="w-full h-full"
    >
      <Resizable.Pane defaultSize={75}>
        {@render children?.()}
        <div class="flex h-full items-center justify-center p-6">
        {#if activeView === 'Documents'}
            <div class="pt-5 px-4 w-full h-[900px] overflow-hidden">
                <TableWithFilters 
                  class="h-full w-full"
                  on:rowSelect={(e) => handleRowSelect(e.detail)}/>
            </div>
        {:else if activeView === 'table-03'}
            <div class="pt-5 px-4 h-[800px] overflow-hidden">
                <Table03 class="h-full w-full"/>
            </div>
        {:else if activeView === 'Login'}
            <div class="pt-5 px-4 w-[200px] h-[500px] items-center justify-center">
                <LoginPage />
            </div>
        {:else if activeView === 'Dashboard' || !activeView}
            <div class="pt-5 px-4 w-full h-[900px] overflow-auto">
                <Dashboard01/>
            </div>
        {:else}
            <div class="flex h-full items-center justify-center">
                <span class="font-semibold text-muted-foreground">{activeView ? `'${activeView}' menu` : 'This menu'} not implemented yet!</span>
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
          {#if activeView === 'Documents' && recordSelected}
            <div class="pt-5 px-4 w-full h-[900px] overflow-auto">
              <!-- Pass selected record to ProcessDetails -->
              <ProcessDetails {recordSelected} />
            </div>
          {:else if activeView === 'Documents'}
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