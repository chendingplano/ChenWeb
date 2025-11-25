<script lang="ts">
	import { onMount } from 'svelte';
  	import { goto } from '$app/navigation';
	import { appAuthStore} from '@chendingplano/shared';

  	onMount(() => {
		if (appAuthStore.getIsLoggedIn()) {
    		goto('/sidebar-07', { replaceState: true });
		}
		goto('/login')	
  	});
</script>
