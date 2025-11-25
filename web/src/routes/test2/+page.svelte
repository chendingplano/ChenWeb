<script lang="ts">
    import { buildFormItems } from "$lib/form/BuildFormItems";
    import { UserFormSchema } from "$lib/schemas/userForm";
    import { GetAllKeys } from '@chendingplano/shared';

    /*
    // 1. Auto-generate form items from Zod schema
    let userFormData = $state(UserFormSchema);

    // 2. Refs to children
    let refs = $state<Record<string, any>>({});

    // 3. On submit validate using Zod
    function handleSubmit() {
        const data: any = {};

        let valid = true;
        const keys = GetAllKeys(userFormData) as string[]
        for (const [key, value] of Object.entries(userFormData)) {
            if (value.checkValue) {
                const ok = ref.checkValue();
                if (!ok) valid = false;
            }
        }
        if (!valid) return alert("Validation failed");

        // collect values
        for (const item of items) {
            data[item.key] = item.value;
        }

        // validate with Zod
        const parsed = UserFormSchema.safeParse(data);

        if (!parsed.success) {
            console.log(parsed.error.format());
            alert("Schema validation error!");
            return;
        }

        alert(`Form OK: ${JSON.stringify(parsed.data, null, 2)}`);
    }
    */
</script>

<main>
    <h1>Auto-Generated Form</h1>

    <!--button onclick={handleSubmit}>Submit</button-->

    <!--pre>{JSON.stringify(items, null, 2)}</pre-->
</main>
