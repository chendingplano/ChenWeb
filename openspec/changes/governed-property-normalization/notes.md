
## 1. Types of normalization
we can't simply say 'normalize this', 'normalize that'. Depending on the things to 
be normalized, the normalization can be quite different. 

I summarize the normalization methods as follows

### 1.1 system
This is kind of hard-coded normalization. Currently, `object_name` falls to this category.
Currently, `object_name` is not fully normalized yet. This will be solved in another proposal.

### 1.2 simple
Normalize the inputs by the method `Tier 1` defined in Section §6.1 of 2026080403-spec,
which is copied below. No table lookup. Purely string modifications. No scores.

| # | Step | Verified example | Note |
|---|---|---|---|
| 1 | Unicode NFKC | `Ｌｕｍｉｎａｎｃｅ` → `luminance` | full-width folded |
| 2 | Strip zero-width / BOM / LTR-RTL | `显示​亮度` → `显示亮度` | soft hyphen U+00AD **not** stripped |
| 3 | Dashes → ASCII | `e–mail` → `e-mail` | |
| 4 | Quotes → ASCII | | |
| 5 | Collapse/trim whitespace | `␠␠LUMINANCE␠␠` → `luminance` | |
| 6 | Collapse dotted initialisms | `U.S.A.` → `usa` | uppercase `A.B.C` only |
| 7 | Case-fold | `亮度` → `亮度` (CJK unaffected) | ⚠️ `unicode.ToLower`, i.e. lowercasing, **not** full Unicode case folding, despite the comment |
| 8 | Drop possessive `'s` | **`AWS's` → `aws'`** | word-final possessive missed (N2) |
| 9 | Strip leading articles | `the cloud` → `cloud` | English, **unguarded by language** |
| 10 | Singularization | **`AIDS` → `aid`**, **`SaaS` → `saa`** | reproduces the failures it was written to avoid (N1) |

Note `显示 亮度` (with a space) → `norm_key` = `显示 亮度`, which does **not** equal `显示亮度`. The `alnum` key bridges them at tier 2 (K1 fixed, §19 step 5).

### 1.3 moderate
It uses Tier-0, Tier-1, Tier2 and Tier-3 defined in 2026080403-spec. It requires
table lookups, with scores: 1.0 (with no Tier-2) or 0.8 (from Tier-2).

### 1.4 strong
It runs the tiers up to Tier 5:
If Tier 5 is not run:
- If the normalization returns `ambiguous`, pick the first candidate, flag it as `ambiguous`
- If the score is no less than 0.80, accept it.

If the normalization reaches Tier 5:

- Input length 4 or fewer: Tier 5 is disabled.
- Length 5–8: at most one edit, and the first character must match.
- Length 9 or more: at most two edits and a score of at least **0.88**.
- Safety checks can reject a candidate regardless of score, such as differing digits.

The normalization:
- Auto-accepts the highest-scoring candidate if its score is at least **0.80** and it is the unique highest score.
- Returns `ambiguous` if multiple candidates tie for the highest score.
- Applies Tier 5 filtering as described above
- Return the score and flag it as 'not-normalized' if the best score is below 0.80.

In practice, every candidate surviving Tier 5 already scores at least 0.80. Therefore, 
the normalization is primarily deciding whether there is a single clear winner, 
not applying an additional meaningful score filter.