// The descriptor ID of a new page or app: the slug of its name (lowercase letters, digits and
// single hyphens, at most 64 characters). A name with no Latin letter or digit has none.
export function appSlug(name: string): string {
 return name.normalize('NFKD').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 64).replace(/-+$/, '');
}

// The identity of a new page: its descriptor ID (the slug of its name) and the details
// authors search when looking for a page to reuse. An app is a separate descriptor that
// names a page by this ID.
export function newPageMetadata(name: string, description: string, keywords: string, tags: string, size: { width: string; height: string }) {
 const items = (value: string) => value.split(',').map((item) => item.trim()).filter(Boolean);
 return {
  descriptor_id: appSlug(name), page_name: name.trim(),
  ...(description.trim() ? { description: description.trim() } : {}),
  keywords: items(keywords), page_tags: items(tags), width: size.width, height: size.height
 };
}
