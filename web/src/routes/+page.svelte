<script lang="ts">
	import { onMount } from 'svelte';
  	import { goto } from '$app/navigation';
	import { appAuthStore} from '@chendingplano/shared';

  	onMount(async () => {
		await appAuthStore.ready();
		if (appAuthStore.getIsLoggedIn()) {
			const norm_route = import.meta.env.VITE_DEFAULT_NORM_ROUTE;
			const admin_route = import.meta.env.VITE_DEFAULT_ADMIN_ROUTE;
			const destination = appAuthStore.getIsAdmin() ? admin_route : norm_route;
			goto(destination, { replaceState: true });
		} else {
			goto('/login', { replaceState: true });
		}
  	});
</script>
