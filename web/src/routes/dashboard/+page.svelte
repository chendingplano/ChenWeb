<script lang="ts">
	import { onMount } from 'svelte';
	import * as Sidebar from '$lib/components/ui/sidebar/index.js';
	import AppSidebar from '$lib/components/app-sidebar.svelte';
	import SiteHeader from '$lib/components/site-header.svelte';
	import SectionCards from '$lib/components/section-cards.svelte';
	import ChartAreaInteractive from '$lib/components/chart-area-interactive.svelte';
	import DataTable from '$lib/components/data-table.svelte';
	import type { Dashboard01Schema } from '$lib/components/schemas';
	import { query_builder, cond_builder } from '@chendingplano/shared';
	import { loadConfig } from '$lib/config/config-client';

	let data = $state<Dashboard01Schema[]>([]);
	let loading = $state(true);
	let error = $state<string>('');

	onMount(async () => {
		/*
		try {
			console.log('Fetching dashboard data (CWB_DBD_019)...');
			const config = await loadConfig();
			const results = await query_builder
				.select()
				.from(config.app_table_names.table_name_process_status)
				.execute();

			console.log('Dashboard data loaded successfully (CWB_DBD_035)');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Unknown error';
			alert('Error loading dashboard data (CWB_DBD_038): ' + error);
		} finally {
			loading = false;
		}
		*/
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
						<!-- DataTable is not implemented yet (Chen Ding, 2026/01/01)
                        <DataTable {data} />
                        -->
					{/if}
				</div>
			</div>
		</div>
	</Sidebar.Inset>
</Sidebar.Provider>
