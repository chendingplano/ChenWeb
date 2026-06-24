// ============================================================
// Document Review Correction Report Template
// Usage: fill in the parameters and call `document-correction-report(...)`
// Renders the reviewer correction actions (LLM Auto Fix, Edit Tool,
// finding deletions, and Document Structure edits) captured in
// kb.doc_review_activities.
// ============================================================

// ── Colour palette ───────────────────────────────────────────
#let clr-accent      = rgb("#15803d")   // green — corrections / improvements
#let clr-secondary   = rgb("#0f766e")
#let clr-muted       = rgb("#6b7280")
#let clr-divider     = rgb("#d1d5db")
#let clr-old-bg      = rgb("#fef2f2")   // before (removed) — soft red
#let clr-old-fg      = rgb("#b91c1c")
#let clr-new-bg      = rgb("#f0fdf4")   // after (added) — soft green
#let clr-new-fg      = rgb("#15803d")
#let clr-chip-bg     = rgb("#e2e8f0")

// ── Helper: horizontal rule ──────────────────────────────────
#let hrule = line(length: 100%, stroke: 0.5pt + clr-divider)

// ── Helper: labelled metadata row ────────────────────────────
#let meta-row(label, value) = grid(
  columns: (140pt, 1fr),
  gutter: 4pt,
  text(weight: "semibold", fill: clr-muted, size: 9pt, label + ":"),
  text(size: 9pt, value),
)

// ── Helper: kind chip ────────────────────────────────────────
#let kind-chip(label) = box(
  fill: clr-chip-bg,
  inset: (x: 6pt, y: 2pt),
  radius: 3pt,
  text(size: 8pt, weight: "semibold", fill: rgb("#334155"), label),
)

// ── Helper: before / after content blocks ────────────────────
#let before-block(content) = block(
  width: 100%,
  fill: clr-old-bg,
  stroke: 0.5pt + clr-old-fg,
  radius: 4pt,
  inset: (x: 10pt, y: 6pt),
  text(font: "Courier New", size: 8pt, fill: clr-old-fg, content),
)

#let after-block(content) = block(
  width: 100%,
  fill: clr-new-bg,
  stroke: 0.5pt + clr-new-fg,
  radius: 4pt,
  inset: (x: 10pt, y: 6pt),
  text(font: "Courier New", size: 8pt, fill: clr-new-fg, content),
)

// ── correction-entry: one correction action ──────────────────
// Parameters:
//   id       – sequential identifier, e.g. "C-01"
//   kind     – human-readable activity label (e.g. "LLM Auto Fix")
//   location – where in the document (line range / page-line)
//   actor    – who performed the action
//   time     – when it happened
//   before   – content (or none) before the change
//   after    – content (or none) after the change
//   note     – optional extra context (e.g. finding title, model)
#let correction-entry(
  id: "",
  kind: "",
  location: "",
  actor: "",
  time: "",
  before: none,
  after: none,
  note: none,
) = {
  block(
    width: 100%,
    stroke: 0.5pt + clr-divider,
    radius: 6pt,
    inset: 0pt,
    clip: true,
    {
      // Entry header
      block(
        width: 100%,
        fill: clr-secondary,
        inset: (x: 10pt, y: 6pt),
        grid(
          columns: (1fr, auto),
          text(weight: "bold", fill: white, size: 9.5pt, id + "  ·  " + kind),
          text(fill: rgb("#cbd5e1"), size: 8pt, time),
        ),
      )
      pad(x: 12pt, y: 10pt, {
        // Meta row: location + actor
        grid(
          columns: (1fr, auto),
          text(size: 8.5pt, fill: clr-muted, if location != "" { "Location: " + location } else { "" }),
          text(size: 8.5pt, fill: clr-muted, if actor != "" { "By: " + actor } else { "" }),
        )
        if note != none {
          v(3pt)
          text(size: 8.5pt, style: "italic", fill: clr-muted, note)
        }
        v(6pt)

        if before != none {
          text(weight: "semibold", size: 8.5pt, fill: clr-old-fg, "Before")
          v(2pt)
          before-block(before)
          v(5pt)
        }
        if after != none {
          text(weight: "semibold", size: 8.5pt, fill: clr-new-fg, "After")
          v(2pt)
          after-block(after)
        }
      })
    },
  )
}

// ════════════════════════════════════════════════════════════
// MAIN TEMPLATE FUNCTION
// ════════════════════════════════════════════════════════════
// Parameters:
//   doc-title       – title of the document being reviewed
//   doc-id          – document identifier / record id
//   doc-date        – document date
//   generated-by    – who generated this correction report
//   generated-date  – when this report was generated
//   summary         – high-level summary text
//   stats           – array of (kind, count) for the summary table
//   corrections     – array of correction-entry(...) content blocks
//   body            – optional additional content appended at the end
#let document-correction-report(
  doc-title: "Untitled Document",
  doc-id: "",
  doc-date: "",
  generated-by: "",
  generated-date: "",
  summary: [],
  stats: (),
  corrections: (),
  body: [],
) = {
  // ── Page setup ──────────────────────────────────────────────
  set page(
    paper: "a4",
    margin: (top: 2.5cm, bottom: 2.5cm, left: 2.8cm, right: 2.8cm),
    header: context {
      if counter(page).get().first() > 1 {
        grid(
          columns: (1fr, auto),
          text(size: 8pt, fill: clr-muted, "Document Review Correction Report"),
          text(size: 8pt, fill: clr-muted, "Page " + str(counter(page).get().first())),
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
        text(size: 7.5pt, fill: clr-muted, generated-date),
      )
    },
  )

  set text(font: "Linux Libertine", size: 10pt, lang: "en")
  set par(justify: true, leading: 0.65em)

  set heading(numbering: "1.1.")
  show heading.where(level: 1): it => {
    v(16pt)
    block(
      width: 100%,
      fill: clr-accent,
      inset: (x: 12pt, y: 8pt),
      radius: 4pt,
      text(fill: white, weight: "bold", size: 12pt, it.body),
    )
    v(6pt)
  }
  show heading.where(level: 2): it => {
    v(12pt)
    text(fill: clr-accent, weight: "bold", size: 11pt, it.body)
    v(2pt)
    hrule
    v(4pt)
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
        text(fill: white, weight: "bold", size: 20pt, "Document Review Correction Report")
        v(6pt)
        text(fill: rgb("#bbf7d0"), size: 11pt, doc-title)
      },
    )
    v(1cm)
  })

  // ── Chapter 1 – Basic Information ───────────────────────────
  heading(level: 1, "Basic Information")
  block(
    width: 100%,
    stroke: 0.5pt + clr-divider,
    radius: 6pt,
    inset: (x: 14pt, y: 12pt),
    {
      meta-row("Document Title", doc-title)
      v(5pt)
      meta-row("Document ID", doc-id)
      v(5pt)
      meta-row("Document Date", doc-date)
      v(5pt)
      meta-row("Generated By", generated-by)
      v(5pt)
      meta-row("Generated Date", generated-date)
    },
  )

  // ── Chapter 2 – Correction Summary ──────────────────────────
  heading(level: 1, "Correction Summary")
  summary
  v(10pt)

  if stats.len() > 0 {
    heading(level: 2, "Actions by Type")
    table(
      columns: (1fr, auto),
      stroke: 0.5pt + clr-divider,
      fill: (_, row) => if row == 0 { clr-accent } else if calc.even(row) { rgb("#f3f4f6") } else { white },
      inset: (x: 10pt, y: 6pt),
      table.header(
        text(fill: white, weight: "bold", size: 9pt, "Action Type"),
        text(fill: white, weight: "bold", size: 9pt, "Count"),
      ),
      ..stats.map(s => (
        text(size: 9pt, s.kind),
        text(size: 9pt, str(s.count)),
      )).flatten(),
    )
  }

  // ── Chapter 3 – Corrections ─────────────────────────────────
  heading(level: 1, "Corrections")
  if corrections.len() > 0 {
    for (i, c) in corrections.enumerate() {
      if i > 0 { v(8pt) }
      c
    }
  } else {
    text(style: "italic", fill: clr-muted, "No correction actions were recorded for this document.")
  }

  // ── Optional extra body ─────────────────────────────────────
  body
}

// ════════════════════════════════════════════════════════════
// EXAMPLE USAGE  (delete or replace before production use)
// ════════════════════════════════════════════════════════════
#document-correction-report(
  doc-title:      "Example Policy Document v1.0",
  doc-id:         "387",
  doc-date:       "2026-06-23",
  generated-by:   "Automated Review",
  generated-date: "2026-06-24",
  summary: [
    A total of 3 correction actions were applied to this document during review:
    1 LLM Auto Fix, 1 Edit Tool change, and 1 Document Structure edit.
  ],
  stats: (
    (kind: "LLM Auto Fix",            count: 1),
    (kind: "Edit Tool",               count: 1),
    (kind: "Document Structure Edit", count: 1),
  ),
  corrections: (
    correction-entry(
      id: "C-01",
      kind: "LLM Auto Fix",
      location: "42",
      actor: "jdoe",
      time: "2026-06-24 09:12",
      before: [42: The system will ensures that all users receive a confirmation email.],
      after:  [42: The system will ensure that all users receive a confirmation email.],
      note: [Finding: Subject-verb agreement (readability, medium)],
    ),
    correction-entry(
      id: "C-02",
      kind: "Document Structure Edit",
      location: "page 1, line 15",
      actor: "jdoe",
      time: "2026-06-24 09:20",
      before: [5 实验室仪器及设备的分类],
      after:  [5 实验室仪器及设备的分类方法],
      note: [Line type: toc],
    ),
  ),
)
