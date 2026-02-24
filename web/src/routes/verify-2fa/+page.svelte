<script lang="ts">
	let code = $state('');
	let error = $state('');
	let isSubmitting = $state(false);

	async function handleVerify() {
		if (!code || code.length !== 6) {
			error = 'Please enter a 6-digit code';
			return;
		}

		error = '';
		isSubmitting = true;

		try {
			const res = await fetch('/auth/totp/verify', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ code })
			});

			const data = await res.json();

			if (res.ok && data.status === 'ok') {
				// 2FA successful, redirect to dashboard
				window.location.href = data.redirect_url || '/dashboard';
			} else {
				error = data.message || 'Invalid verification code';
			}
		} catch (err) {
			error = `Network error: ${err}`;
		} finally {
			isSubmitting = false;
		}
	}

	function handleInput(event: Event) {
		const input = event.target as HTMLInputElement;
		// Only allow digits
		input.value = input.value.replace(/\D/g, '').slice(0, 6);
		code = input.value;
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter') {
			handleVerify();
		}
	}
</script>

<div class="container">
	<div class="card">
		<div class="icon">
			<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
				<rect width="18" height="11" x="3" y="11" rx="2" ry="2"/>
				<path d="M7 11V7a5 5 0 0 1 10 0v4"/>
			</svg>
		</div>

		<h1>Two-Factor Authentication</h1>
		<p class="subtitle">Enter the 6-digit code from your authenticator app</p>

		<div class="form">
			<input
				type="text"
				inputmode="numeric"
				autocomplete="one-time-code"
				maxlength="6"
				placeholder="000000"
				bind:value={code}
				oninput={handleInput}
				onkeydown={handleKeydown}
				class="code-input"
				class:error={error}
				disabled={isSubmitting}
			/>

			{#if error}
				<p class="error-message">{error}</p>
			{/if}

			<button
				onclick={handleVerify}
				disabled={isSubmitting || code.length !== 6}
				class="verify-btn"
			>
				{isSubmitting ? 'Verifying...' : 'Verify'}
			</button>

			<a href="/login" class="back-link">Back to login</a>
		</div>
	</div>
</div>

<style>
	.container {
		min-height: 100vh;
		display: flex;
		align-items: center;
		justify-content: center;
		background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
		padding: 20px;
	}

	.card {
		background: white;
		border-radius: 16px;
		padding: 40px;
		box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.25);
		max-width: 400px;
		width: 100%;
		text-align: center;
	}

	.icon {
		color: #667eea;
		margin-bottom: 20px;
	}

	h1 {
		font-size: 24px;
		font-weight: 700;
		color: #1a202c;
		margin: 0 0 8px 0;
	}

	.subtitle {
		color: #718096;
		font-size: 14px;
		margin: 0 0 32px 0;
	}

	.form {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.code-input {
		font-size: 32px;
		font-weight: 600;
		text-align: center;
		letter-spacing: 8px;
		padding: 16px;
		border: 2px solid #e2e8f0;
		border-radius: 12px;
		outline: none;
		transition: border-color 0.2s, box-shadow 0.2s;
		font-family: 'Courier New', monospace;
	}

	.code-input:focus {
		border-color: #667eea;
		box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.2);
	}

	.code-input.error {
		border-color: #e53e3e;
	}

	.code-input:disabled {
		background: #f7fafc;
		cursor: not-allowed;
	}

	.error-message {
		color: #e53e3e;
		font-size: 14px;
		margin: 0;
	}

	.verify-btn {
		background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
		color: white;
		font-size: 16px;
		font-weight: 600;
		padding: 14px 24px;
		border: none;
		border-radius: 12px;
		cursor: pointer;
		transition: transform 0.2s, box-shadow 0.2s;
	}

	.verify-btn:hover:not(:disabled) {
		transform: translateY(-2px);
		box-shadow: 0 10px 20px rgba(102, 126, 234, 0.3);
	}

	.verify-btn:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.back-link {
		color: #718096;
		font-size: 14px;
		text-decoration: none;
		margin-top: 8px;
	}

	.back-link:hover {
		color: #667eea;
		text-decoration: underline;
	}
</style>
