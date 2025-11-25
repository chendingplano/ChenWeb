export type FormItemBase = {
    field_name:     string,
    value:          string,
    label:          string,
    required:       boolean,
    helpText:       string,
    error:          string
}

export type FormItemSelect = {
    options:        string[]
}
