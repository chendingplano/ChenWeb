<script lang="ts">
	import { page } from '$app/stores';

	let password = '';
	let confirmPassword = '';
	let message = '';
	let isLoading = false;
	let showPassword = false;

	// Get the token from the query string (?token=...)
	$: token = $page.url.searchParams.get('token') || '';

	async function handleResetPassword() {
		message = '';

		if (!password) {
			message = 'Please enter a new password.';
			return;
		}

		if (password.length < 8) {
			message = 'Password must be at least 8 characters long.';
			return;
		}

		if (password !== confirmPassword) {
			message = 'Passwords do not match.';
			return;
		}

		isLoading = true;
		try {
			const res = await fetch('/auth/email/reset/confirm1', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ token, password })
			});

			if (res.ok) {
				message = 'Your password has been reset successfully! Redirecting to login...';
				setTimeout(() => {
					window.location.href = '/login';
				}, 2000);
			} else {
				const errorMsg = await res.text();
				message = errorMsg || 'Failed to reset password. Please try again.';
			}
		} catch (err) {
			message = 'Network error. Please check your connection and try again.';
		} finally {
			isLoading = false;
		}
	}
</script>

<div
	class="flex min-h-screen items-center justify-center bg-gradient-to-br from-gray-50 to-gray-100 p-4"
>
	<div
		class="w-full max-w-md overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-lg"
	>
		<div class="bg-indigo-600 px-6 py-6 text-center">
			<h1 class="text-2xl font-bold text-white">Reset Your Password</h1>
			<p class="mt-1 text-indigo-200">Enter a new secure password below</p>
		</div>

		<div class="p-6">
			{#if message}
				<div
					class="mb-5 rounded-lg p-3 text-sm font-medium {message.includes('successfully')
						? 'bg-green-100 text-green-800'
						: 'bg-red-100 text-red-800'}"
				>
					{message}
				</div>
			{/if}

			<form class="space-y-5" on:submit|preventDefault={handleResetPassword}>
				<div>
					<label for="password" class="mb-1 block text-sm font-medium text-gray-700">
						New Password
					</label>
					<div class="relative">
						<input
							id="password"
							type={showPassword ? 'text' : 'password'}
							placeholder="••••••••"
							bind:value={password}
							required
							class="w-full rounded-lg border border-gray-300 px-4 py-3 transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500"
						/>
						<button
							type="button"
							class="absolute right-3 top-3.5 text-gray-500 hover:text-gray-700"
							on:click={() => (showPassword = !showPassword)}
						>
							{#if showPassword}
								<svg
									xmlns="http://www.w3.org/2000/svg"
									class="h-5 w-5"
									fill="none"
									viewBox="0 0 24 24"
									stroke="currentColor"
								>
									<path
										stroke-linecap="round"
										stroke-linejoin="round"
										stroke-width="2"
										d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
									/>
									<path
										stroke-linecap="round"
										stroke-linejoin="round"
										stroke-width="2"
										d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"
									/>
								</svg>
							{:else}
								<svg
									xmlns="http://www.w3.org/2000/svg"
									class="h-5 w-5"
									fill="none"
									viewBox="0 0 24 24"
									stroke="currentColor"
								>
									<path
										stroke-linecap="round"
										stroke-linejoin="round"
										stroke-width="2"
										d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21"
									/>
								</svg>
							{/if}
						</button>
					</div>
					<p class="mt-1 text-xs text-gray-500">Must be at least 8 characters</p>
				</div>

				<div>
					<label for="confirmPassword" class="mb-1 block text-sm font-medium text-gray-700">
						Confirm New Password
					</label>
					<input
						id="confirmPassword"
						type="password"
						placeholder="••••••••"
						bind:value={confirmPassword}
						required
						class="w-full rounded-lg border border-gray-300 px-4 py-3 transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500"
					/>
				</div>

				<button
					type="submit"
					disabled={isLoading}
					class="flex w-full items-center justify-center rounded-lg bg-indigo-600 px-4 py-3 font-semibold text-white shadow-md transition duration-200 hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-70"
				>
					{#if isLoading}
						<svg
							class="-ml-1 mr-2 h-4 w-4 animate-spin text-white"
							xmlns="http://www.w3.org/2000/svg"
							fill="none"
							viewBox="0 0 24 24"
						>
							<circle
								class="opacity-25"
								cx="12"
								cy="12"
								r="10"
								stroke="currentColor"
								stroke-width="4"
							></circle>
							<path
								class="opacity-75"
								fill="currentColor"
								d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
							></path>
						</svg>
						Resetting...
					{:else}
						Reset Password
					{/if}
				</button>
			</form>

			<div class="mt-6 border-t border-gray-200 pt-5 text-center">
				<a
					href="/login"
					class="text-sm font-medium text-indigo-600 transition hover:text-indigo-500"
				>
					← Back to Login AA
				</a>
			</div>
		</div>
	</div>
</div>
