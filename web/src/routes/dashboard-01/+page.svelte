<script lang="ts">
    import { onMount } from "svelte";
    import * as Sidebar from "$lib/components/ui/sidebar/index.js";
    import AppSidebar from "$lib/components/app-sidebar.svelte";
    import SiteHeader from "$lib/components/site-header.svelte";
    import SectionCards from "$lib/components/section-cards.svelte";
    import ChartAreaInteractive from "$lib/components/chart-area-interactive.svelte";
    import DataTable from "$lib/components/data-table.svelte";
    import type { Dashboard01Schema } from "$lib/components/schemas";

    let data = $state<Dashboard01Schema[]>([]);
    let loading = $state(true);
    let error = $state<string>("");

    onMount(async () => {
        try {
            const res = await fetch("/api/v1/dashboard-01-data");
            if (!res.ok) throw new Error("Failed to fetch data");
            console.log("Fetch data success (MID_001_019)");
            data = await res.json();
        } catch (e) {
            error = e.message;
        } finally {
            loading = false;
        }
    });
</script>

<Sidebar.Provider
    style="--sidebar-width: calc(var(--spacing) * 72); --header-height: calc(var(--spacing) * 12);"
>
    <AppSidebar variant="inset" />
    <Sidebar.Inset>
        <SiteHeader />
        <div class="flex flex-1 flex-col">
            <div class="@container/main flex flex-1 flex-col gap-2">
                <div class="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
                    <SectionCards />
                    <div class="px-4 lg:px-6">
                        <ChartAreaInteractive />
                    </div>
                    {#if loading}
                        <p>Loading data...</p>
                    {:else if error}
                        <p style="color:red">{error}</p>
                    {:else}
                        <DataTable {data} />
                    {/if}
                </div>
            </div>
        </div>
    </Sidebar.Inset>
</Sidebar.Provider>
