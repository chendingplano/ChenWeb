import { z } from "zod";

// schema.ts
export const UserFormSchema = {
    country: {
        type: "select",
        label: "Country",
        required: true,
        helpText: "Please select where you currently reside.",
        options: ["USA", "Canada", "UK", "Germany", "Japan"],
        default: ""
    },

    name: {
        type: "string",
        label: "Name",
        required: true,
        helpText: "Please enter your name.",
        default: ""
    },

    age: {
        type: "number",
        label: "Age",
        required: false,
        helpText: "Your age in years.",
        default: 18
    }
} as const;

