<!-- A value that is shown but not edited. -->
<script lang="ts">
	import { jsonText } from './values';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, value }: ValueEditorProps = $props();
</script>

<textarea {id} class="ve-input" rows="1" readonly value={typeof value === 'string' ? value : jsonText(value)}></textarea>
