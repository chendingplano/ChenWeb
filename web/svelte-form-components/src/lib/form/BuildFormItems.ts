import { ZodObject, ZodNumber, ZodString, ZodEnum } from "zod";
import type { FormItem } from "./FormTypes";

export function buildFormItems<T extends ZodObject<any>>(schema: T) {
    const shape = schema.shape;
    const items: FormItem<any>[] = [];

    for (const key in shape) {
        const field = shape[key];

        // Detect field type → assign component + metadata
        if (field instanceof ZodEnum) {
            items.push({
                id: key,
                key,
                type: "select",
                label: key,
                required: true,
                helpText: "",
                options: field.options,
                value: "",
                error: ""
            });
        }
        else if (field instanceof ZodString) {
            items.push({
                id: key,
                key,
                type: "input",
                label: key,
                required: true,
                helpText: "",
                value: "",
                error: ""
            });
        }
        else if (field instanceof ZodNumber) {
            items.push({
                id: key,
                key,
                type: "number",
                label: key,
                required: true,
                helpText: "",
                value: 0,
                error: ""
            });
        }
        else {
            console.warn(`Unsupported field type: ${key}`);
        }
    }

    return items;
}
