<script lang="ts">
	import EmojiPickerEditor from '../../src/lib/components/shared-ui/value-editors/EmojiPickerEditor.svelte';
	import { overwriteGetLocale } from '../../src/lib/paraglide/runtime';
	let value = $state<string | undefined>(localStorage.getItem('wpi-emoji-test') || undefined);
	overwriteGetLocale(() => new URLSearchParams(location.search).get('zh') ? 'zh-cn' : 'en');
</script>
<EmojiPickerEditor id="emoji" label="Emoji" {value} options={{}} check={(v) => v === '🎈' ? 'Rejected' : null} onChange={(v) => {value = typeof v === 'string' ? v : undefined; localStorage.setItem('wpi-emoji-test', value ?? '');}} />
<output data-testid="value">{value}</output>
