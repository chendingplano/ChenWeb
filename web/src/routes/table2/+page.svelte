<script lang="ts">
	import { onMount } from 'svelte';
	import { beforeNavigate } from '$app/navigation';
	import type {
		GridApi,
		GridOptions,
	} from "ag-grid-community"
	import {
		AllCommunityModule,
		ModuleRegistry,
		createGrid,
	} from "ag-grid-community"

	ModuleRegistry.registerModules([AllCommunityModule]);

	let gridEl = $state<HTMLDivElement | null>(null);
	let hotInstance = $state<GridApi | null>(null);
	let tableDirty = $state<boolean>(false)

	// Row Data Interface
	interface IRow {
  		mission: string;
  		company: string;
  		location: string;
  		date: string;
  		time: string;
  		rocket: string;
  		price: number;
  		successful: boolean;
	}

	// Grid Options: Contains all of the grid configurations
	const gridOptions: GridOptions<IRow> = {
  		// Data to be displayed
  		rowData: [],
		pagination: true,
		defaultColDef: {
			filter: true
		},
  		// Columns to be displayed (Should match rowData properties)
  		columnDefs: [
    		{ field: "mission", filter: true },
    		{ field: "company", 
			},
    		{ field: "location", minWidth: 300},
    		{ field: "date" },
    		{ 
				field: "price",
				editable: true,
				singleClickEdit: false,
				valueFormatter: (params) => { return '£' + params.value.toLocaleString(); },
				// Keep track of edits using 'tableDirty' variable
    			onCellValueChanged: (event) => {
        			console.log(`New Cell Value: ${event.newValue}, Old value: ${event.oldValue}`)
					if (event.newValue != event.oldValue) {
						tableDirty = true
						console.log('Value changed');
					}
    			},
			},
    		{ field: "successful" },
    		{ field: "rocket" },
  		],
		rowSelection: { mode: "multiRow", headerCheckbox: false },
		onSelectionChanged: (event: SelectionChangedEvent) => {
    		console.log(`Row Selection Event`);

			// Get all selected row nodes
  			const selectedNodes = event.api.getSelectedNodes();
  
  			// Get all selected row data
  			const selectedData = event.api.getSelectedRows();
  
  			console.log('Selected nodes:', selectedNodes);
  			console.log('Selected data:', selectedData);
  
  			// If you want just the first selected row:
  			if (selectedData.length > 0) {
    			console.log('First selected row:', selectedData[0]);
  			}
  		},
	};

	onMount(() => {
		if (gridEl === null) throw Error('***** SHOULD NEVER HAPPEN: gridEl is null')

		hotInstance = createGrid(gridEl, gridOptions);	
		fetch("https://www.ag-grid.com/example-assets/space-mission-data.json")
  			.then((response) => response.json())
  			.then((data: any) => {
				if (hotInstance === null) throw Error("***** SHOULD NEVER HAPPEN");
				hotInstance.setGridOption("rowData", data);
			});

		beforeNavigate(({ cancel, navigation }) => {
			console.log("Check before leaving");
      		if (tableDirty) {
        		// const leave = confirm('You have unsaved changes. Are you sure you want to leave?');
        		if (!leave) {
          			cancel();
        		} else {
          			// User chose to leave, clear the dirty state
					saveChanges()
          			tableDirty = false;
        		}
				return "You have unsaved changes";
      		}
    	});
	});

	// Function to save changes and clear dirty state
  	const saveChanges = async () => {
    	// ... your save logic
		console.log("To save the changes");
    	tableDirty = false;
  	};
</script>

<div bind:this={gridEl} style="width: 1200px; height: 500px"></div>

