<!-- web/src/lib/components/table-with-filters.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { beforeNavigate } from '$app/navigation';
	import type { IProcessTableRow } from "$lib/components/common-types.js";
    import type { Condition, Field, Operator, LogicalOperator} from '$lib/types';
    import QueryEditor from '$lib/components/query-editor.svelte';
	import { createEventDispatcher } from 'svelte';
	import type {
		GridApi,
		GridOptions,
		SelectionChangedEvent
	} from "ag-grid-community";
	import {
		AllCommunityModule,
		ModuleRegistry,
		createGrid,
	} from "ag-grid-community";

	ModuleRegistry.registerModules([AllCommunityModule]);
	const dispatch = createEventDispatcher();

	let gridEl = $state<HTMLDivElement | null>(null);
	let hotInstance = $state<GridApi | null>(null);
	let tableDirty = $state<boolean>(false);
	let { class: className = '' } = $props();
    let filterConditions = $state<Condition[]>([]);
	let filterLogic = $state<LogicalOperator>('AND');
	let selectedRow = $state<IProcessTableRow | null>(null);

	// 🔍 Filter state
	let globalFilter = $state<string>('');
    let _filterTimeout: number | null = null;

	// Fetch data with current filters
	const fetchData = async () => {
		if (!hotInstance) return;

		try {
			const url = `/api/v1/retrieve-process-data`;

            console.log("Fetching data from URL:", url);
			const res = await fetch(url);
			if (!res.ok) throw new Error("Failed to fetch data (MID_002_086)");
			const data: IProcessTableRow[] = await res.json();
			console.log("Set data (MID_002_052)");
			hotInstance.setGridOption("rowData", data);
		} catch (e) {
			if (e instanceof Error) {
				console.log("Error fetching process data (MID_002_094):", e.message);
			} else {
				console.log("Error fetching process data (MID_002_097):", String(e));
			}
		}
	};

	// Grid Options
	const gridOptions: GridOptions<IProcessTableRow> = {
		rowData: [],
		pagination: true,
		defaultColDef: {
			filter: true,
			editable: true,
		},
		columnDefs: [
			{ field: "record_uuid", filter: true },
			{ field: "md5", filter: true },
			{ field: "doc_name", minWidth: 300 },
			{ field: "doc_num" },
			{ field: "file_url", filter: true },
			{ field: "status", filter: true },
			{ field: "data_proc_status", filter: true },
			{ field: "data_proc_at", filter: true },
			{ field: "chunk_proc_status", filter: true },
			{ field: "chunk_proc_at", filter: true },
			{ field: "embedding_proc_status", filter: true },
			{ field: "embedding_proc_at", filter: true },
			{ field: "es_proc_status", filter: true },
			{ field: "es_proc_at", filter: true },
			{ field: "parser_source_tname", filter: true },
			{ field: "parser_content_proc_tname", filter: true },
			{ field: "chunk_tablename", filter: true },
			{ field: "error_msg" },
			{ field: "knowledge_base_id", filter: true },
			{ field: "create_at", filter: true },
			{ field: "proc_status_json"}
		],
		rowSelection: { 
			mode: "singleRow", 
			checkboxes: false,
			enableClickSelection: true,
		},
		onSelectionChanged: (event: SelectionChangedEvent) => {
			console.log(`Row Selection Event`);
			const selectedNodes = event.api.getSelectedNodes();
			const selectedData = event.api.getSelectedRows();
			if (selectedData.length > 0) {
				console.log('First selected row:', selectedData[0]);
				selectedRow = selectedData[0];
				dispatch('rowSelect', selectedData[0]);
			}
		},
	};

	// ✅ Call beforeNavigate at top level — synchronous
    beforeNavigate(({ cancel }) => {
      console.log("Check before leaving");
      if (tableDirty) {
        const leave = confirm('You have unsaved changes. Are you sure you want to leave?');
        if (!leave) {
          cancel();
        } else {
          saveChanges();
          tableDirty = false;
        }
        return "You have unsaved changes";
      }
    });

	onMount(async () => {
		if (gridEl === null) throw Error('***** SHOULD NEVER HAPPEN: gridEl is null');
		hotInstance = createGrid(gridEl, gridOptions);
		await fetchData();
	});

	const saveChanges = async () => {
		console.log("To save the changes");
		tableDirty = false;
	};

	$effect(() => {
  		const params = new URLSearchParams();
  		filterConditions.forEach((cond, i) => {
    		params.append(`filters[${i}].field`, cond.field);
    		params.append(`filters[${i}].op`, cond.operator);
    		params.append(`filters[${i}].value`, cond.value);
    		params.append(`filters[${i}].logic_opr`, cond.logic_opr);
		});
		console.log("Current filter conditions (MID_003_058):", JSON.stringify(filterConditions, null, 2));
		console.log("Constructed query params (MID_003_061):", params.toString());

  		// fetch(`/api/...?${params}`)
	});

	// Fetch using current filters
	const fetchDataWithFilters = async () => {
      if (!hotInstance) return;
      const params = new URLSearchParams();
	  console.log("Applying filter conditions (MID_002_130):", JSON.stringify(filterConditions, null, 2));
	  console.log("Using logical operator (MID_002_131):", filterLogic);
      filterConditions.forEach((cond, i) => {
		params.append(`field_${i}`, cond.field);
		params.append(`op_${i}`, cond.operator);
		params.append(`val_${i}`, cond.value);
		params.append(`logic_opr_${i}`, cond.logic_opr);
      });
      const url = params.toString()
        ? `/api/v1/retrieve-process-data?${params.toString()}`
        : '/api/v1/retrieve-process-data';

	  console.log("Fetching data with filters from URL (MID_002_133):", url);
      const res = await fetch(url);
      const data: IProcessTableRow[] = await res.json();
      hotInstance.setGridOption('rowData', data);
    };

    // Reset and reload all data
    const resetAndFetchAll = async () => {
      filterConditions.length = 0; // clear
      await fetchDataWithFilters(); // fetch unfiltered
    };

</script>

<div class="layout-container">
  <QueryEditor 
  	bind:conditions={filterConditions} 
	bind:logic={filterLogic}
	onquery={fetchDataWithFilters}
    onreset={resetAndFetchAll}/>
  <div bind:this={gridEl} class={`ag-theme-quartz ${className}`}></div>
</div>

<style>
  .layout-container {
    display: flex;
    flex-direction: column;
    gap: 15px;
	height: 100%;
  }

  /* Ensure grid fills available space */
  div.ag-theme-quartz {
  	flex: 1;
    min-height: 400px; /* fallback if flex doesn't work */
    width: 100%;
  }
</style>