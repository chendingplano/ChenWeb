import type { Component } from "svelte";

export type ComponentDef = {
    fieldName:      string;
    type:           string;
    label:          string;
    required:       boolean;
    helpText:       string;
    placeholder:    string;
    error:          string;
    rows?:          number;
    Comp?:          Component;
} & Record<string, unknown>;

export type PageDef = {
    pageName:       string;
    pageType:       string;
    components:     ComponentDef[];
}

class AosPageMgr {
    constructor() {} 

    getPage(page_name: string): PageDef | null{
        return null
    }
}
