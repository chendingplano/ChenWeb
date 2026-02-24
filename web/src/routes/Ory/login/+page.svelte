<script lang="ts">
	import { onMount } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Card from '$lib/components/ui/card';
	import * as Tabs from '$lib/components/ui/tabs';
	import {
		createLoginFlow,
		createRegistrationFlow,
		createRecoveryFlow,
		submitLogin,
		submitEmailRegistration,
		submitUsernameRegistration,
		submitRecoveryEmail,
		extractCsrfToken,
		initiateGoogleLogin,
		hasOidcProvider,
		logout,
		getSession,
		OryErrorIds,
		type OryLoginFlow,
		type OryRegistrationFlow,
		type OryRecoveryFlow,
		type OrySession
	} from '$lib/services/ory';

	// Page state
	type PageMode = 'login' | 'signup' | 'forgot';
	let mode = $state<PageMode>('login');

	// Login state
	type LoginMethod = 'email' | 'username';
	let loginMethod = $state<LoginMethod>('email');

	// Signup state
	type SignupMethod = 'email' | 'username';
	let signupMethod = $state<SignupMethod>('email');

	// Form fields
	let email = $state('');
	let username = $state('');
	let password = $state('');
	let confirmPassword = $state('');
	let firstName = $state('');
	let lastName = $state('');

	// Flow state
	let loginFlow = $state<OryLoginFlow | null>(null);
	let registrationFlow = $state<OryRegistrationFlow | null>(null);
	let recoveryFlow = $state<OryRecoveryFlow | null>(null);

	// UI state
	let isLoading = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');
	let showPassword = $state(false);
	let hasGoogleOidc = $state(false);
	let isAlreadyLoggedIn = $state(false);
	let currentSession = $state<OrySession | null>(null);

	onMount(async () => {
		await initializeFlow();
	});

	async function initializeFlow() {
		isLoading = true;
		errorMessage = '';
		isAlreadyLoggedIn = false;
		currentSession = null;

		try {
			if (mode === 'login') {
				const result = await createLoginFlow();
				if (result.success && result.data) {
					loginFlow = result.data;
					hasGoogleOidc = hasOidcProvider(result.data.ui, 'google');
				} else if (result.errorId === OryErrorIds.SESSION_ALREADY_AVAILABLE) {
					// User is already logged in - fetch session info
					isAlreadyLoggedIn = true;
					const sessionResult = await getSession();
					if (sessionResult.success && sessionResult.data) {
						currentSession = sessionResult.data;
					}
				} else {
					errorMessage = result.error || 'Failed to initialize login flow';
				}
			} else if (mode === 'signup') {
				const result = await createRegistrationFlow();
				if (result.success && result.data) {
					registrationFlow = result.data;
					hasGoogleOidc = hasOidcProvider(result.data.ui, 'google');
				} else if (result.errorId === OryErrorIds.SESSION_ALREADY_AVAILABLE) {
					// User is already logged in - fetch session info
					isAlreadyLoggedIn = true;
					const sessionResult = await getSession();
					if (sessionResult.success && sessionResult.data) {
						currentSession = sessionResult.data;
					}
				} else {
					errorMessage = result.error || 'Failed to initialize registration flow';
				}
			} else if (mode === 'forgot') {
				const result = await createRecoveryFlow();
				if (result.success && result.data) {
					recoveryFlow = result.data;
				} else {
					errorMessage = result.error || 'Failed to initialize recovery flow';
				}
			}
		} catch (err) {
			errorMessage = err instanceof Error ? err.message : 'An unexpected error occurred';
		} finally {
			isLoading = false;
		}
	}

	async function handleLogin(event: SubmitEvent) {
		event.preventDefault();
		if (!loginFlow) return;

		isLoading = true;
		errorMessage = '';

		try {
			const identifier = loginMethod === 'email' ? email : username;
			const csrfToken = extractCsrfToken(loginFlow.ui);

			if (!csrfToken) {
				errorMessage = 'Security token missing. Please refresh the page.';
				return;
			}

			const result = await submitLogin(loginFlow.id, identifier, password, csrfToken);

			if (result.success) {
				if (result.redirectTo) {
					window.location.href = result.redirectTo;
				} else {
					successMessage = 'Login successful!';
					window.location.href = '/login';
				}
			} else {
				errorMessage = result.error || 'Login failed';
				// Refresh the flow to get a new CSRF token
				await initializeFlow();
			}
		} catch (err) {
			errorMessage = err instanceof Error ? err.message : 'An unexpected error occurred';
		} finally {
			isLoading = false;
		}
	}

	async function handleSignup(event: SubmitEvent) {
		event.preventDefault();
		if (!registrationFlow) return;

		// Validate password confirmation
		if (password !== confirmPassword) {
			errorMessage = 'Passwords do not match';
			return;
		}

		isLoading = true;
		errorMessage = '';

		try {
			const csrfToken = extractCsrfToken(registrationFlow.ui);

			if (!csrfToken) {
				errorMessage = 'Security token missing. Please refresh the page.';
				return;
			}

			let result;
			if (signupMethod === 'email') {
				if (!email) {
					errorMessage = 'Email is required';
					return;
				}
				result = await submitEmailRegistration(
					registrationFlow.id,
					email,
					password,
					firstName,
					lastName,
					csrfToken
				);
			} else {
				if (!username) {
					errorMessage = 'Username is required';
					return;
				}
				result = await submitUsernameRegistration(
					registrationFlow.id,
					username,
					password,
					firstName,
					lastName,
					csrfToken
				);
			}

			if (result.success) {
				if (result.redirectTo) {
					window.location.href = result.redirectTo;
				} else {
					successMessage = 'Registration successful! Please check your email for verification.';
					mode = 'login';
					await initializeFlow();
				}
			} else {
				errorMessage = result.error || 'Registration failed';
				// Refresh the flow to get a new CSRF token
				await initializeFlow();
			}
		} catch (err) {
			errorMessage = err instanceof Error ? err.message : 'An unexpected error occurred';
		} finally {
			isLoading = false;
		}
	}

	async function handleForgotPassword(event: SubmitEvent) {
		event.preventDefault();
		if (!recoveryFlow) return;

		if (!email) {
			errorMessage = 'Email is required for password recovery';
			return;
		}

		isLoading = true;
		errorMessage = '';

		try {
			const csrfToken = extractCsrfToken(recoveryFlow.ui);

			if (!csrfToken) {
				errorMessage = 'Security token missing. Please refresh the page.';
				return;
			}

			const result = await submitRecoveryEmail(recoveryFlow.id, email, csrfToken);

			if (result.success) {
				successMessage = 'Recovery email sent! Please check your inbox.';
			} else {
				if (result.error?.includes('no recovery')) {
					errorMessage =
						'No account found with this email, or the account does not have email recovery enabled.';
				} else {
					errorMessage = result.error || 'Failed to send recovery email';
				}
			}
		} catch (err) {
			errorMessage = err instanceof Error ? err.message : 'An unexpected error occurred';
		} finally {
			isLoading = false;
		}
	}

	async function handleGoogleLogin() {
		const flow = mode === 'login' ? loginFlow : registrationFlow;
		if (!flow) return;

		isLoading = true;
		errorMessage = '';

		try {
			const csrfToken = extractCsrfToken(flow.ui);

			if (!csrfToken) {
				errorMessage = 'Security token missing. Please refresh the page.';
				return;
			}

			await initiateGoogleLogin(flow.id, csrfToken);
		} catch (err) {
			errorMessage = err instanceof Error ? err.message : 'Failed to initiate Google login';
			isLoading = false;
		}
	}

	function switchMode(newMode: PageMode) {
		mode = newMode;
		errorMessage = '';
		successMessage = '';
		initializeFlow();
	}

	function clearMessages() {
		errorMessage = '';
		successMessage = '';
	}

	async function handleLogout() {
		isLoading = true;
		errorMessage = '';

		try {
			const result = await logout();
			if (result.success && result.data?.logout_url) {
				// Perform logout by navigating to the logout URL
				window.location.href = result.data.logout_url;
			} else {
				errorMessage = result.error || 'Failed to log out';
			}
		} catch (err) {
			errorMessage =
				err instanceof Error ? err.message : 'An unexpected error occurred during logout';
		} finally {
			isLoading = false;
		}
	}

	function getUserDisplayName(): string {
		if (!currentSession?.identity?.traits) return 'User';
		const traits = currentSession.identity.traits;
		if (traits.name?.first || traits.name?.last) {
			return [traits.name.first, traits.name.last].filter(Boolean).join(' ');
		}
		return traits.email || traits.username || 'User';
	}
</script>

<div
	class="flex min-h-screen items-center justify-center bg-gradient-to-br from-gray-50 to-gray-100 p-4"
>
	<Card.Root class="w-full max-w-md">
		<Card.Header class="space-y-1 text-center">
			<Card.Title class="text-2xl font-bold">
				{#if isAlreadyLoggedIn}
					Already Signed In
				{:else if mode === 'login'}
					Welcome Back
				{:else if mode === 'signup'}
					Create Account
				{:else}
					Reset Password
				{/if}
			</Card.Title>
			<Card.Description>
				{#if isAlreadyLoggedIn}
					You have an active session
				{:else if mode === 'login'}
					Sign in to your account
				{:else if mode === 'signup'}
					Create a new account to get started
				{:else}
					Enter your email to reset your password
				{/if}
			</Card.Description>
		</Card.Header>

		<Card.Content class="space-y-4">
			<!-- Already Logged In Message -->
			{#if isAlreadyLoggedIn}
				<div class="space-y-4">
					<div class="rounded-lg bg-blue-50 p-4 text-sm text-blue-800">
						<p class="font-medium">You are already logged in</p>
						<p class="mt-1">
							Signed in as <span class="font-semibold">{getUserDisplayName()}</span>
							{#if currentSession?.identity?.traits?.email}
								<span class="text-blue-600">({currentSession.identity.traits.email})</span>
							{/if}
						</p>
					</div>

					<div class="flex flex-col gap-2">
						<Button class="w-full" onclick={() => (window.location.href = '/')}>
							Continue to Dashboard
						</Button>
						<Button variant="outline" class="w-full" onclick={handleLogout} disabled={isLoading}>
							{#if isLoading}
								<svg
									class="mr-2 h-4 w-4 animate-spin"
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
								Logging out...
							{:else}
								Log out and sign in as a different user
							{/if}
						</Button>
					</div>
				</div>
			{:else}
				<!-- Error Message -->
				{#if errorMessage}
					<div class="rounded-lg bg-red-50 p-3 text-sm text-red-600">
						{errorMessage}
					</div>
				{/if}

				<!-- Success Message -->
				{#if successMessage}
					<div class="rounded-lg bg-green-50 p-3 text-sm text-green-600">
						{successMessage}
					</div>
				{/if}

				<!-- Login Form -->
				{#if mode === 'login'}
					<Tabs.Root bind:value={loginMethod} onValueChange={clearMessages}>
						<Tabs.List class="grid w-full grid-cols-2">
							<Tabs.Trigger value="email">Email</Tabs.Trigger>
							<Tabs.Trigger value="username">Username</Tabs.Trigger>
						</Tabs.List>

						<form onsubmit={handleLogin} class="mt-4 space-y-4">
							<Tabs.Content value="email" class="space-y-4">
								<div class="space-y-2">
									<Label for="login-email">Email</Label>
									<Input
										id="login-email"
										type="email"
										placeholder="you@example.com"
										bind:value={email}
										required={loginMethod === 'email'}
										disabled={isLoading}
									/>
								</div>
							</Tabs.Content>

							<Tabs.Content value="username" class="space-y-4">
								<div class="space-y-2">
									<Label for="login-username">Username</Label>
									<Input
										id="login-username"
										type="text"
										placeholder="your_username"
										bind:value={username}
										required={loginMethod === 'username'}
										disabled={isLoading}
									/>
								</div>
							</Tabs.Content>

							<div class="space-y-2">
								<Label for="login-password">Password</Label>
								<div class="relative">
									<Input
										id="login-password"
										type={showPassword ? 'text' : 'password'}
										placeholder="Enter your password"
										bind:value={password}
										required
										disabled={isLoading}
									/>
									<button
										type="button"
										class="absolute top-1/2 right-3 -translate-y-1/2 text-gray-500 hover:text-gray-700"
										onclick={() => (showPassword = !showPassword)}
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
							</div>

							<div class="flex justify-end">
								<button
									type="button"
									class="text-sm text-blue-600 hover:underline"
									onclick={() => switchMode('forgot')}
								>
									Forgot password?
								</button>
							</div>

							<Button type="submit" class="w-full" disabled={isLoading}>
								{#if isLoading}
									<svg
										class="mr-2 h-4 w-4 animate-spin"
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
									Signing in...
								{:else}
									Sign In
								{/if}
							</Button>
						</form>
					</Tabs.Root>

					<!-- Signup Form -->
				{:else if mode === 'signup'}
					<Tabs.Root bind:value={signupMethod} onValueChange={clearMessages}>
						<Tabs.List class="grid w-full grid-cols-2">
							<Tabs.Trigger value="email">Email</Tabs.Trigger>
							<Tabs.Trigger value="username">Username</Tabs.Trigger>
						</Tabs.List>

						<form onsubmit={handleSignup} class="mt-4 space-y-4">
							<Tabs.Content value="email" class="space-y-4">
								<div class="space-y-2">
									<Label for="signup-email">Email</Label>
									<Input
										id="signup-email"
										type="email"
										placeholder="you@example.com"
										bind:value={email}
										required={signupMethod === 'email'}
										disabled={isLoading}
									/>
								</div>
							</Tabs.Content>

							<Tabs.Content value="username" class="space-y-4">
								<div class="space-y-2">
									<Label for="signup-username">Username</Label>
									<Input
										id="signup-username"
										type="text"
										placeholder="your_username"
										bind:value={username}
										required={signupMethod === 'username'}
										disabled={isLoading}
									/>
									<p class="text-xs text-gray-500">
										Only letters, numbers, underscores, and hyphens allowed
									</p>
								</div>
							</Tabs.Content>

							<div class="grid grid-cols-2 gap-4">
								<div class="space-y-2">
									<Label for="signup-firstname">First Name</Label>
									<Input
										id="signup-firstname"
										type="text"
										placeholder="John"
										bind:value={firstName}
										disabled={isLoading}
									/>
								</div>
								<div class="space-y-2">
									<Label for="signup-lastname">Last Name</Label>
									<Input
										id="signup-lastname"
										type="text"
										placeholder="Doe"
										bind:value={lastName}
										disabled={isLoading}
									/>
								</div>
							</div>

							<div class="space-y-2">
								<Label for="signup-password">Password</Label>
								<div class="relative">
									<Input
										id="signup-password"
										type={showPassword ? 'text' : 'password'}
										placeholder="Create a password"
										bind:value={password}
										required
										disabled={isLoading}
									/>
									<button
										type="button"
										class="absolute top-1/2 right-3 -translate-y-1/2 text-gray-500 hover:text-gray-700"
										onclick={() => (showPassword = !showPassword)}
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
								<p class="text-xs text-gray-500">Must be at least 8 characters</p>
							</div>

							<div class="space-y-2">
								<Label for="signup-confirm-password">Confirm Password</Label>
								<Input
									id="signup-confirm-password"
									type="password"
									placeholder="Confirm your password"
									bind:value={confirmPassword}
									required
									disabled={isLoading}
								/>
							</div>

							<Button type="submit" class="w-full" disabled={isLoading}>
								{#if isLoading}
									<svg
										class="mr-2 h-4 w-4 animate-spin"
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
									Creating account...
								{:else}
									Create Account
								{/if}
							</Button>
						</form>
					</Tabs.Root>

					<!-- Forgot Password Form -->
				{:else if mode === 'forgot'}
					<form onsubmit={handleForgotPassword} class="space-y-4">
						<div class="space-y-2">
							<Label for="forgot-email">Email</Label>
							<Input
								id="forgot-email"
								type="email"
								placeholder="you@example.com"
								bind:value={email}
								required
								disabled={isLoading}
							/>
							<p class="text-xs text-gray-500">
								Password reset is only available for accounts registered with an email address. If
								you signed up with a username only, please contact support.
							</p>
						</div>

						<Button type="submit" class="w-full" disabled={isLoading}>
							{#if isLoading}
								<svg
									class="mr-2 h-4 w-4 animate-spin"
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
								Sending...
							{:else}
								Send Reset Link
							{/if}
						</Button>
					</form>
				{/if}

				<!-- Divider -->
				{#if mode !== 'forgot'}
					<div class="relative">
						<div class="absolute inset-0 flex items-center">
							<span class="w-full border-t"></span>
						</div>
						<div class="relative flex justify-center text-xs uppercase">
							<span class="bg-white px-2 text-gray-500">Or continue with</span>
						</div>
					</div>

					<!-- Social Login Buttons -->
					<div class="space-y-2">
						<Button
							variant="outline"
							class="w-full"
							onclick={handleGoogleLogin}
							disabled={isLoading}
						>
							<svg
								class="mr-2 h-4 w-4"
								viewBox="0 0 48 48"
								fill="none"
								xmlns="http://www.w3.org/2000/svg"
							>
								<path
									fill="#FFC107"
									d="M43.611,20.083H42V20H24v8h11.303c-1.649,4.657-6.08,8-11.303,8c-6.627,0-12-5.373-12-12c0-6.627,5.373-12,12-12c3.059,0,5.842,1.154,7.961,3.039l5.657-5.657C34.046,6.053,29.268,4,24,4C12.955,4,4,12.955,4,24c0,11.045,8.955,20,20,20c11.045,0,20-8.955,20-20C44,22.659,43.862,21.35,43.611,20.083z"
								/>
								<path
									fill="#FF3D00"
									d="M6.306,14.691l6.571,4.819C14.655,15.108,18.961,12,24,12c3.059,0,5.842,1.154,7.961,3.039l5.657-5.657C34.046,6.053,29.268,4,24,4C16.318,4,9.656,8.337,6.306,14.691z"
								/>
								<path
									fill="#4CAF50"
									d="M24,44c5.166,0,9.86-1.977,13.409-5.192l-6.19-5.238C29.211,35.091,26.715,36,24,36c-5.202,0-9.619-3.317-11.283-7.946l-6.522,5.025C9.505,39.556,16.227,44,24,44z"
								/>
								<path
									fill="#1976D2"
									d="M43.611,20.083H42V20H24v8h11.303c-0.792,2.237-2.231,4.166-4.087,5.571c0.001-0.001,0.002-0.001,0.003-0.002l6.19,5.238C36.971,39.205,44,34,44,24C44,22.659,43.862,21.35,43.611,20.083z"
								/>
							</svg>
							{mode === 'login' ? 'Sign in with Google' : 'Sign up with Google'}
						</Button>
					</div>
				{/if}
			{/if}
		</Card.Content>

		<Card.Footer class="flex justify-center">
			{#if isAlreadyLoggedIn}
				<!-- No footer links needed when already logged in -->
			{:else if mode === 'login'}
				<p class="text-sm text-gray-600">
					Don't have an account?
					<button
						class="ml-1 font-medium text-blue-600 hover:underline"
						onclick={() => switchMode('signup')}
					>
						Sign up
					</button>
				</p>
			{:else if mode === 'signup'}
				<p class="text-sm text-gray-600">
					Already have an account?
					<button
						class="ml-1 font-medium text-blue-600 hover:underline"
						onclick={() => switchMode('login')}
					>
						Sign in
					</button>
				</p>
			{:else}
				<p class="text-sm text-gray-600">
					Remember your password?
					<button
						class="ml-1 font-medium text-blue-600 hover:underline"
						onclick={() => switchMode('login')}
					>
						Sign in
					</button>
				</p>
			{/if}
		</Card.Footer>
	</Card.Root>
</div>
