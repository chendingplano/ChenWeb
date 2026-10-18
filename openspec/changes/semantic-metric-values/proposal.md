## Why

Mandatory splitting can change alternatives or required collections into unrelated assertions. Retained multi-value text can also be reduced to its first number by scalar normalization.

## What Changes

- Split only independently interpretable assertions with their source conditions and frame.
- Preserve alternatives, required point collections, composite values and unresolved expressions in an additive `value_structure` JSON field.
- Replace unconditional split recovery with evidence-backed split/keep/unresolved decisions.
- Guard numeric normalization against partial parsing; unsupported structures remain unparsed.
- Align benchmark rules and scoring with meaning-preserving granularity.

## Capabilities

### New Capabilities
- `metric-value-structure`: Persistence and consumer handling of structured non-scalar values.

### Modified Capabilities
- `metric-row-granularity`: Independent assertions, rather than numeric tokens, determine row boundaries.
- `metric-value-notation`: Semantic review replaces mandatory splitting of combined values.

## Impact

ChenWeb extraction prompts, recovery, merge identity, metric persistence/API, ontology normalization and scoring; project goose migration for production and benchmark tables; versioned benchmark and scoring skills in the separate `.agents` repository. Existing data is not automatically reprocessed or rewritten.
