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
			class="flex flex-col items-center gap-4"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			aria-label="Login"
			tabindex="0"
		>
			<LoginPanel />
			<button
				type="button"
				onclick={() => loginPrompt.hide()}
				class="text-sm font-semibold text-white/80 transition-colors hover:text-white"
			>
				Cancel
			</button>
		</div>
	</div>
{/if}
