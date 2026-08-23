## Context

`governed-class-signature-resolution` (merged same day, not yet archived — this design deltas against
that change's `specs/class-identity-signature/spec.md`) left `matchClassBySignature` permanently
inert. Prior revisions of this proposal settled: `metric_name`/`subject` resolve via
`names.Resolver`/`KeywordFamily` (already built, tested, running in production; `scope` gives
per-field isolation with zero new tables); `object_name` is excluded (separate `kb.object_nodes`
identity system); `range_type` reuses its existing, working `kb.metric_value_range_type_map` /
`ValueRangeTypeMapper` (ADR `2026081401`) rather than inventing a new mechanism.

This revision's contribution (`notes.md`): a named taxonomy for "how a field normalizes," and a
data-driven correction to `value_class`'s assignment.

### The four methods, mapped onto spec `2026080403`'s tier ladder

- **system** — a field-specific, hand-built mechanism, not a general tier-ladder application. Not
  "no normalization" — it's "this field's normalization doesn't fit the generic spectrum below, so it
  gets its own code." `range_type` (existing curated bucket map) and `value_type` (new curated bucket
  map, this change) both belong here. `object_name` belongs here *conceptually* but has no
  implementation yet (see D4).
- **simple** — spec `2026080403` §6.1's tier-1 preparation pipeline only (NFKC, zero-width strip,
  dash/quote normalization, whitespace collapse, case-fold, possessive/article stripping) —
  `semid.Normalizer.Normalize(...).Norm`. Deterministic, no table, no DB, no score.
- **moderate** — tiers 0-3 (exact surface match, norm-key match, alternate-key match, rewrite-rule
  match) — table-backed, deterministic scores (1.0 for tiers 0/1/3, 0.8 for tier 2 — verified directly
  against `keywordfamily.go`'s tier comments), no fuzzy matching, no initials bridge.
- **strong** — the full tiers 0-5 ladder (adds tier 4 initials-bridge and tier 5 trigram-blocked
  fuzzy match), auto-accept at `MinScore: 0.8` with `MaxCandidates: 1` (verified:
  `keywordfamily.go:89`, `semid/adjudicate.go`'s `Adjudicate`) — what `names.Resolver.ResolveAndObserve`
  already runs, unmodified.

### Why `moderate` isn't built in this change

`KeywordFamily.CandidateNodes` (`keywordfamily.go:108`) is not parameterized by a stopping tier — it's
a hardcoded sequence (tier0 → tier1 → tier2 → tier3 → tier4 → tier5, exit at first hit). `strong` costs
nothing extra because it's exactly this existing function, called as-is. `moderate` would require
adding a genuinely new capability (e.g. a `MaxTier` field checked before tiers 4/5 run) to shared,
tested, production code every other `KeywordFamily` consumer in the app also depends on. Confirmed
with the user: since no field in this proposal needs tier-0-3-only matching, building that capability
now would be speculative configurability this codebase's own guidelines (`ChenWeb/CLAUDE.md` §1.2)
warn against. `moderate` is a real, valid config value — config load must recognize it as a *known*
method — but rejects it with a clear error until the capability actually exists.

### `value_class` correction: data doesn't support a curated map

Queried `kb.metrics` directly rather than assuming:

```
value_class:      requirement(46), reference(8), definition(2)                       -- 3 clean values
value_data_type:   numeric(13), integer(8), text(8), standard_reference(6), float(5),
                    string(4), qualitative(2), table_reference(2), percentage(2),
                    number(2), ratio(1)                                                -- 12 values, real synonym clusters
```

`value_class` has no synonym problem to solve — 3 distinct, already-clean values. A curated
resolve-or-propose map (this change's earlier draft) is pure overhead. `value_data_type` genuinely has
clusters a curated map is the right tool for (`numeric`/`integer`/`float`/`number`/`ratio` should
likely canonicalize together; `text`/`string`/`qualitative`/`standard_reference`/`table_reference`
likely another way) — it keeps the bucket-map design from the prior revision. Both corrections are
narrow: only `value_class`'s assigned *method* changes (bucket-map → simple); nothing about
`value_type`/`range_type`/`metric_name`/`subject`/`object_name` changes from the prior revision.

## Goals / Non-Goals

**Goals:**
- Give the `normalize` config value a name that says *how* a field resolves, not just *whether* it
  does — self-documenting config, matching what the field's actual resolution code does.
- Keep `BuildConfiguredProperties` mechanism-agnostic regardless of how many methods exist.
- Assign each field's method based on what its actual data needs (verified), not by assumption.
- Fail loudly, not silently, on `moderate` or any unrecognized `normalize` value.

**Non-Goals:**
- Implementing `moderate` (tier-0-3 capping in `KeywordFamily`) — deferred until a real need exists.
- `object_name`'s actual normalization — separate, future change scoped to `kb.object_nodes`.
- Touching `kb.governed_property_value_map`/`GovernedPropertyResolver`, `kb.metric_value_range_type_map`,
  or `metric_range_type_errors_handler.go` — all unchanged.
- `search_document`/ADR `2026082102` (DR6) — still unbuilt.

## Decisions

### D1 — `Normalize` is a string, validated at config load against a fixed set

```go
type OntologyTermPropertyMapEntry struct {
    Field     string `mapstructure:"field"`
    Property  string `mapstructure:"property"`
    Identity  bool   `mapstructure:"identity"`
    Normalize string `mapstructure:"normalize"` // "", "system", "simple", "moderate", "strong"
}
```

`LoadConfig` validates every entry's `Normalize` against `{"", "system", "simple", "moderate",
"strong"}` and errors (not warns — this is a real misconfiguration, not an edge case worth merely
flagging) on anything else, **including `"moderate"`** — recognized as a real method name, rejected
because no code implements it yet, with an error message that says so explicitly (distinguishing "you
typo'd this" from "you asked for something not built yet").

**Rejected: a bare bool plus a separate enum-ish `Method` field.** Two fields for one concept is
unnecessary indirection — a field either doesn't normalize (`""`) or normalizes one specific way
(one of the four names); there's no case where a field needs "normalize=true" independent of which
method.

### D2 — `BuildConfiguredProperties` only ever checks `Normalize != ""`

The builder does not switch on `"system"`/`"simple"`/`"moderate"`/`"strong"` — it wraps a field
whenever `Normalize` is non-empty, using whatever value is in the `resolved` map for that field's
name. *Which* method populates `resolved[field]` is entirely the caller's concern (`metric_normalizer.go`'s
`Normalize` loop, `extract-metrics.go`'s `resolveAll`) — this keeps the one shared function from ever
needing to know about `semid.Normalizer`, `names.Resolver`, or any bucket map, and means adding a fifth
method later touches zero code in `property_map.go`.

### D3 — Per-field resolution mechanism (restating prior decisions plus this round's correction)

- `metric_name`/`subject` — **strong**, via `names.Resolver.ResolveAndObserve`, scopes `"_"` (existing)
  and `"metric_subject"` (new).
- `value_class` — **simple**, via `semid.Normalizer{Version: semid.CurrentNormalizerVersion}.Normalize(r.ValueClass.String).Norm`,
  computed inline in `metric_normalizer.go`, no DB.
- `value_type` — **system**, via the new `ValueBucketMapper.Lookup(ctx, "value_type", r.ValueDataType.String, inputRecordID)`.
- `range_type` — **system**, via the existing `ValueRangeTypeMapper.Lookup` call already in
  `metric_normalizer.go` — its `canonicalBucket` result is reused, not recomputed.
- `object_name` — no mechanism; `normalize` left unset in config.

### D4 — `object_name`'s config entry stays `normalize` unset, not `"system"`, until a real mechanism exists

Setting `normalize = "system"` in config today, with no `resolved["object_name"]` ever populated,
would read as "this is handled" when it isn't — indistinguishable, to a future reader of
`config.local.toml`, from a working `"system"` field like `range_type`. Leaving it unset is the
honest state: `identity = true` (participates in signature matching, contributes nothing since it's
never resolved) with no `normalize` at all. A future change that actually builds `object_name`'s
mechanism (scoped to `kb.object_nodes`) is the one that should flip this to `"system"`.

## Risks / Trade-offs

- **[Risk] `"moderate"` sitting in the recognized-values set but permanently erroring until built is
  a config foot-gun** — someone reads the taxonomy, reasonably tries `normalize = "moderate"`, and
  gets a config-load failure. **Mitigation:** the error message names exactly why (not implemented,
  not a typo) and points at this ADR; this is deliberately preferred over silently treating
  `"moderate"` as `"strong"` or `"simple"`, which would misrepresent what actually ran.
- **[Trade-off] `value_class` gets no growth path if its vocabulary later becomes messier** (e.g. a
  future extractor emits `"Requirement"` vs `"requirement_spec"` as genuinely different intents).
  **Mitigation:** none needed now — the data says 3 clean values today; if that changes, upgrading
  `value_class` from `simple` to `system` (adding it as a second dimension to
  `kb.metric_value_bucket_map`) is a small, additive follow-up, not a redesign.
- Unchanged from prior revisions: `value_type` and `object_name`'s underlying risks/trade-offs (catalog
  growth without a curation UI; `object_name` staying unresolved through this whole ADR series).

## Migration Plan

1. Add `kb.metrics.subject_concept_id` and `kb.metric_value_bucket_map` (unchanged from prior
   revision, migration content unaffected by this round's changes).
2. Land the config schema (`Normalize` as validated string), collapsed `BuildConfiguredProperties`,
   the `extract-metrics.go` subject call, `ValueBucketMapper`, and the `resolved`-map construction
   (now includes a `semid.Normalizer` call for `value_class` instead of a bucket-map lookup) together.
3. Rewrite `config.local.toml`'s both property-map sections with the method names from D3.
4. Sequence this change's archive after (or together with) `governed-class-signature-resolution`'s own
   archive.

## Open Questions

None blocking. `object_name`/`kb.object_nodes` normalization, `moderate`'s eventual implementation, and
admin UIs for either bucket-map table remain explicitly deferred.
