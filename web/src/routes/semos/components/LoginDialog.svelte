<script lang="ts">
	import { loginPrompt } from '../loginPrompt.svelte';
	import LoginPanel from '$lib/components/login-01.svelte';
</script>

{#if loginPrompt.open}
	<div
		class="fixed inset-0 z-[100] flex items-center justify-center p-6"
		style="background:rgba(23,24,28,0.55);"
		onclick={() => loginPrompt.hide()}
		onkeydown={(e) => {
			if (e.key === 'Escape' || e.key === 'Enter' || e.key === ' ') loginPrompt.hide();
		}}
		role="button"
		tabindex="0"
	>
		<div
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			aria-label="Login"
			tabindex="0"
		>
			<LoginPanel>
				<button
					type="button"
					onclick={() => loginPrompt.hide()}
					class="mb-3 w-full rounded-[20px] bg-gray-200 py-2.5 text-center text-sm font-semibold text-[#747474] transition-colors hover:bg-gray-300 hover:text-black"
				>
					Cancel
				</button>
			</LoginPanel>
		</div>
	</div>
{/if}
