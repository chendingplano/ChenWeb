import type { Component } from "svelte";

export type FormItem<T> = {
    id:         string;
    key:        string;     // corresponds to schema key
    type:       string;     // "input" | "select" | "number" | ...
    label:      string;
    required:   boolean;
    helpText?:  string;
    options?:   T[];
    value:      T;
    error?:     string;
} & Record<string, unknown>;

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
    page_name:      string;
    page_type:      string;
    components:     ComponentDef[];
}