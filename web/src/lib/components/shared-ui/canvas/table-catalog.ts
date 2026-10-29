export type CatalogTable = { schema: string; table: string };

export function catalogSchemas(tables: CatalogTable[]): string[] {
 return [...new Set(tables.map((item) => item.schema))].sort();
}
export function catalogTables(tables: CatalogTable[], schema: string): string[] {
 return tables.filter((item) => item.schema === schema).map((item) => item.table).sort();
}
export function schemaForTable(value: string, tables: CatalogTable[]): string {
 const match = tables.find((item) => `${item.schema}.${item.table}` === value);
 if (match) return match.schema;
 const dot = value.indexOf('.');
 return dot > 0 ? value.slice(0, dot) : value ? 'public' : '';
}
