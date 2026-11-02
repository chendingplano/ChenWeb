<script lang="ts">
	import './full.css';
	import WpiEditor from '../../src/lib/components/shared-ui/wpi-editor.svelte';
	import { overwriteGetLocale } from '../../src/lib/paraglide/runtime';
	overwriteGetLocale(() => 'en');
</script>
<WpiEditor railOffset={0} onClose={() => {}} />
