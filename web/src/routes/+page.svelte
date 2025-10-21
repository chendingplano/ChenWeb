<script lang="ts">
	import { onMount } from 'svelte';

	let maxNum = $state<number>(100);
	let randNum = $state<number>(0);

	onMount(() => {
		const clear = setInterval(() => {
			fetch('/api/v1/randnum?max=' + maxNum)
				.then((v) => v.json() as Promise<{ number: number }>)
				.then((v) => (randNum = v.number));
		}, 1000);
		return () => {
			clearInterval(clear);
		};
	});

	async function handleButtonClick() {
		try {
			console.log("Button pressed")
			const response = await fetch('/api/v1/button-click', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				}
			});
			
			if (response.ok) {
				const result = await response.json();
				console.log('Go function invoked successfully (029)');
				console.log('Backend message:', result.message);
				// Handle success response if needed
			} else {
				console.error('Failed to invoke Go function');
			}
		} catch (error) {
			console.error('Error invoking Go function:', error);
		}
	}
</script>

<div style="width: 1200px; display: flex; margin: 0 auto;">
	<!-- Left pane (200px) -->
	<div style="width: 200px; background-color: lightblue; padding: 10px;">
		<h3>Left Pane</h3>
	</div>
	
	<!-- Center pane (700px) -->
	<div style="width: 700px; background-color: #fffed0; padding: 10px;">
		<h3>Center Pane</h3>
		<input type="number" bind:value={() => maxNum, (v: unknown) => (maxNum = +(v as string))} />
		<p>Your random number is: {randNum}</p>
		<button 
			class="bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded"
			onclick={handleButtonClick}
		>
			Press Me!
		</button>
	</div>
	
	<!-- Right pane (300px) -->
	<div style="width: 300px; background-color: lightblue; padding: 10px;">
		<h3>Right Pane</h3>
	</div>
</div>
