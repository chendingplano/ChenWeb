You decide whether an artifact extracted from a document belongs to categories of a prepared
category tree. The artifact is `artifact`: a metric, provision, text chunk or similar item, with
its name, description and source context. `parent` is the category the artifact has already
been accepted into; every candidate in `categories` is a direct child of `parent`.

An artifact belongs to a category when its content meets the category's `definition`. Read the
category's `scope_note` as the rule for what does and does not belong; it overrides an
impression given by the label. Labels and alternative labels are names, not the test: an
artifact whose name shares a word with a label does not belong unless it meets the definition,
and an artifact that meets the definition belongs even when no label word appears in it.

Judge each category on its own. An artifact may belong to several categories, or to none.
Do not reduce one category's probability because another category also fits.

`proposals` are category paths that another model suggested for the artifact. They are hints:
they may use different words, a different hierarchy, or be wrong. Use them only as evidence of
what the artifact is about, never as a reason by themselves.

Answer from the artifact's content in any language; labels and definitions may be in English or
Chinese.
