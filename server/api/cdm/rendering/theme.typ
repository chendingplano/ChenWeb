// Default CDM Phase 1 theme (spec §5.3/§5.4 fallback template).
//
// All styling lives here, not in the canonical document (spec §5.5): a
// mandatory formatting standard is a template like this one, never a change
// to the AST. #callout and #definition receive only semantic values (role,
// title, term, body) and decide the visual treatment themselves.

#let callout-colors = (
  note: rgb("#e8f0ff"),
  tip: rgb("#e6f7ee"),
  important: rgb("#fff4e5"),
  warning: rgb("#fff4e5"),
  caution: rgb("#fdeaea"),
)

#let callout(role, title, body) = block(
  width: 100%,
  inset: 12pt,
  radius: 4pt,
  fill: callout-colors.at(role, default: callout-colors.note),
  stroke: 0.8pt,
  [
    *#title*

    #body
  ],
)

#let definition(term, body) = block(
  width: 100%,
  inset: 12pt,
  radius: 4pt,
  fill: rgb("#f5f5f5"),
  stroke: 0.8pt,
  [
    *Definition: #term*

    #body
  ],
)
