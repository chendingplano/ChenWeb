## 1. Component State Updates

- [x] 1.1 Add `let packages = $state<ReviewPackageInfo[]>([])` to component state and import `ReviewPackageInfo` type
- [x] 1.2 In `pollStatus`, store `result.packages` into the new `packages` state variable

## 2. Derived Chart Data

- [x] 2.1 Add `$derived.by` for severity chart slices: array of `{ label, count, color }` for High (#ef4444), Medium (#f59e0b), Low (#22c55e)
- [x] 2.2 Add `$derived.by` for package chart data: `nonEmptyPackages` (pass codes with count > 0) and `emptyPackages` (packages from state with no findings)
- [x] 2.3 Add `$derived.by` for reviewer chart data: `nonEmptyReviewers` (aspects with count > 0) and `emptyReviewers` (aspectStatuses entries with `finding_count === 0`)
- [x] 2.4 Add `$derived` for `elapsedSeconds`: compute from `request.start_time` and `request.end_time`; return null if `end_time` absent

## 3. SVG Donut Chart Helper

- [x] 3.1 Write a pure function `buildDonutSlices(items: {count: number, color: string}[], r: number)` that returns SVG arc parameters (`strokeDasharray`, `strokeDashoffset`) for each slice using the stroke-dasharray/dashoffset technique
- [x] 3.2 Verify the function handles edge cases: all zero counts (render empty circle) and single-item (full circle)

## 4. Replace Summary Cards

- [x] 4.1 Remove the four stat cards (lines 292–310) and replace the grid with `grid-template-columns: repeat(2, 1fr)`
- [x] 4.2 Add severity donut chart panel: SVG donut using `buildDonutSlices` + legend rows (label + count for High / Medium / Low)
- [x] 4.3 Add package donut chart panel: SVG donut for non-empty packages + "No findings" list for empty packages beside it
- [x] 4.4 Add reviewer donut chart panel: SVG donut for non-empty reviewers + "No findings" list for empty reviewers beside it
- [x] 4.5 Add metadata block panel: rows for Start Time, Time Used, Total Findings, Total Non-Empty Packages, Total Non-Empty Reviewers

## 5. Polish & Verification

- [x] 5.1 Ensure all panels respect `darkMode`, `cardBg`, `borderColor`, `textPrimary`, `textSecondary`, `textMuted` design tokens
- [x] 5.2 Verify layout in completed review state — panels fill the grid without overflow
- [x] 5.3 Check edge case: review with zero findings (all chart panels show empty state gracefully)
