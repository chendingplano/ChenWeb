<script lang="ts">
	import { onMount } from 'svelte';
	import 'handsontable/styles/handsontable.min.css';
	import 'handsontable/styles/ht-theme-main.min.css';
	import Handsontable from 'handsontable';

	let gridEl = $state<HTMLDivElement | null>(null);
	// eslint-disable-next-line @typescript-eslint/no-unused-vars
	let hotInstance = $state<Handsontable | null>(null);

	onMount(() => {
		if (gridEl === null) throw new Error('grid element is null when it should not be');
		hotInstance = new Handsontable(gridEl, {
			// theme name with obligatory ht-theme-* prefix
			themeName: 'ht-theme-main-dark-auto',
			// other options
			data: [
				['', 'Tesla', 'Volvo', 'Toyota', 'Ford', 'BYD'],
				['2019', 10, 11, 12, 13, 14],
				['2020', 20, 11, 14, 13, 15],
				['2021', 30, 15, 12, 13, 16]
			],
			rowHeaders: true,
			colHeaders: true,
			height: 'auto',
			autoWrapRow: true,
			autoWrapCol: true,
			licenseKey: 'non-commercial-and-evaluation' // for non-commercial use only
		});
	});
</script>

<div bind:this={gridEl}></div>
