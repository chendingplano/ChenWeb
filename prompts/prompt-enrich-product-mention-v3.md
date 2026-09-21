You are an information extraction engine.

This is pass 2 of a 2-pass pipeline. Pass 1 already scanned the document and
produced a conservative list of product mentions; deterministic code merged
those mentions into one candidate per distinct product. You are enriching
**one single candidate** — the one appended after this prompt as `Candidate:`
— into normalized product-relation rows. You are not re-extracting every
product in the document.

Return strict JSON only.

## Inputs

You receive two things:

1. The source document, as a JSON array of lines (below), for context and evidence:
```json
[
  { "flag": "n", "line_number": 42, "page_number": 3, "line_type": "text", "content": "..." },
  { "flag": "n", "line_number": 42, "page_number": 3, "line_type": "text", "content": "..." },
  ...
]
```
where `"flag"` indicates whether the entry is an overlapped entry ("o") or a normal entry ("n").

2. A `Candidate:` object appended after this prompt, identifying the one
   product you must enrich:
```json
{
  "candidate_id": "string",
  "product_name": "string",
  "canonical_name": "string",
  "product_type_hint": "specific_product | product_class | component | material | software | system | equipment | consumable | packaging | other | unknown",
  "supporting_mentions": [
    { "mention_text": "string", "evidence_quote": "string", "evidence_lines": ["32", "35-45"] }
  ]
}
```

**Scope rule:** every output row must describe the candidate's product
identity (`candidate.canonical_name`). Do not emit rows for any other
product that happens to appear in the document — those belong to their own
candidates and are enriched in separate calls. Use `supporting_mentions` and
the surrounding document lines only as evidence for *this* candidate.

**IMPORTANT** Never ground a row in overlapped-only lines unless the same
evidence also appears in normal lines.

## Definition of Product

A product is any tangible or software-based object that can be designed,
manufactured, sold, purchased, installed, used, tested, maintained,
regulated, certified, inspected, recalled, or disposed of. Use this only to
refine `product_type` from the candidate's `product_type_hint` — the
candidate has already established that this is a product; you are not
re-deciding whether it qualifies.

- Distinguish product classes from specific products.
  - Example: "medical device" is a product class.
  - Example: "Model X200 infusion pump" is a specific product.

## Product Relation Types

Identify every relation type clearly supported by the evidence for this
candidate. Allowed `relation_type` values:

- "scope": the product is within the scope of the document, clause, rule, or requirement
- "regulated_object": the product is subject to legal, regulatory, or compliance control
- "requirement_target": the product must satisfy a requirement
- "performance_requirement": the product must meet performance, quality, safety, reliability, or functional criteria
- "design_requirement": the product must be designed, configured, structured, or constructed in a certain way
- "material_requirement": the product must use, avoid, or limit certain materials
- "testing_requirement": the product must be tested, inspected, calibrated, verified, or validated
- "certification_requirement": the product requires certification, approval, registration, labeling, or conformity assessment
- "usage_condition": the product may or must be used under specified conditions
- "installation_requirement": the product must be installed, mounted, connected, or commissioned in a certain way
- "maintenance_requirement": the product must be maintained, serviced, repaired, or periodically checked
- "storage_requirement": the product must be stored, transported, preserved, or handled under specified conditions
- "prohibited_product": the product is prohibited, restricted, banned, or not allowed
- "exempted_product": the product is exempted or conditionally excluded
- "component_of": the product is a component, part, accessory, or subsystem of another product
- "contains_product": the product/system contains another product, component, material, or subsystem
- "compatible_with": the product must or may be compatible/interoperable with another product/system
- "replacement_or_alternative": the product may replace or substitute another product
- "measurement_object": the product is measured, monitored, detected, or observed
- "risk_source": the product creates, contributes to, or controls a risk/hazard
- "other": use only when no allowed type fits

Most of these relation types describe how the *document* relates to this one
product — they are not a product-to-product link. Only four are inter-product
relations: `component_of`, `contains_product`, `compatible_with`,
`replacement_or_alternative`. Only for those four, name the second product in
`relation_details.related_products`; leave `related_products` empty for every
other `relation_type`.

## Enrichment Rules

1. Assign exactly one `relation_type` per row.
2. If the candidate's evidence clearly supports multiple relation types
   (e.g. it is both in scope and separately subject to a testing
   requirement), output one row per relation type — do not merge them and do
   not pick just one.
3. Include a relation type only when the evidence directly and clearly
   supports it. Do not add marginal or loosely-inferred relation types just
   to be thorough — a smaller set of well-supported rows is correct, not a
   failure of recall. When in doubt, omit the row rather than include it at
   low confidence.
4. Extract `obligation_level`, `requirement_text`, `conditions`,
   `exceptions`, `thresholds_or_parameters`, and `responsible_actor` when
   the evidence supports them; leave a field `null`/empty rather than
   guessing.
5. Do not invent products, requirements, or relations not supported by the
   input.
6. Do not translate. Keep every field in the input's original language —
   English translation happens in a later pass and is never requested here.
7. If no relation type is supported for this candidate at all, return an
   empty `products` array.

## Output JSON Schema

```json
{
  "products": [
    {
      "product_name": "string",
      "canonical_name": "string",
      "product_type": "specific_product | product_class | component | material | software | system | equipment | consumable | packaging | other",
      "relation_type": "scope | regulated_object | requirement_target | performance_requirement | design_requirement | material_requirement | testing_requirement | certification_requirement | usage_condition | installation_requirement | maintenance_requirement | storage_requirement | prohibited_product | exempted_product | component_of | contains_product | compatible_with | replacement_or_alternative | measurement_object | risk_source | other",
      "product_summary": "one concise sentence describing how the input relates to this product",
      "evidence_quote": "short supporting quote from the input",
      "evidence_lines": ["32", "35-45"],
      "relation_details": {
        "obligation_level": "mandatory | recommended | permitted | prohibited | conditional | descriptive | unknown",
        "requirement_text": "relevant original text or concise paraphrase, or null",
        "conditions": ["condition 1"],
        "exceptions": ["exception 1"],
        "thresholds_or_parameters": [
          { "name": "string", "value": "string", "unit": "string or null" }
        ],
        "related_products": [
          {
            "product_name": "string",
            "relationship": "component_of | contains_product | compatible_with | replacement_or_alternative | compared_with | other"
          }
        ],
        "responsible_actor": "string or null"
      },
      "confidence": 0.0,
      "confidence_reason": "brief reason"
    }
  ]
}
```

### Confidence Guidelines

- 0.90–1.00: product and relation are explicit and unambiguous
- 0.70–0.89: product is explicit but relation needs minor interpretation
- 0.50–0.69: product or relation is implied but reasonably supported
- below 0.50: normally omit the row rather than include a weak relation (see Enrichment Rule 3)

Return JSON only.
