<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { appAuthStore } from '@chendingplano/shared';

	let error = $state('');
	let loading = $state(true);

	/**
	 * Validates that a return URL is safe for redirection.
	 * Only allows relative paths starting with '/'.
	 */
	function getSafeReturnUrl(url: string | null): string | null {
		if (!url) return null;

		// Must start with / (relative path)
		if (!url.startsWith('/')) return null;

		// Reject protocol-relative URLs (//evil.com)
		if (url.startsWith('//')) return null;

		// Reject dangerous schemes embedded in path
		const lower = url.toLowerCase();
		if (lower.includes('javascript:') || lower.includes('data:')) return null;

		// Reject absolute URLs embedded in path
		if (url.includes('://')) return null;

		return url;
	}

	onMount(async () => {
		try {
			loading = true;

			// Wait for the auth store to finish its initial session check (/auth/me).
			// For Kratos OIDC flow, the session cookie is already set by Kratos.
			await appAuthStore.ready();

			const returnUrl = getSafeReturnUrl(page.url.searchParams.get('returnUrl'));
			const norm_route = import.meta.env.VITE_DEFAULT_NORM_ROUTE;
			const admin_route = import.meta.env.VITE_DEFAULT_ADMIN_ROUTE;
			if (typeof norm_route !== 'string' || norm_route.length <= 0) {
				alert('missing VITE_DEFAULT_NORM_ROUTE (CWB_0211130500)');
			}

			if (typeof admin_route !== 'string' || admin_route.length <= 0) {
				alert('missing VITE_DEFAULT_ADMIN_ROUTE (CWB_0211130501)');
			}

			// Check if we have a valid session (Kratos sets cookies during OIDC flow)
			if (appAuthStore.getIsLoggedIn() && appAuthStore.getUser()) {
				// Determine redirect based on admin status
				const destination = appAuthStore.getIsAdmin() ? admin_route : norm_route;
				const finalRedirect = returnUrl || destination;
				console.log('[OAuth Callback] Authenticated, redirecting to:', finalRedirect);
				await goto(finalRedirect, { replaceState: true });
				return;
			}

			// No session found - this shouldn't happen if Kratos OIDC succeeded
			// Try refreshing the session one more time
			console.log('[OAuth Callback] No session found, attempting refresh...');
			await appAuthStore.checkAuthStatus();
			if (appAuthStore.getIsLoggedIn() && appAuthStore.getUser()) {
				const destination = appAuthStore.getIsAdmin() ? admin_route : norm_route;
				const finalRedirect = returnUrl || destination;
				console.log('[OAuth Callback] Session found after refresh, redirecting to:', finalRedirect);
				await goto(finalRedirect, { replaceState: true });
				return;
			}

			// Still no session - redirect to login with error
			console.error('[OAuth Callback] No valid session after OAuth');
			await goto('/login?error=oauth_failed', { replaceState: true });
		} catch (err) {
			console.error('OAuth callback error:', err);
			error = err instanceof Error ? err.message : 'Authentication failed';
			loading = false;
		}
	});
</script>

{#if loading}
	<div class="flex min-h-screen items-center justify-center">
		<div class="text-center">
			<div
				class="mb-4 inline-block h-8 w-8 animate-spin rounded-full border-4 border-solid border-current border-r-transparent motion-reduce:animate-[spin_1.5s_linear_infinite]"
			></div>
			<p class="text-lg">Completing sign in...</p>
		</div>
	</div>
{:else if error}
	<div class="flex min-h-screen items-center justify-center">
		<div class="rounded-lg border border-red-200 bg-red-50 p-6 text-center">
			<h2 class="mb-2 text-xl font-semibold text-red-800">Authentication Error</h2>
			<p class="mb-4 text-red-600">{error}</p>
			<a href="/login" class="text-blue-600 underline hover:text-blue-800">Return to login</a>
		</div>
	</div>
{/if}
