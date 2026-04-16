<script lang="ts">
	import { goto } from '$app/navigation';

	type Method = 'google' | 'github' | 'email';

	let selectedMethod = $state<Method>('email');
	let isLoading = $state(false);
	let email = $state('');
	let password = $state('');
	let errorMessage = $state('');
	let successMessage = $state('');

	async function handleGoogleRegister() {
		isLoading = true;
		errorMessage = '';
		successMessage = '';
		try {
			await goto('/api/auth/google');
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : 'Google registration failed.';
		} finally {
			isLoading = false;
		}
	}

	async function handleGitHubRegister() {
		isLoading = true;
		errorMessage = '';
		successMessage = '';
		try {
			const response = await fetch('/api/auth/github', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' }
			});
			if (!response.ok) {
				throw new Error('GitHub registration failed. Please try again.');
			}
			successMessage = 'GitHub registration started. Continue in the opened flow.';
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : 'GitHub registration failed.';
		} finally {
			isLoading = false;
		}
	}

	async function handleEmailRegister(event: SubmitEvent) {
		event.preventDefault();
		isLoading = true;
		errorMessage = '';
		successMessage = '';
		try {
			const response = await fetch('/api/auth/email', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email, password })
			});

			if (!response.ok) {
				throw new Error('Email registration failed. Please try again.');
			}

			successMessage = 'Registration successful. Please check your email for next steps.';
			email = '';
			password = '';
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : 'Email registration failed.';
		} finally {
			isLoading = false;
		}
	}
</script>

<svelte:head>
	<title>Register | AI Builder Workspace</title>
	<meta
		name="description"
		content="Create your account to build AI-enabled applications and manage your personal knowledge base."
	/>
</svelte:head>

<div class="auth-page">
	<div class="atmosphere"></div>
	<div class="grid-overlay"></div>

	<section class="showcase">
		<p class="badge">AI Builder Platform</p>
		<h1>Build AI products faster.<br />Grow a knowledge base that compounds.</h1>
		<p>
			One workspace for vibe-based app development, AI orchestration, and personal knowledge
			management.
		</p>
		<div class="pill-row">
			<span>App Prototyping</span>
			<span>Agent Workflows</span>
			<span>Knowledge Graph</span>
		</div>
	</section>

	<section class="auth-card" aria-label="Registration form">
		<div class="auth-header">
			<h2>Create Account</h2>
			<p>Choose your preferred sign-up method.</p>
		</div>

		<div class="method-tabs" role="tablist" aria-label="Registration methods">
			<button
				type="button"
				class:active={selectedMethod === 'google'}
				onclick={() => {
					selectedMethod = 'google';
					errorMessage = '';
					successMessage = '';
				}}
			>
				Google
			</button>
			<button
				type="button"
				class:active={selectedMethod === 'github'}
				onclick={() => {
					selectedMethod = 'github';
					errorMessage = '';
					successMessage = '';
				}}
			>
				GitHub
			</button>
			<button
				type="button"
				class:active={selectedMethod === 'email'}
				onclick={() => {
					selectedMethod = 'email';
					errorMessage = '';
					successMessage = '';
				}}
			>
				Email
			</button>
		</div>

		{#if selectedMethod === 'google'}
			<div class="method-panel">
				<p>Fastest option for getting started with your existing Google account.</p>
				<button type="button" class="primary" onclick={handleGoogleRegister} disabled={isLoading}>
					{isLoading ? 'Redirecting...' : 'Continue with Google'}
				</button>
			</div>
		{:else if selectedMethod === 'github'}
			<div class="method-panel">
				<p>Perfect for developer workflows, repositories, and AI coding pipelines.</p>
				<button type="button" class="primary" onclick={handleGitHubRegister} disabled={isLoading}>
					{isLoading ? 'Connecting...' : 'Continue with GitHub'}
				</button>
			</div>
		{:else}
			<form class="method-panel" onsubmit={handleEmailRegister}>
				<p>Use email if you prefer a classic account with password login.</p>
				<label for="email">Email</label>
				<input id="email" type="email" placeholder="you@company.com" bind:value={email} required />
				<label for="password">Password</label>
				<input
					id="password"
					type="password"
					placeholder="At least 8 characters"
					bind:value={password}
					required
					minlength="8"
				/>
				<button type="submit" class="primary" disabled={isLoading}>
					{isLoading ? 'Creating account...' : 'Create account'}
				</button>
			</form>
		{/if}

		{#if errorMessage}
			<p class="notice error" role="alert">{errorMessage}</p>
		{/if}
		{#if successMessage}
			<p class="notice success">{successMessage}</p>
		{/if}
	</section>
</div>

<style>
	@import url('https://fonts.googleapis.com/css2?family=Outfit:wght@400;500;600;700&family=Plus+Jakarta+Sans:wght@400;500;600&display=swap');

	:global(body) {
		margin: 0;
		font-family: 'Plus Jakarta Sans', sans-serif;
		background: #060b14;
	}

	.auth-page {
		--page-pad-x: 48px;
		--showcase-width: 860px;
		--auth-gap-x: 110px;
		--auth-offset-y: -90px;
		position: relative;
		isolation: isolate;
		min-height: 100vh;
		display: block;
		padding: 48px var(--page-pad-x);
		overflow: hidden;
		background:
			radial-gradient(circle at 10% 15%, rgba(36, 157, 255, 0.22), transparent 36%),
			radial-gradient(circle at 85% 10%, rgba(106, 255, 207, 0.18), transparent 36%), #060b14;
	}

	.atmosphere,
	.grid-overlay {
		position: absolute;
		inset: 0;
		pointer-events: none;
	}

	.atmosphere {
		z-index: -2;
		background: radial-gradient(circle at 74% 58%, rgba(26, 228, 173, 0.15), transparent 35%);
	}

	.grid-overlay {
		z-index: -1;
		opacity: 0.18;
		background-image:
			linear-gradient(rgba(190, 210, 255, 0.14) 1px, transparent 1px),
			linear-gradient(90deg, rgba(190, 210, 255, 0.14) 1px, transparent 1px);
		background-size: 42px 42px;
	}

	.showcase {
		width: var(--showcase-width);
		max-width: var(--showcase-width);
		padding-right: 0;
		animation: fadeUp 0.6s ease both;
	}

	.badge {
		display: inline-flex;
		align-items: center;
		padding: 0.35rem 0.75rem;
		border-radius: 999px;
		border: 1px solid rgba(161, 197, 255, 0.4);
		color: #dbe9ff;
		font-size: 0.78rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}

	h1 {
		margin: 1rem 0 0;
		font-family: 'Outfit', sans-serif;
		font-size: clamp(2rem, 5.5vw, 4.2rem);
		line-height: 1.04;
		letter-spacing: -0.01em;
		color: #f3f8ff;
		text-wrap: balance;
	}

	.showcase p {
		margin: 1rem 0 0;
		font-size: clamp(1rem, 1.7vw, 1.2rem);
		line-height: 1.7;
		color: #b7c7df;
		max-width: 58ch;
	}

	.pill-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.6rem;
		margin-top: 1.3rem;
	}

	.pill-row span {
		padding: 0.4rem 0.75rem;
		border-radius: 999px;
		font-size: 0.86rem;
		color: #d9e6ff;
		border: 1px solid rgba(161, 197, 255, 0.35);
		background: rgba(23, 38, 64, 0.55);
	}

	.auth-card {
		width: min(100%, 31rem);
		position: absolute;
		left: calc(var(--page-pad-x) + var(--showcase-width) + var(--auth-gap-x));
		top: calc(50% + var(--auth-offset-y));
		transform: translateY(-50%);
		z-index: 2;
		border-radius: 1.2rem;
		padding: 1.25rem;
		border: 1px solid rgba(174, 203, 255, 0.32);
		background: linear-gradient(160deg, rgba(14, 23, 38, 0.93), rgba(9, 16, 28, 0.91));
		box-shadow: 0 28px 80px rgba(0, 0, 0, 0.35);
		backdrop-filter: blur(10px);
		animation: fadeIn 0.75s ease both;
	}

	.auth-header h2 {
		margin: 0;
		font-family: 'Outfit', sans-serif;
		font-size: 1.55rem;
		color: #f4f8ff;
	}

	.auth-header p {
		margin: 0.45rem 0 0;
		color: #9db0cc;
		font-size: 0.95rem;
	}

	.method-tabs {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.45rem;
		margin-top: 1rem;
	}

	.method-tabs button {
		padding: 0.56rem 0.45rem;
		border-radius: 0.66rem;
		border: 1px solid rgba(169, 197, 255, 0.25);
		background: rgba(18, 30, 48, 0.7);
		color: #bad0f4;
		font-weight: 600;
		cursor: pointer;
		transition: all 0.18s ease;
	}

	.method-tabs button:hover {
		border-color: rgba(179, 224, 255, 0.6);
		color: #e7f2ff;
	}

	.method-tabs button.active {
		color: #061221;
		border-color: transparent;
		background: linear-gradient(135deg, #2ac7ff, #63f7c4);
	}

	.method-panel {
		display: grid;
		gap: 0.6rem;
		margin-top: 1rem;
	}

	.method-panel p {
		margin: 0 0 0.2rem;
		color: #a8bddf;
		font-size: 0.95rem;
		line-height: 1.6;
	}

	label {
		font-size: 0.84rem;
		font-weight: 600;
		color: #d9e8ff;
	}

	input {
		width: 100%;
		box-sizing: border-box;
		padding: 0.72rem 0.8rem;
		border-radius: 0.7rem;
		border: 1px solid rgba(167, 191, 228, 0.3);
		background: rgba(10, 20, 35, 0.78);
		color: #eff6ff;
		font-size: 0.96rem;
		outline: none;
	}

	input::placeholder {
		color: #8299ba;
	}

	input:focus {
		border-color: rgba(104, 232, 255, 0.85);
		box-shadow: 0 0 0 3px rgba(66, 180, 255, 0.18);
	}

	.primary {
		margin-top: 0.15rem;
		width: 100%;
		padding: 0.78rem 1rem;
		border: none;
		border-radius: 0.74rem;
		font-size: 0.96rem;
		font-weight: 700;
		cursor: pointer;
		color: #071423;
		background: linear-gradient(135deg, #2ac7ff, #63f7c4);
		transition:
			transform 0.16s ease,
			filter 0.16s ease;
	}

	.primary:hover:enabled {
		transform: translateY(-1px);
		filter: brightness(1.05);
	}

	.primary:disabled {
		opacity: 0.72;
		cursor: not-allowed;
	}

	.notice {
		margin: 0.8rem 0 0;
		padding: 0.6rem 0.72rem;
		border-radius: 0.66rem;
		font-size: 0.9rem;
	}

	.notice.error {
		background: rgba(255, 107, 122, 0.16);
		border: 1px solid rgba(255, 126, 139, 0.45);
		color: #ffc7ce;
	}

	.notice.success {
		background: rgba(99, 247, 196, 0.14);
		border: 1px solid rgba(99, 247, 196, 0.45);
		color: #c7ffef;
	}

	@keyframes fadeUp {
		from {
			opacity: 0;
			transform: translateY(12px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	@keyframes fadeIn {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}

	@media (max-width: 980px) {
		.auth-page {
			display: grid;
			grid-template-columns: 1fr;
			align-items: start;
			padding: clamp(1.2rem, 4vw, 3.25rem);
		}

		.showcase {
			width: auto;
			max-width: 52rem;
			padding-right: 0;
		}

		.auth-card {
			position: static;
			transform: none;
			width: 100%;
		}
	}
</style>
