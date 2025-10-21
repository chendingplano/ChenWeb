<script lang="ts">
	import {onMount} from 'svelte';
	import type {
  		ColDef,
  		DefaultMenuItem,
  		GetMainMenuItemsParams,
  		GridApi,
  		GridOptions,
  		MenuItemDef,
	} from "ag-grid-community";

	import {
  		ClientSideRowModelModule,
  		ModuleRegistry,
  		ValidationModule,
  		createGrid,
	} from "ag-grid-community";

	ModuleRegistry.registerModules([
  		ClientSideRowModelModule,
  		...(process.env.NODE_ENV !== "production" ? [ValidationModule] : []),
		]);

	interface IOlympicData {
    	athlete: string,
    	age: number,
    	country: string,
    	year: number,
    	date: string,
    	sport: string,
    	gold: number,
    	silver: number,
    	bronze: number,
    	total: number
	}

	const columnDefs: ColDef[] = [
  		{ field: "athlete", minWidth: 200 },
  		{
    		field: "age",
    		mainMenuItems: (params: GetMainMenuItemsParams) => {
      			const athleteMenuItems: (MenuItemDef | DefaultMenuItem)[] =
        		params.defaultItems.slice(0);
      			athleteMenuItems.push({
        			name: "A Custom Item",
        			action: () => {
          			console.log("A Custom Item selected");
        			},
      			});

      			athleteMenuItems.push({
        			name: "Another Custom Item",
        			action: () => {
          			console.log("Another Custom Item selected");
        			},
      			});

      			athleteMenuItems.push({
        			name: "Custom Sub Menu",
        			subMenu: [
          				{
            				name: "Black",
            				action: () => {
              					console.log("Black was pressed");
            				},
          				},
          				{
            				name: "White",
            				action: () => {
              					console.log("White was pressed");
            				},
          				},
          				{
            				name: "Grey",
            				action: () => {
              					console.log("Grey was pressed");
            				},
          				},
        			],
      			});
      			return athleteMenuItems;
    		},
  		},
  		{
    		field: "country",
    		minWidth: 200,
    		mainMenuItems: [
      			{
        			// our own item with an icon
        			name: "A Custom Item",
        			action: () => {
          				console.log("A Custom Item selected");
        			},
        			icon: '<img src="https://www.ag-grid.com/example-assets/lab.png" style="width: 14px;" />',
      			},
      			{
        			// our own icon with a check box
        			name: "Another Custom Item",
        			action: () => {
          				console.log("Another Custom Item selected");
        			},
        			checked: true,
      			},
      			"resetColumns", // a built in item
    		],
  		},
  		{
    		field: "year",
    		mainMenuItems: (params: GetMainMenuItemsParams) => {
      			const menuItems: (MenuItemDef | DefaultMenuItem)[] = [];
      				const itemsToExclude = ["separator", "pinSubMenu", "valueAggSubMenu"];
      					params.defaultItems.forEach((item) => {
        					if (itemsToExclude.indexOf(item) < 0) {
          						menuItems.push(item);
        					}
      					});
      			return menuItems;
    		},
  		},
  		{ field: "sport", minWidth: 200 },
  		{ field: "gold" },
  		{ field: "silver" },
  		{ field: "bronze" },
  		{ field: "total" },
	];

	let gridEl = $state<HTMLDivElement | null>(null);
	let hotInstance = $state<GridApi | null>(null);

	const gridOptions: GridOptions<IOlympicData> = {
  		columnDefs: columnDefs,
  		defaultColDef: {
    		flex: 1,
    		minWidth: 100,
  		},
	};

	onMount(() => {
		if (gridEl === null) throw Error('***** SHOULD NEVER HAPPEN');

		hotInstance = createGrid(gridEl, gridOptions);

		fetch("https://www.ag-grid.com/example-assets/olympic-winners.json")
  			.then((response) => response.json())
  			.then((data: IOlympicData[]) => hotInstance!.setGridOption("rowData", data));
	});
</script>

<div bind:this={gridEl} style="height:1000px"></div>

