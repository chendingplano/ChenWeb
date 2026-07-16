// ============================================================
// Document Review Report Template
// Usage: fill in the parameters and call `document-review-report(...)`
// ============================================================

// ── Colour palette ───────────────────────────────────────────
#let clr-accent      = rgb("#1a4f8a")
#let clr-secondary   = rgb("#3a7bc8")
#let clr-muted       = rgb("#6b7280")
#let clr-divider     = rgb("#d1d5db")
#let clr-source-bg   = rgb("#f3f4f6")
#let clr-context-bg  = rgb("#e2e8f4")   // context lines (before/after source)
#let clr-context-fg  = rgb("#5a6a7e")   // muted text for context lines
#let clr-error-bg    = rgb("#fef2f2")
#let clr-error-fg    = rgb("#b91c1c")
#let clr-warn-bg     = rgb("#fffbeb")
#let clr-ok-bg       = rgb("#f0fdf4")

// ── Helper: horizontal rule ──────────────────────────────────
#let hrule = line(length: 100%, stroke: 0.5pt + clr-divider)

// ── Helper: rendered section number for a heading, e.g. "4.1.3." ─────
// Returns none when the heading has no numbering assigned.
#let heading-number(it) = if it.numbering != none {
  context numbering(it.numbering, ..counter(heading).at(it.location()))
} else { none }

// ── Helper: labelled metadata row ────────────────────────────
#let meta-row(label, value) = grid(
  columns: (120pt, 1fr),
  gutter: 4pt,
  text(weight: "semibold", fill: clr-muted, size: 9pt, label + ":"),
  text(size: 9pt, value),
)

// ── Helper: merged context + source + context block ──────────
// Renders before-context, source lines, and after-context as a single
// visually unified block. Pass `none` for before/after to omit them.
#let source-with-context(before: none, source: [], after: none) = block(
  width: 100%,
  stroke: 0.5pt + clr-divider,
  radius: 4pt,
  inset: 0pt,
  clip: true,
  {
    if before != none {
      block(
        width: 100%,
        fill: clr-context-bg,
        inset: (x: 10pt, y: 6pt),
        text(font: "Courier New", size: 8pt, fill: clr-context-fg, before),
      )
    }
    block(
      width: 100%,
      fill: clr-source-bg,
      inset: (x: 10pt, y: 8pt),
      text(font: "Courier New", size: 8pt, source),
    )
    if after != none {
      block(
        width: 100%,
        fill: clr-context-bg,
        inset: (x: 10pt, y: 6pt),
        text(font: "Courier New", size: 8pt, fill: clr-context-fg, after),
      )
    }
  },
)

// ── Helper: error block ──────────────────────────────────────
#let error-block(content) = block(
  width: 100%,
  fill: clr-error-bg,
  stroke: 0.5pt + clr-error-fg,
  radius: 4pt,
  inset: (x: 10pt, y: 8pt),
  text(size: 9pt, fill: clr-error-fg, content),
)

// ── Helper: recommendation block ─────────────────────────────
#let recommendation-block(content) = block(
  width: 100%,
  fill: clr-ok-bg,
  stroke: 0.5pt + rgb("#16a34a"),
  radius: 4pt,
  inset: (x: 10pt, y: 8pt),
  text(size: 9pt, content),
)

// ── review-finding: one individual finding ───────────────────
// Parameters:
//   id      – finding identifier, e.g. "F-01"
//   sources – array of source-context dicts, each with keys:
//               before: content or none  (up to five context lines before)
//               source: content          (the source line(s))
//               after:  content or none  (up to five context lines after)
//             When the array has more than one entry, "Source N:" labels appear.
//   errors      – description of the error(s) found
//   explanation – detailed explanation of why it is an error
//   correction  – recommended correction
//   metric-fields / related-metric-fields – array of (label:, value:) dicts:
//     name-value snapshot of the artifact under review, and of the matched/
//     referenced artifact, for the metrics/provisions/inventory_items
//     reviewers only (ADR 2026071603). Empty for other reviewers or legacy
//     findings with no snapshot.
//   metric-fields-label / related-metric-fields-label – localized section
//     headers for the two blocks above (e.g. "Metric" / "Referenced Metric").
#let review-finding(
  labels: (:),
  id: "",
  sources: (),
  related-sources: (),
  errors: [],
  explanation: [],
  correction: [],
  metric-fields: (),
  metric-fields-label: "",
  related-metric-fields: (),
  related-metric-fields-label: "",
) = {
  let label(key) = labels.at(key, default: key)
  block(
    width: 100%,
    stroke: 0.5pt + clr-divider,
    radius: 6pt,
    inset: 0pt,
    clip: true,
    {
      // Finding header
      block(
        width: 100%,
        fill: clr-secondary,
        inset: (x: 10pt, y: 6pt),
        text(weight: "bold", fill: white, size: 9.5pt,
          if id.starts-with("F-") { label("finding") + " " + id } else { label("finding") + "-" + id }),
      )
      pad(x: 12pt, y: 10pt, {
        // Errors
        text(weight: "semibold", size: 9pt, fill: clr-error-fg, label("errors"))
        v(3pt)
        error-block(errors)
        v(8pt)

        // Explanation
        text(weight: "semibold", size: 9pt, fill: clr-muted, label("explanation"))
        v(3pt)
        block(width: 100%, text(size: 9pt, explanation))
        v(8pt)

        // Recommended correction
        text(weight: "semibold", size: 9pt, fill: rgb("#15803d"), label("recommended-correction"))
        v(3pt)
        recommendation-block(correction)
        v(8pt)

        // Metric/Provision/Inventory-Item fields (artifact under review)
        if metric-fields.len() > 0 {
          text(weight: "semibold", size: 9pt, fill: clr-muted, metric-fields-label)
          v(3pt)
          block(width: 100%, {
            for f in metric-fields {
              meta-row(f.at("label"), f.at("value"))
              v(2pt)
            }
          })
          v(8pt)
        }

        // Source blocks (one per location group)
        text(weight: "semibold", size: 9pt, fill: clr-muted, label("related-source-lines"))
        v(3pt)
        let src-n = sources.len()
        for (i, sc) in sources.enumerate() {
          if src-n > 1 {
            text(size: 8.5pt, weight: "semibold", fill: clr-muted,
              label("source") + " " + str(i + 1) + " " + label("of") + " " + str(src-n) + ":")
            v(2pt)
          }
          source-with-context(
            before: sc.at("before", default: none),
            source: sc.at("source", default: []),
            after:  sc.at("after",  default: none),
          )
          if i < src-n - 1 { v(6pt) }
        }
        v(8pt)

        // Referenced Metric/Provision/Inventory-Item fields (matched artifact)
        if related-metric-fields.len() > 0 {
          text(weight: "semibold", size: 9pt, fill: clr-muted, related-metric-fields-label)
          v(3pt)
          block(width: 100%, {
            for f in related-metric-fields {
              meta-row(f.at("label"), f.at("value"))
              v(2pt)
            }
          })
          v(8pt)
        }

        if related-sources.len() > 0 {
          text(weight: "semibold", size: 9pt, fill: clr-muted, label("referenced-matching-metric-lines"))
          v(3pt)
          let rel-n = related-sources.len()
          for (i, sc) in related-sources.enumerate() {
            if rel-n > 1 {
              text(size: 8.5pt, weight: "semibold", fill: clr-muted,
              label("matched-source") + " " + str(i + 1) + " " + label("of") + " " + str(rel-n) + ":")
              v(2pt)
            }
            source-with-context(
              before: sc.at("before", default: none),
              source: sc.at("source", default: []),
              after:  sc.at("after",  default: none),
            )
            if i < rel-n - 1 { v(6pt) }
          }
        }
      })
    },
  )
}

// ── artifact-group: findings + comparison analyses for one artifact ─
// Parameters:
//   title    – artifact identifier (metric_id / prov_id / inventory_item_id)
//   analyses – array of dicts: (related: string, relationship: string, summary: content).
//              Comparison records for this artifact, retained independent of
//              whether a finding was raised (ADR 2026070602). Empty when the
//              reviewer has no analyses table yet (metrics, inventory_items).
//   findings – array of content blocks produced by review-finding(...)
#let artifact-group(
  labels: (:),
  title: "",
  analyses: (),
  findings: (),
) = {
  let label(key) = labels.at(key, default: key)
  heading(level: 3, title)

  if analyses.len() > 0 {
    text(weight: "semibold", size: 9pt, fill: clr-muted, label("comparison-analyses"))
    v(3pt)
    for a in analyses {
      block(
        width: 100%,
        fill: clr-source-bg,
        radius: 4pt,
        inset: (x: 10pt, y: 8pt),
        {
          text(weight: "semibold", size: 8.5pt, fill: clr-muted,
            label("versus") + " " + a.related + "  ·  " + a.relationship)
          v(3pt)
          text(size: 9pt, a.summary)
        },
      )
      v(6pt)
    }
    v(4pt)
  }

  if findings.len() > 0 {
    for f in findings {
      v(8pt)
      f
    }
  } else {
    v(4pt)
    text(style: "italic", fill: clr-muted, size: 9pt, label("no-findings-artifact"))
  }
}

// ── aspect-section: one reviewed aspect ──────────────────────
// Parameters:
//   title      – aspect name, e.g. "Grammar & Style"
//   findings   – array of content blocks produced by review-finding(...)
//   assessment – overall quality assessment for this aspect
//   problems   – main problem analysis
//   guidelines – guidelines / recommendations for the future
#let aspect-section(
  labels: (:),
  title: "",
  findings: (),
  artifact-groups: (),
  assessment: [],
  problems: [],
  guidelines: [],
) = {
  let label(key) = labels.at(key, default: key)
  heading(level: 2, title)

  if artifact-groups.len() > 0 {
    for g in artifact-groups {
      v(8pt)
      g
    }
  } else if findings.len() > 0 {
    for f in findings {
      v(8pt)
      f
    }
  } else {
    v(4pt)
    text(style: "italic", fill: clr-muted, size: 9pt, label("no-findings-aspect"))
  }

  v(12pt)
  heading(level: 4, label("overall-assessment"))
  assessment
  v(8pt)

  heading(level: 4, label("main-problem-analysis"))
  problems
  v(8pt)

  heading(level: 4, label("guidelines-recommendations"))
  guidelines
}

// ── reference-list: grounding / supporting references ────────
// Each entry is a dictionary: (id, title, description)
#let reference-list(entries, labels: (:)) = {
  let label(key) = labels.at(key, default: key)
  if entries.len() == 0 {
    text(style: "italic", fill: clr-muted, size: 9pt, label("no-references"))
    return
  }
  for e in entries {
    grid(
      columns: (40pt, 1fr),
      gutter: 6pt,
      text(weight: "bold", size: 9pt, fill: clr-accent, e.id),
      {
        text(weight: "semibold", size: 9pt, e.title)
        if "description" in e and e.description != "" [
          #linebreak()
          #text(size: 8.5pt, fill: clr-muted, e.description)
        ]
      },
    )
    v(6pt)
  }
}

// ════════════════════════════════════════════════════════════
// MAIN TEMPLATE FUNCTION
// ════════════════════════════════════════════════════════════
// Parameters:
//   doc-title        – title of the document being reviewed
//   doc-id           – document identifier / version
//   doc-date         – document date
//   reviewer         – reviewer name(s)
//   review-date      – date this review was conducted
//   review-scope     – short description of review scope
//   aspects          – list of aspect-section(...) content blocks
//   aspect-stats     – array of (aspect, count) for the summary table
//   summary          – high-level summary text (content or string)
//   grounding-refs   – array of reference entries (id, title, description)
//   supporting-refs  – array of reference entries (id, title, description)
//   body             – optional additional content appended at the end
#let document-review-report(
  lang: "en",
  font: "Linux Libertine",
  labels: (:),
  doc-title: "Untitled Document",
  doc-id: "",
  doc-date: "",
  reviewer: "",
  review-date: "",
  review-scope: "",
  aspects: (),
  aspect-stats: (),
  summary: [],
  grounding-refs: (),
  supporting-refs: (),
  body: [],
) = {
  let label(key) = labels.at(key, default: key)
  // ── Page setup ──────────────────────────────────────────────
  set page(
    paper: "a4",
    margin: (top: 2.5cm, bottom: 2.5cm, left: 2.8cm, right: 2.8cm),
    header: context {
      if counter(page).get().first() > 1 {
        grid(
          columns: (1fr, auto),
          text(size: 8pt, fill: clr-muted, label("report-title")),
          text(size: 8pt, fill: clr-muted, label("page") + " " + str(counter(page).get().first())),
        )
        v(-4pt)
        hrule
      }
    },
    footer: context {
      hrule
      v(-4pt)
      grid(
        columns: (1fr, auto),
        text(size: 7.5pt, fill: clr-muted, doc-title),
        text(size: 7.5pt, fill: clr-muted, review-date),
      )
    },
  )

  set text(font: font, size: 10pt, lang: lang)
  set par(justify: true, leading: 0.65em)

  set heading(numbering: "1.1.")
  show heading.where(level: 1): it => {
    v(16pt)
    block(
      width: 100%,
      fill: clr-accent,
      inset: (x: 12pt, y: 8pt),
      radius: 4pt,
      text(fill: white, weight: "bold", size: 12pt, {
        heading-number(it)
        if it.numbering != none { h(6pt) }
        it.body
      }),
    )
    v(6pt)
  }
  show heading.where(level: 2): it => {
    v(12pt)
    text(fill: clr-accent, weight: "bold", size: 11pt, {
      heading-number(it)
      if it.numbering != none { h(6pt) }
      it.body
    })
    v(2pt)
    hrule
    v(4pt)
  }
  show heading.where(level: 3): it => {
    v(10pt)
    text(fill: clr-secondary, weight: "semibold", size: 10.5pt, {
      heading-number(it)
      if it.numbering != none { h(6pt) }
      it.body
    })
    v(3pt)
  }
  show heading.where(level: 4): it => {
    v(6pt)
    text(fill: clr-muted, weight: "semibold", size: 9.5pt, {
      heading-number(it)
      if it.numbering != none { h(6pt) }
      it.body
    })
    v(2pt)
  }

  // ── Cover block ─────────────────────────────────────────────
  align(center, {
    v(1cm)
    block(
      width: 100%,
      fill: clr-accent,
      radius: 8pt,
      inset: (x: 24pt, y: 20pt),
      {
        text(fill: white, weight: "bold", size: 20pt, label("report-title"))
        v(6pt)
        text(fill: rgb("#93c5fd"), size: 11pt, doc-title)
      },
    )
    v(1cm)
  })

  // ── Table of Contents ───────────────────────────────────────
  heading(level: 1, label("table-of-contents"))
  outline(title: none)
  pagebreak()

  // ── Chapter 1 – Basic Information ───────────────────────────
  heading(level: 1, label("basic-information"))

  block(
    width: 100%,
    stroke: 0.5pt + clr-divider,
    radius: 6pt,
    inset: (x: 14pt, y: 12pt),
    {
      meta-row(label("document-title"), doc-title)
      v(5pt)
      meta-row(label("document-id"), doc-id)
      v(5pt)
      meta-row(label("document-date"), doc-date)
      v(5pt)
      meta-row(label("reviewer"), reviewer)
      v(5pt)
      meta-row(label("review-date"), review-date)
      v(5pt)
      meta-row(label("review-scope"), review-scope)
    },
  )

  // ── Chapter 2 – Review Results ──────────────────────────────
  heading(level: 1, label("review-results"))

  // 2.1 Summary
  heading(level: 2, label("summary"))
  summary
  v(10pt)

  // 2.2 Statistics table
  if aspect-stats.len() > 0 {
    heading(level: 2, label("statistics"))
    table(
      columns: (1fr, auto),
      stroke: 0.5pt + clr-divider,
      fill: (_, row) => if row == 0 { clr-accent } else if calc.even(row) { clr-source-bg } else { white },
      inset: (x: 10pt, y: 6pt),
      table.header(
        text(fill: white, weight: "bold", size: 9pt, label("aspect")),
        text(fill: white, weight: "bold", size: 9pt, label("findings")),
      ),
      ..aspect-stats.map(s => (
        text(size: 9pt, s.aspect),
        text(size: 9pt, str(s.count)),
      )).flatten(),
    )
  }

  // ── Package and aspect sections ─────────────────────────────
  // Each content block in `aspects` contains a Level 1 heading for the
  // package name followed by Level 2 headings for its individual aspects.
  if aspects.len() > 0 {
    for a in aspects { a }
  } else {
    text(style: "italic", fill: clr-muted, label("no-aspects"))
  }

  // ── Chapter 3 – Grounding References ────────────────────────
  heading(level: 1, label("grounding-references"))
  reference-list(grounding-refs, labels: labels)

  // ── Chapter 4 – Supporting References ───────────────────────
  heading(level: 1, label("supporting-references"))
  reference-list(supporting-refs, labels: labels)

  // ── Optional extra body ─────────────────────────────────────
  body
}

// ════════════════════════════════════════════════════════════
// EXAMPLE USAGE  (delete or replace before production use)
// ════════════════════════════════════════════════════════════
#document-review-report(
  doc-title:    "Example Policy Document v1.0",
  doc-id:       "doc-20260101-01-example",
  doc-date:     "2026-01-01",
  reviewer:     "Jane Reviewer",
  review-date:  "2026-06-22",
  review-scope: "Grammar, Completeness, Logical Consistency",

  summary: [
    This review examined the document across three aspects: grammar and style,
    completeness, and logical consistency. A total of two findings were identified.
    The document is generally well-structured but contains several grammatical errors
    and one logical gap that should be addressed before publication.
  ],

  aspect-stats: (
    (aspect: "Grammar & Style",        count: 2),
    (aspect: "Completeness",           count: 0),
    (aspect: "Logical Consistency",    count: 1),
  ),

  aspects: (
    aspect-section(
      title: "Grammar & Style",
      findings: (
        review-finding(
          id: "F-01",
          sources: (
            (
              before: [1: All users must authenticate before accessing the system.\
2: Authentication tokens are valid for 24 hours.\
3: Upon successful authentication, the system grants access.],
              source: [4: The system will ensures that all users receive a confirmation email.],
              after:  [5: Failed attempts are logged for audit purposes.\
6: Account lockout occurs after five consecutive failures.\
7: Locked accounts must be unlocked by an administrator.],
            ),
          ),
          errors:      [Subject-verb agreement error: "will ensures" should be "will ensure".],
          explanation: [When a modal verb (will, shall, may, etc.) is used, the main verb
                        must remain in its base (infinitive) form without the third-person
                        singular -s suffix.],
          correction:  [Change to: "The system will ensure that all users..."],
        ),
        review-finding(
          id: "F-02",
          sources: (
            (
              before: [10: User Roles and Permissions\
11: Three roles are defined: Admin, Editor, Viewer.\
12: Role assignment is managed by the system administrator.\
13: Roles are reviewed annually.],
              source: [14: Procedures For Handling Requests],
              after:  [15: Requests are submitted via the online portal.\
16: A reference number is issued upon submission.],
            ),
          ),
          errors:      [Inconsistent capitalisation in heading; sentence case should be used.],
          explanation: [The style guide mandates sentence case for all headings (only first
                        word and proper nouns capitalised).],
          correction:  [Change to: "Procedures for handling requests"],
        ),
      ),
      assessment:  [The document's grammar is mostly acceptable. Two minor errors were found,
                    both of which are straightforward to correct.],
      problems:    [The main issue is inconsistent application of modal verb rules and
                    heading capitalisation conventions.],
      guidelines:  [
        - Run a grammar checker before submission.
        - Review the style guide section on headings and modal verbs.
        - Consider a peer proofreading pass for future documents.
      ],
    ),
    aspect-section(
      title: "Completeness",
      findings: (),
      assessment:  [The document covers all required sections. No completeness issues found.],
      problems:    [None identified.],
      guidelines:  [Continue using the standard document checklist to ensure completeness.],
    ),
    aspect-section(
      title: "Logical Consistency",
      findings: (
        review-finding(
          id: "F-03",
          sources: (
            (
              before: [28: All access requests must be submitted in writing.\
29: Requests are reviewed within two business days.\
30: Access Control Requirements],
              source: [31: Access is granted after two approvals.\
32: The approvers must be from different departments.],
              after:  [33: Read-only access to shared resources.\
34: Write access always requires the full approval chain.\
35: Audit and Compliance],
            ),
            (
              before: [68: Data classification governs access tiers.\
69: Tier 1 data requires no approval.\
70: Tier 2 data requires manager sign-off.],
              source: [71: One approval is sufficient for read-only access.],
              after:  [72: Write access to Tier 3 data requires CISO approval.\
73: Access logs are retained for 12 months.],
            ),
          ),
          errors:      [Contradiction between line 31 and line 71 regarding approval requirements.],
          explanation: [Readers cannot determine which rule takes precedence. This ambiguity
                        may lead to incorrect access-control decisions.],
          correction:  [Add a clarifying note: "For read-only access one approval suffices;
                        write access requires two approvals from different departments."
                        Align or remove the conflicting statement at line 71.],
        ),
      ),
      assessment:  [One significant logical contradiction was found. It must be resolved
                    before the document is approved.],
      problems:    [Contradictory requirements across sections create ambiguity for implementers.],
      guidelines:  [
        - Use a requirements-traceability matrix to cross-reference related rules.
        - Perform a consistency check as part of the final review gate.
      ],
    ),
  ),

  grounding-refs: (
    (id: "[G1]", title: "Internal Style Guide v3.2",       description: "Authoritative style rules for all company documents."),
    (id: "[G2]", title: "Access Control Policy ACP-2024",  description: "Defines approval workflows for system access."),
  ),

  supporting-refs: (
    (id: "[S1]", title: "Grammar Best Practices Handbook", description: "Reference for grammatical conventions used in this review."),
    (id: "[S2]", title: "Document Review Checklist RC-06", description: "Standard checklist applied during this review, per configuration."),
  ),
)
