<!-- src/lib/components/query-editor.svelte -->
<script lang="ts">
	import type { Field, Operator, LogicalOperator } from '$types/types';

	// ✅ Define the fields array
	const fields: Field[] = [
		'record_uuid',
		'doc_name',
		'doc_num',
		'md5',
		'file_url',
		'status',
		'data_proc_status'
	];

	const operators: Operator[] = ['=', '<>', '>', '>=', '<', '<=', 'contain', 'prefix'];

	// Define events the parent can listen to
	let {
		conditions = $bindable([]),
		onquery = () => {},
		onreset = () => {},
		logic = $bindable('AND' as LogicalOperator)
	} = $props();

	// Reactive state
	let currentField = $state<Field>('doc_name');
	let currentOperator = $state<Operator>('=');
	let currentValue = $state<string>('');
	let logicalOperator = $state<LogicalOperator>('AND'); // 👈 NEW: global logic

	// Add a new condition (to the list, not auto-query)
	function addCondition() {
		if (currentValue.trim() === '') return; // skip empty
		conditions.push({
			field: currentField,
			operator: currentOperator,
			value: currentValue.trim(),
			logic_opr: logic
		});
		console.log('Added condition:', $state.snapshot(conditions[conditions.length - 1]));
		// Reset input for next entry
		currentValue = '';
	}

	// Optional: remove a condition
	function removeCondition(index: number) {
		conditions.splice(index, 1);
	}

	// Emit query event (parent handles actual fetch)
	function handleQuery() {
		// Parent should listen via `on:query`
		console.log('Query conditions:', $state.snapshot(conditions));
		console.log('Logical operator:', logic);
		onquery();
	}

	// Clear all conditions
	function handleReset() {
		conditions.length = 0;
		logic = 'AND'; // reset to default
		onreset();
	}
</script>

<div class="query-editor space-y-4">
	<!-- Logical operator selector -->
	{#if conditions.length > 0}
		<div class="flex items-center gap-2">
			<span class="text-sm font-medium text-gray-700">Combine filters with:</span>
			<div class="flex gap-2">
				<label class="inline-flex items-center">
					<input
						type="radio"
						name="logic"
						value="AND"
						bind:group={logic}
						onchange={() => (logic = 'AND')}
						class="text-blue-600"
					/>
					<span class="ml-1 text-sm text-gray-700">AND</span>
				</label>
				<label class="inline-flex items-center">
					<input
						type="radio"
						name="logic"
						value="OR"
						bind:group={logic}
						onchange={() => (logic = 'OR')}
						class="text-blue-600"
					/>
					<span class="ml-1 text-sm text-gray-700">OR</span>
				</label>
			</div>
		</div>
	{/if}

	<!-- Input row -->
	<div class="flex flex-wrap items-end gap-3">
		<div>
			<label for="field-select" class="mb-1 block text-sm font-medium text-gray-700">Field</label>
			<select
				id="field-select"
				class="rounded-md border border-gray-300 px-3 py-2 shadow-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
				bind:value={currentField}
			>
				{#each fields as field}
					<option value={field}>{field}</option>
				{/each}
			</select>
		</div>

		<div>
			<label for="operator-select" class="mb-1 block text-sm font-medium text-gray-700"
				>Operator</label
			>
			<select
				id="operator-select"
				class="rounded-md border border-gray-300 px-3 py-2 shadow-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
				bind:value={currentOperator}
			>
				{#each operators as op}
					<option value={op}>{op}</option>
				{/each}
			</select>
		</div>

		<div class="min-w-48 flex-grow">
			<label for="value-input" class="mb-1 block text-sm font-medium text-gray-700">Value</label>
			<input
				id="value-input"
				type="text"
				class="w-full rounded-md border border-gray-300 px-3 py-2 shadow-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
				bind:value={currentValue}
				onkeyup={(e) => {
					if (e.key === 'Enter') addCondition();
				}}
			/>
		</div>

		<div>
			<button
				type="button"
				class="h-full rounded-md bg-blue-600 px-4 py-2 text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500"
				onclick={addCondition}
				disabled={currentValue.trim() === ''}
			>
				Add Condition
			</button>
		</div>
	</div>

	<!-- Display current conditions -->
	{#if conditions.length > 0}
		<div class="mt-4 rounded-md border border-gray-200 bg-gray-50 p-3">
			<h3 class="mb-2 text-sm font-medium text-gray-900">Active Filters ({conditions.length})</h3>
			<ul class="space-y-1">
				{#each conditions as cond, i}
					<li class="flex items-center justify-between rounded border bg-white p-2">
						<span class="font-mono text-sm">
							{cond.field}
							{cond.operator} "{cond.value}"
						</span>
						<button
							type="button"
							class="text-sm text-red-600 hover:text-red-800"
							onclick={() => removeCondition(i)}
						>
							Remove
						</button>
					</li>
				{/each}
			</ul>
		</div>
	{/if}

	<!-- Action buttons -->
	<div class="flex gap-3">
		<button
			type="button"
			class="rounded-md bg-green-600 px-4 py-2 text-white hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-green-500"
			onclick={handleQuery}
		>
			Query
		</button>
		<button
			type="button"
			class="rounded-md bg-gray-600 px-4 py-2 text-white hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-gray-500"
			onclick={handleReset}
		>
			Reset
		</button>
	</div>
</div>

<style>
	.query-editor {
		width: 100%;
	}
</style>
