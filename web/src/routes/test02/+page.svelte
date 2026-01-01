<script lang="ts">
    import { string } from 'zod/v4';
    import {TestSuperForm} from './index.js';
    let formData = $state<Record<string, any>>({});
</script>

<main>
    <TestSuperForm data={formData}/>
</main>
