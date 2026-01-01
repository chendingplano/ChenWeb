export const FormItemMap: Record<string, () => Promise<any>> = {
    select:             () => import("$lib/form/FormItemSelect.svelte"),
    select_multi:       () => import("$lib/form/FormItemSelectMulti.svelte"),
    input:              () => import("$lib/form/FormItemInput.svelte"),
    textarea:           () => import("$lib/form/FormItemTextArea.svelte"),
    checkbox_group:     () => import("$lib/form/FormItemCheckboxGroup.svelte"),
    date_time_picker:   () => import("$lib/form/FormItemDateTimePicker.svelte"),
};