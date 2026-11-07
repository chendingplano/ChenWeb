## Review task: source coverage, statement boundaries and correctness

Apply the extraction rules and compact v2 schema above. The input is now an
object with `source` (the original input serialized as text) and `draft` (the
first extraction in expanded internal field names). The draft is a suggestion,
not ground truth. Return a COMPLETE corrected compact v2 extraction, replacing
the draft, not a patch or a list of additions. Do not copy expanded field names.

`candidate_units` enumerates owned source lines with punctuation-separated clause
fragments, retaining delimiters and original line numbers. Account for EVERY
fragment: include a normative statement, join it to adjacent fragments about the
same subject, or exclude it under the rules. These fragments are hints, not final
provision units: many fragments must be joined. Use the original source to restore
complete conditions, sentence boundaries, section context and table row/cell context.
Never copy a merged draft just because the entire cell shares one source line.

Re-read the SOURCE first. For every owned line in normative sections, technical
requirement cells and annex procedures, account for each normative sentence and
each clause whose subject or type changes. Recover statements absent from the
draft. Exclude statements supported only by overlap lines. Never use a draft's
presence or absence as evidence that a statement is normative.

Audit each clause by its bound subject before grouping:
- Distinct grammatical subjects mean separate provisions, even across commas,
  even in one technical requirement cell, and even with the same mandatory type.
  A process, its emissions, its residue and its facility are different subjects.
- “人员应记录，记录表应保存。” gives two statements. “工艺应符合甲，排放应符合乙。”
  gives two. “室内无积水，设备运行良好，场地整洁。” gives three implicit requirements.
- An implicit binding state/assigned responsibility still counts without 应/shall.
  An instruction without its own actor and a following clause naming an actor
  are different subjects. Do not retain the original merged row as well.
- Keep comma clauses without a new subject with their preceding clause.
  Do not split lists of actions or properties just to increase the number of rows.
  “车辆应密闭，并保持清洁。” is one. A sentence about one fertilizer/product's
  several properties stays together, including properties separated by semicolons.
  Alternative conditions for the SAME obligation stay together (e.g. treatment
  before sewer discharge versus treatment before direct discharge).
- Separate consecutive sentences in procedure steps, including storage conditions.

Audit exclusions independently of coverage:
- Definitions remain definitions even if they contain 要/应/shall. Do not extract
  the clauses of a term's definition as operating requirements.
- Pure goals, orientations and general management principles remain excluded.
  A principle plus a concrete observable obligation/outcome is included. General
  aspirations such as comprehensive governance or long-term management are not
  made testable simply by containing a verb.
- Exclude descriptions of what a treatment technology IS or how it generally
  works, including introductory descriptions in a technical table. Retain that
  cell's actual equipment, siting, operating and compliance requirements.
- Exclude numeric value tables invoked elsewhere, scope, reference boilerplate,
  formulas and explanations of results. Keep the invoking requirements.

Check `u`, `t`, `lt` and the explicit flag against each corrected SOURCE statement,
not the draft's labels. Preserve exact text without clause numbers. Do not invent
translations, omit conditions, summarize, duplicate rows or add explanatory prose.
Every corrected provision and object needs explicit nonempty source spans.
