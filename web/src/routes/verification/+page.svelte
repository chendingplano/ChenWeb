<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';

	let status = $state<'loading' | 'input' | 'submitting' | 'success' | 'error'>('loading');
	let errorMessage = $state('');
	let code = $state('');
	let flowId = $state('');

	onMount(async () => {
		flowId = page.url.searchParams.get('flow') ?? '';

		if (!flowId) {
			await goto('/login', { replaceState: true });
			return;
		}

		try {
			const res = await fetch(`/auth/verification/flow?id=${encodeURIComponent(flowId)}`);

			if (!res.ok) {
				status = 'error';
				errorMessage = 'The verification flow could not be found. It may have expired.';
				return;
			}

			const flow = await res.json();
			if (flow.state === 'passed_challenge') {
				status = 'success';
				return;
			}

			const codeNode = flow.ui?.nodes?.find(
				(node: { attributes?: { name?: string; value?: string } }) => node.attributes?.name === 'code'
			);
			const prefilledCode = codeNode?.attributes?.value;

			if (prefilledCode) {
				code = prefilledCode;
				await submitCode(prefilledCode);
				return;
			}

			status = 'input';
		} catch {
			status = 'error';
			errorMessage = 'Could not connect to the authentication service. Please try again later.';
		}
	});

	async function submitCode(codeValue: string) {
		status = 'submitting';
		errorMessage = '';

		try {
			const res = await fetch('/auth/verification', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ flow_id: flowId, code: codeValue })
			});

			const result = await res.json().catch(() => ({}));
			if (res.ok && result.state === 'passed_challenge') {
				status = 'success';
				return;
			}

			const msg =
				result.ui?.messages?.[0]?.text ||
				result.ui?.nodes?.find((node: { messages?: { text?: string }[] }) => node.messages?.length)
					?.messages?.[0]?.text ||
				result.message;

			errorMessage = msg || 'The code is invalid or has expired.';
			status = 'input';
		} catch {
			errorMessage = 'Could not connect to the authentication service. Please try again later.';
			status = 'input';
		}
	}

	function handleSubmit() {
		if (code.length !== 6) {
			errorMessage = 'Please enter the 6-digit code from your email.';
			return;
		}
		void submitCode(code);
	}

	function handleInput(event: Event) {
		const input = event.target as HTMLInputElement;
		input.value = input.value.replace(/\D/g, '').slice(0, 6);
		code = input.value;
		errorMessage = '';
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter') {
			handleSubmit();
		}
	}
</script>

<div class="verification-shell">
	<div class="verification-card">
		{#if status === 'loading' || status === 'submitting'}
			<h1>{status === 'loading' ? 'Loading verification...' : 'Verifying your email...'}</h1>
			<p>Please wait a moment.</p>
		{:else if status === 'input'}
			<h1>Check Your Email</h1>
			<p>We sent a 6-digit verification code to your email. Enter it below to verify your account.</p>
			<input
				type="text"
				inputmode="numeric"
				autocomplete="one-time-code"
				maxlength="6"
				placeholder="000000"
				bind:value={code}
				oninput={handleInput}
				onkeydown={handleKeydown}
			/>
			{#if errorMessage}
				<p class="error">{errorMessage}</p>
			{/if}
			<button type="button" onclick={handleSubmit} disabled={code.length !== 6}>Verify Email</button>
			<a href="/login">Back to login</a>
		{:else if status === 'success'}
			<h1>Email Verified</h1>
			<p>Your email has been verified successfully.</p>
			<button type="button" onclick={() => goto('/dashboard')}>Continue</button>
		{:else}
			<h1>Verification Failed</h1>
			<p>{errorMessage}</p>
			<a href="/login">Back to login</a>
		{/if}
	</div>
</div>

<style>
	.verification-shell {
		min-height: 100vh;
		display: grid;
		place-items: center;
		padding: 1.5rem;
		background:
			radial-gradient(circle at top, rgba(237, 214, 164, 0.25), transparent 32rem),
			linear-gradient(180deg, #f8f4eb 0%, #efe7d8 100%);
	}

	.verification-card {
		width: min(100%, 26rem);
		display: grid;
		gap: 1rem;
		padding: 2rem;
		border-radius: 1.25rem;
		background: rgba(255, 255, 255, 0.92);
		box-shadow: 0 24px 60px rgba(75, 56, 19, 0.15);
		text-align: center;
	}

	h1 {
		margin: 0;
		font-size: 1.75rem;
	}

	p {
		margin: 0;
		color: #5f5748;
		line-height: 1.5;
	}

	input {
		width: 100%;
		padding: 0.9rem 1rem;
		border: 2px solid #d7ccb4;
		border-radius: 0.9rem;
		font-size: 2rem;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		text-align: center;
		letter-spacing: 0.45rem;
		box-sizing: border-box;
	}

	input:focus {
		outline: none;
		border-color: #b38a2d;
		box-shadow: 0 0 0 4px rgba(179, 138, 45, 0.15);
	}

	button,
	a {
		width: 100%;
		box-sizing: border-box;
	}

	button {
		border: none;
		border-radius: 999px;
		padding: 0.9rem 1rem;
		font-size: 1rem;
		font-weight: 600;
		cursor: pointer;
		color: #fff;
		background: linear-gradient(135deg, #b38a2d 0%, #8b6623 100%);
	}

	button:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}

	a {
		display: inline-block;
		color: #6f6248;
		text-decoration: none;
	}

	.error {
		color: #b42318;
	}
</style>
