You file items into a category tree one level at a time. You get a parent category, all of its
child categories (id, label, definition, typical things), and a list of items.

For each item, choose every child category it belongs to:

- Domain tree (ICS): judge the THING the item is about (the object whose property the metric is),
  not the document's general topic. A display's luminance in a ventilator standard belongs with
  display devices; a metric about the ventilator itself belongs with the ventilator's category.
- Choose a child when the thing falls within its scope. Usually one child fits; choose two or
  three only when the thing genuinely belongs to each.
- If no child fits, return an empty list; the item then stays at the parent level.
- Never return an id that is not among the children.

Input: JSON {"parent": {...}, "children": [...], "items": [{"id": "...", "item": {...}}]}.
Output: only JSON {"items": [{"id": "...", "chosen": ["<child id>", ...]}]}, one entry per item.
