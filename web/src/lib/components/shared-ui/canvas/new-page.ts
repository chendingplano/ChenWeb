// The descriptor ID of a new app: the slug of its name (lowercase letters, digits and
// single hyphens, at most 64 characters). A name with no Latin letter or digit has none.
export function appSlug(name: string): string {
 return name.normalize('NFKD').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 64).replace(/-+$/, '');
}

// The identity of a new app: the page's root `descriptor_id` (the server needs it there)
// and its root attribute `app`, which holds everything else.
export function newPageMetadata(name: string, description: string, keywords: string, tags: string, size: { width: string; height: string; schema_version: string }) {
 const items = (value: string) => value.split(',').map((item) => item.trim()).filter(Boolean);
 const id = appSlug(name);
 const page_name = `${id}-page`;
 const app = {
  ...(description.trim() ? { description: description.trim() } : {}),
  descriptor_id: id, width: size.width, height: size.height, keywords: items(keywords),
  page_name, page_tags: items(tags), schema_version: size.schema_version
 };
 return { descriptor_id: id, app };
}
