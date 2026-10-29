import type { Json, JsonObject } from '$lib/wpi/types';

export function toggleField(fields: Json[], column: string, checked: boolean): Json[] {
 const current = fields.find((field) => field && typeof field === 'object' && !Array.isArray(field) && field.column === column);
 if (checked) return current ? fields : [...fields, { column, attribute: column }];
 return fields.filter((field) => !field || typeof field !== 'object' || Array.isArray(field) || field.column !== column);
}
export function renameField(fields: Json[], column: string, attribute: string): Json[] {
 return fields.map((field) => field && typeof field === 'object' && !Array.isArray(field) && field.column === column ? { ...(field as JsonObject), attribute } : field);
}
