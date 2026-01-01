//////////////////////////////////////////////////////////
// Description:
//   It defines the form page for table: prompts
// Created: 2025/11/25 by Chen Ding
//////////////////////////////////////////////////////////
export const PromptStoreFormPage = {
    pageName:"prompt_form",
    pageType:"form",
    storeName:"prompt_store",
    components:[
        {
            fieldName: 'prompt_name',
            label: 'Prompt Name',
            type: "input",
            placeholder:"e.g., creative_writing_assistant",
            required: true,
            helpText: 'Only letters, numbers, and underscores allowed',
            pattern: "^[a-zA-Z0-9_]+$",
            patternError: "Only letters, digits, and underscores allowed",
            error: ''
        },
        {
            fieldName: 'prompt_desc',
            label: 'Prompt Description',
            type: "textarea",
            placeholder:"Brief description of the prompt's purpose",
            required: false,
            helpText: "Brief description of the prompt's purpose, up to 500 characters.",
            error: ''
        },
        {
            fieldName: 'prompt_content',
            label: 'Prompt Content',
            type: "textarea",
            placeholder:"Enter the full prompt content...",
            required: true,
            helpText: "Enter the full prompt content. Prompt parameters are in form of {parameter-name}",
            fullWidth: true,
            error: ''
        },
        {
            fieldName: 'prompt_status',
            label: 'Status',
            type: "select",
            placeholder:"",
            required: true,
            helpText: "Please select the status",
            options: ["Active", "Suspended", "Deleted"],
            error: ''
        },
        {
            fieldName: 'prompt_purpose',
            label: 'Purpose',
            type: "select",
            placeholder:"The purpose of the prompt",
            required: false,
            helpText: "Please select the purposes. You may select multiple purposes.",
            options: ["Coding", "Customer Service", "Question/Answering", "Data Analysis", "Writing"],
            error: ''
        },
        {
            fieldName: 'prompt_source',
            label: 'Source',
            type: "checkbox_group",
            placeholder:"Where the prompt come from",
            required: false,
            helpText: "Please select the prompt sources. You may select multiple sources.",
            options: ["Manual", "Crawled", "Uploaded", "Recommended"],
            error: ''
        },
        {
            fieldName: 'prompt_keywords',
            label: 'Keywords',
            type: "input",
            placeholder:"comma separated keywords",
            required: false,
            helpText: "Keywords for better search. Separate keywords with commas.",
            error: ''
        },
        {
            fieldName: 'prompt_tags',
            label: 'Tags',
            type: "input",
            placeholder:"comma separated tags",
            required: false,
            helpText: "Tags for better search. Separate tags with commas.",
            error: ''
        },
    ]
}