export function newPageMetadata(name: string, description: string, keywords: string, tags: string, id: string) {
 const items = (value: string) => value.split(',').map((item) => item.trim()).filter(Boolean);
 return { descriptor_id: `page-${id}`, page_name: name.trim(), description: description.trim(), keywords: items(keywords), page_tags: items(tags) };
}
