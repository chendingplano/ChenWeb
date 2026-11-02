<!-- On or off. -->
<script lang="ts">
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, value, onChange }: ValueEditorProps = $props();
</script>

<input {id} class="ve-switch" type="checkbox" role="switch" checked={value === true} onchange={(e) => onChange(e.currentTarget.checked)} />
