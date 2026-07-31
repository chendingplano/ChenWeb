# Store-profile report

## Provenance

- dataset_id: doc-processors-corpus-display-module
- dataset_version: 1.1.0
- content_hash: sha256:16ed1dbb544e6bdf2cafa47b308eab0b1c6e415b7544a381ea4e25906e1da86b
- case_id: display-module-v1

## Selected processors

- generate_summaries
- extract_inventory_items
- extract_provisions

## Rows

| Store profile | Document kind | Processor | Documents | Successful documents | Failed documents | Documents with output | Output rows | Not registered | Applicability | Evidence kind | Assessment |
|---|---|---|---:|---:|---:|---:|---:|---:|---|---|---|
| narrative-research | marketing-narrative | extract_inventory_items | 1 | 1 | 0 | 1 | 1 | 0 | not_required | structural_yield | informational_not_required |
| narrative-research | marketing-narrative | extract_provisions | 1 | 1 | 0 | 0 | 0 | 1 | not_required | structural_yield | informational_not_required |
| narrative-research | marketing-narrative | generate_summaries | 1 | 1 | 0 | 1 | 1 | 0 | required | structural_yield | required_output_observed |
| product-specification | enterprise-standard | extract_inventory_items | 3 | 2 | 1 | 1 | 1 | 0 | required | structural_yield | required_failure |
| product-specification | enterprise-standard | extract_provisions | 3 | 2 | 1 | 2 | 2 | 0 | useful | structural_yield | useful_review_warning |
| product-specification | enterprise-standard | generate_summaries | 3 | 2 | 1 | 2 | 2 | 0 | not_required | structural_yield | informational_not_required |
| regulated-reference | authority-standard | extract_inventory_items | 5 | 5 | 0 | 5 | 5 | 0 | not_required | structural_yield | informational_not_required |
| regulated-reference | authority-standard | extract_provisions | 5 | 5 | 0 | 5 | 5 | 0 | required | structural_yield | required_output_observed |
| regulated-reference | authority-standard | generate_summaries | 5 | 5 | 0 | 5 | 5 | 0 | not_required | structural_yield | informational_not_required |

Structural yield is not semantic correctness.
