<script lang="ts">
	import FormItemSelect from '$lib/form/FormItemSelect.svelte';
	import FormItemInput from '$lib/form/FormItemInput.svelte';
	import type {ComponentDef} from '$lib/form/FormTypes';
	import AutoForm from '$lib/form/AutoForm.svelte';

	// --------------------------------
	// 1. Form item component map
	// --------------------------------
	const formItemMap: Record<string, unknown> = {
		select: FormItemSelect,
		input: FormItemInput
	};

	// --------------------------------
	// 2. Form state
	// --------------------------------
	let myFormItem = $state<ComponentDef[]>([
        {
            fieldName: 'promptName',
            label: 'Prompt Name',
            type: "input",
            placeholder:"e.g., creative_writing_assistant",
            required: true,
            helpText: 'Only letters, numbers, and underscores allowed',
            pattern: "/^[a-zA-Z0-9_]+$/",
            patternError: "Only letters, digits, and underscores allowed",
            error: ''
        },
        {
            fieldName: 'promptDesc',
            label: 'Prompt Description',
            type: "textarea",
            placeholder:"Brief description of the prompt's purpose",
            required: false,
            helpText: "Brief description of the prompt's purpose, up to 500 characters.",
            error: ''
        },
        {
            fieldName: 'promptContent',
            label: 'Prompt Content',
            type: "textarea",
            placeholder:"Enter the full prompt content...",
            required: true,
            helpText: "Enter the full prompt content. Prompt parameters are in form of {parameter-name}",
            fullWidth: true,
            error: ''
        },
        {
            fieldName: 'status',
            label: 'Status',
            type: "select",
            placeholder:"",
            required: true,
            helpText: "Please select the status",
            options: ["Active", "Suspended", "Deleted"],
            error: ''
        },
        {
            fieldName: 'PromptPurpose',
            label: 'Purpose',
            type: "select",
            placeholder:"The purpose of the prompt",
            required: false,
            helpText: "Please select the purposes. You may select multiple purposes.",
            options: ["Coding", "Customer Service", "Question/Answering", "Data Analysis", "Writing"],
            error: ''
        },
        {
            fieldName: 'Source',
            label: 'Source',
            type: "checkbox_group",
            placeholder:"Where the prompt come from",
            required: false,
            helpText: "Please select the prompt sources. You may select multiple sources.",
            options: ["User Wriate", "Crawled", "Uploaded", "Recommended"],
            error: ''
        },
        {
            fieldName: 'Keywords',
            label: 'Keywords',
            type: "input",
            placeholder:"comma separated keywords",
            required: false,
            helpText: "Keywords for better search. Separate keywords with commas.",
            error: ''
        },
        {
            fieldName: 'Tags',
            label: 'Tags',
            type: "input",
            placeholder:"comma separated tags",
            required: false,
            helpText: "Tags for better search. Separate tags with commas.",
            error: ''
        },
	]);

	// --------------------------------
	// 3. Child refs map (one per form item)
	// --------------------------------
	let formData = $state<Record<string, any>>({});

	// --------------------------------
	// 4. Submit handler
	// --------------------------------
	function handleSubmit() {
		let allValid = true;

		for (const item of myFormItem) {
			if (typeof item.fieldName === "string") {
				const ref = formData[item.fieldName];
				if (ref?.checkValue) {
					const ok = ref.checkValue();
					if (!ok) allValid = false;
				}
			}
		}

		if (allValid) {
			alert(`Form Submitted!  Country: ${formData}`);
		} else {
			console.log("Validation failed");
		}
	}
</script>

<main>
	<h1>Form Example (Dynamic Rendering)</h1>

    <AutoForm bind:schema={myFormItem}
              bind:formData={formData}/>

	<pre style="background: #f4f4f4; padding: 10px; margin-top: 20px;">
	FormData: {JSON.stringify(formData, null, 2)}
	</pre>
</main>