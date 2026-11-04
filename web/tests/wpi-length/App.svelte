<script lang="ts">
 import LengthEditor from '../../src/lib/components/shared-ui/value-editors/LengthEditor.svelte';
 import type { Json } from '../../src/lib/wpi/types';
 let value = $state<Json | undefined>('12px');
</script>
<LengthEditor id="length" label="Width" {value} options={{units:['px','%'],keywords:['auto']}} onChange={(v) => value = v} />
<output>{value ?? 'unset'}</output><button>Outside</button>
