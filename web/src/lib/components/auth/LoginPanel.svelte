<script lang="ts">
	import { onMount } from 'svelte';
	import LoginEmailGoogle from './LoginEmailGoogle.svelte';
	import LoginCellPhoneOnly from './LoginCellPhoneOnly.svelte';

	// The single entry point for every login surface (/login route, the /semos
	// hero panel, the /semos modal). It picks the actual screen from
	// [system].login_page (served in GET /api/config as `login_page`) and
	// forwards `variant` / `onClose` unchanged. Add a new screen by adding a
	// case here plus a sibling component — callers never change.
	let {
		variant = 'page',
		onClose
	}: { variant?: 'page' | 'modal'; onClose?: () => void } = $props();

	type LoginPage = 'login-email-google' | 'login-cell-phone-only';
	const KNOWN: LoginPage[] = ['login-email-google', 'login-cell-phone-only'];
	const DEFAULT_PAGE: LoginPage = 'login-email-google';

	// null until /api/config resolves; the fetch is a local proxied call so the
	// blank frame is imperceptible. Any failure or unknown value falls back to
	// the default screen, mirroring the backend's GetLoginPage().
	let page = $state<LoginPage | null>(null);

	onMount(async () => {
		try {
			const res = await fetch('/api/config');
			if (res.ok) {
				const data = await res.json();
				page = KNOWN.includes(data.login_page) ? data.login_page : DEFAULT_PAGE;
			} else {
				page = DEFAULT_PAGE;
			}
		} catch (err) {
			console.error('Failed to load config:', err);
			page = DEFAULT_PAGE;
		}
	});
</script>

{#if page === 'login-cell-phone-only'}
	<LoginCellPhoneOnly {variant} {onClose} />
{:else if page}
	<LoginEmailGoogle {variant} {onClose} />
{/if}
