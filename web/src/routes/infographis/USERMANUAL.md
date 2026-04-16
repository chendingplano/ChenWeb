# AntV Infographic User Manual (ChenWeb Wrapper)

Created by: Codex \
Date: 2026/04/08 \
GitHub: https://infographic.antv.vision/ \
Version: 1.0

## Scope

This manual is for the wrapper at:

- `web/src/routes/infographis/example-01/InfographicWrapper.svelte`

It explains how to pass the two wrapper parameters:

1. `syntax`
2. `data`

## What the wrapper expects

The wrapper renders one final AntV syntax string like this:

```txt
<syntax>
<data block>
```

Behavior:

- If `data` already starts with `data`, it is used directly.
- Otherwise the wrapper prepends `data\n`.

So both of these are valid:

```ts
const syntax = 'infographic list-row-simple-horizontal-arrow';
const data = `data
  lists
    - label Step 1
      desc Start`;
```

```ts
const syntax = 'infographic list-row-simple-horizontal-arrow';
const data = `lists
  - label Step 1
    desc Start`;
```

## Mental model: `syntax` vs `data`

- `syntax` picks template/design/theme (visual language).
- `data` provides content structure.
- They are separable, but **not fully independent**.

The official docs state template and data are typically corresponding (for example, some templates expect `lists`, others expect `nodes + relations`, others `values`, etc.).

Practical rule:

- You can change `syntax` to any valid template.
- The chart will render well only when `data` shape matches that template family.

## Syntax rules (important)

AntV syntax is indentation-sensitive.

- First line: `infographic <template-name>`
- Indentation: 2 spaces
- Arrays: `-` list item style
- Keep container config (`width`, `height`, `padding`, `editable`) in `new Infographic({...})`, not in syntax text.

## Common data families

### 1. List format (`data.lists`)

Use for unordered collections (feature lists, checklists, cards).

```txt
infographic list-grid-compact-card
data
  title Purchase List
  lists
    - label Watermelon
      icon watermelon
    - label Apple
      icon apple
```

### 2. Sequence format (`data.sequences`)

Use for ordered steps or timelines. You can add `order asc|desc`.

```txt
infographic sequence-steps-simple
data
  title Release Flow
  sequences
    - label Plan
    - label Build
    - label Ship
  order asc
```

### 3. Hierarchy format (`data.root` + `children`)

Use for tree structures (org chart, taxonomy, decomposition).

```txt
infographic hierarchy-structure
data
  root
    label Company
    children
      - label Engineering
        children
          - label Backend
          - label Frontend
      - label Operations
```

### 4. Compare format (`data.compares`)

Use for side-by-side comparison groups, optionally with nested details.

```txt
infographic compare-swot
data
  compares
    - label Option A
      value 68
      children
        - label Fast delivery
        - label Higher cost
    - label Option B
      value 82
      children
        - label Lower cost
        - label Slower onboarding
```

### 5. Statistics format (`data.values`)

Use for KPI, chart, and metric-like templates. Add `category` for grouped series.

```txt
infographic chart-column-grouped-simple
data
  title Monthly Rainfall
  values
    - label Jan
      value 18.9
      category Chongqing
    - label Jan
      value 12.4
      category Beijing
    - label Feb
      value 15.6
      category Chongqing
```

### 6. Relation format (`data.nodes` + `data.relations`)

Use for flow graphs and networks. Define nodes and edges explicitly.

```txt
infographic relation-dagre-flow-tb-simple-circle-node
data
  title Relation Graph
  nodes
    - id A
      label Node A
    - id B
      label Node B
  relations
    - from A
      to B
      label A to B
```

### 7. Generic format (`data.items`)

Fallback when you are unsure of the exact family. Some templates can adapt from `items`.

```txt
infographic list-row-horizontal-icon-arrow
data
  title Generic Items
  items
    - label Item 1
      desc Description 1
      value 12
    - label Item 2
      desc Description 2
      value 24
```

## Working pattern in this project

In `+page.svelte`:

1. Set `syntax` to a template id
2. Set `data` with matching shape
3. Pass both into `<InfographicWrapper {syntax} {data} />`

## Recommended workflow when changing template

1. Start with only:
   - `syntax = 'infographic <new-template-id>'`
   - a minimal `data` block (`title`, one or two items)
2. Add fields gradually (`desc`, `value`, `icon`, `category`, `children`)
3. If blank output appears:
   - verify indentation
   - verify `data` root key matches template family
   - test with `items` as fallback

## Debug tips

Attach event handlers on the Infographic instance for diagnostics:

- `warning`: non-fatal syntax parse warnings
- `error`: parse/render failure
- `rendered`: render done

Also useful API methods:

- `getOptions()` to inspect parsed options
- `getTypes()` to get expected TS type hints for current template
- `update()` for partial updates

## Optional object mode (advanced)

AntV `render()` supports either:

- syntax string, or
- partial options object (`{ template, data, design, theme, ... }`)

Current wrapper is string-first. If needed, we can add object-mode wrapper props later.

## Reference links

- Reference home: https://infographic.antv.vision/reference
- Infographic API: https://infographic.antv.vision/reference/infographic-api
- Infographic options: https://infographic.antv.vision/reference/infographic-options
- Infographic syntax guide: https://infographic.antv.vision/learn/infographic-syntax
- Built-in templates: https://infographic.antv.vision/reference/built-in-templates
- Built-in structures: https://infographic.antv.vision/reference/built-in-structures
- Built-in items: https://infographic.antv.vision/reference/built-in-items

## All supported syntax values (for @antv/infographic 0.2.16)

These are all template IDs returned by `getTemplates()` in your installed package.
Use each one as:

```txt
infographic <template-id>
```

Total templates: 276

### chart (11)

```txt
chart-bar-plain-text
chart-column-simple
chart-line-plain-text
chart-pie-compact-card
chart-pie-donut-compact-card
chart-pie-donut-pill-badge
chart-pie-donut-plain-text
chart-pie-pill-badge
chart-pie-plain-text
chart-wordcloud
chart-wordcloud-rotate
```

### compare (20)

```txt
compare-binary-horizontal-badge-card-arrow
compare-binary-horizontal-badge-card-fold
compare-binary-horizontal-badge-card-vs
compare-binary-horizontal-compact-card-arrow
compare-binary-horizontal-compact-card-fold
compare-binary-horizontal-compact-card-vs
compare-binary-horizontal-simple-arrow
compare-binary-horizontal-simple-fold
compare-binary-horizontal-simple-vs
compare-binary-horizontal-underline-text-arrow
compare-binary-horizontal-underline-text-fold
compare-binary-horizontal-underline-text-vs
compare-hierarchy-left-right-circle-node-pill-badge
compare-hierarchy-left-right-circle-node-plain-text
compare-hierarchy-row-letter-card-compact-card
compare-hierarchy-row-letter-card-rounded-rect-node
compare-quadrant-quarter-circular
compare-quadrant-quarter-simple-card
compare-quadrant-simple-illus
compare-swot
```

### hierarchy (112)

```txt
hierarchy-mindmap-branch-gradient-capsule-item
hierarchy-mindmap-branch-gradient-circle-progress
hierarchy-mindmap-branch-gradient-compact-card
hierarchy-mindmap-branch-gradient-lined-palette
hierarchy-mindmap-branch-gradient-rounded-rect
hierarchy-mindmap-level-gradient-capsule-item
hierarchy-mindmap-level-gradient-circle-progress
hierarchy-mindmap-level-gradient-compact-card
hierarchy-mindmap-level-gradient-lined-palette
hierarchy-mindmap-level-gradient-rounded-rect
hierarchy-structure
hierarchy-structure-mirror
hierarchy-tree-bt-curved-line-badge-card
hierarchy-tree-bt-curved-line-capsule-item
hierarchy-tree-bt-curved-line-compact-card
hierarchy-tree-bt-curved-line-ribbon-card
hierarchy-tree-bt-curved-line-rounded-rect-node
hierarchy-tree-bt-dashed-arrow-badge-card
hierarchy-tree-bt-dashed-arrow-capsule-item
hierarchy-tree-bt-dashed-arrow-compact-card
hierarchy-tree-bt-dashed-arrow-ribbon-card
hierarchy-tree-bt-dashed-arrow-rounded-rect-node
hierarchy-tree-bt-dashed-line-badge-card
hierarchy-tree-bt-dashed-line-capsule-item
hierarchy-tree-bt-dashed-line-compact-card
hierarchy-tree-bt-dashed-line-ribbon-card
hierarchy-tree-bt-dashed-line-rounded-rect-node
hierarchy-tree-bt-distributed-origin-badge-card
hierarchy-tree-bt-distributed-origin-capsule-item
hierarchy-tree-bt-distributed-origin-compact-card
hierarchy-tree-bt-distributed-origin-ribbon-card
hierarchy-tree-bt-distributed-origin-rounded-rect-node
hierarchy-tree-bt-tech-style-badge-card
hierarchy-tree-bt-tech-style-capsule-item
hierarchy-tree-bt-tech-style-compact-card
hierarchy-tree-bt-tech-style-ribbon-card
hierarchy-tree-bt-tech-style-rounded-rect-node
hierarchy-tree-curved-line-badge-card
hierarchy-tree-curved-line-capsule-item
hierarchy-tree-curved-line-compact-card
hierarchy-tree-curved-line-ribbon-card
hierarchy-tree-curved-line-rounded-rect-node
hierarchy-tree-dashed-arrow-badge-card
hierarchy-tree-dashed-arrow-capsule-item
hierarchy-tree-dashed-arrow-compact-card
hierarchy-tree-dashed-arrow-ribbon-card
hierarchy-tree-dashed-arrow-rounded-rect-node
hierarchy-tree-dashed-line-badge-card
hierarchy-tree-dashed-line-capsule-item
hierarchy-tree-dashed-line-compact-card
hierarchy-tree-dashed-line-ribbon-card
hierarchy-tree-dashed-line-rounded-rect-node
hierarchy-tree-distributed-origin-badge-card
hierarchy-tree-distributed-origin-capsule-item
hierarchy-tree-distributed-origin-compact-card
hierarchy-tree-distributed-origin-ribbon-card
hierarchy-tree-distributed-origin-rounded-rect-node
hierarchy-tree-lr-curved-line-badge-card
hierarchy-tree-lr-curved-line-capsule-item
hierarchy-tree-lr-curved-line-compact-card
hierarchy-tree-lr-curved-line-ribbon-card
hierarchy-tree-lr-curved-line-rounded-rect-node
hierarchy-tree-lr-dashed-arrow-badge-card
hierarchy-tree-lr-dashed-arrow-capsule-item
hierarchy-tree-lr-dashed-arrow-compact-card
hierarchy-tree-lr-dashed-arrow-ribbon-card
hierarchy-tree-lr-dashed-arrow-rounded-rect-node
hierarchy-tree-lr-dashed-line-badge-card
hierarchy-tree-lr-dashed-line-capsule-item
hierarchy-tree-lr-dashed-line-compact-card
hierarchy-tree-lr-dashed-line-ribbon-card
hierarchy-tree-lr-dashed-line-rounded-rect-node
hierarchy-tree-lr-distributed-origin-badge-card
hierarchy-tree-lr-distributed-origin-capsule-item
hierarchy-tree-lr-distributed-origin-compact-card
hierarchy-tree-lr-distributed-origin-ribbon-card
hierarchy-tree-lr-distributed-origin-rounded-rect-node
hierarchy-tree-lr-tech-style-badge-card
hierarchy-tree-lr-tech-style-capsule-item
hierarchy-tree-lr-tech-style-compact-card
hierarchy-tree-lr-tech-style-ribbon-card
hierarchy-tree-lr-tech-style-rounded-rect-node
hierarchy-tree-rl-curved-line-badge-card
hierarchy-tree-rl-curved-line-capsule-item
hierarchy-tree-rl-curved-line-compact-card
hierarchy-tree-rl-curved-line-ribbon-card
hierarchy-tree-rl-curved-line-rounded-rect-node
hierarchy-tree-rl-dashed-arrow-badge-card
hierarchy-tree-rl-dashed-arrow-capsule-item
hierarchy-tree-rl-dashed-arrow-compact-card
hierarchy-tree-rl-dashed-arrow-ribbon-card
hierarchy-tree-rl-dashed-arrow-rounded-rect-node
hierarchy-tree-rl-dashed-line-badge-card
hierarchy-tree-rl-dashed-line-capsule-item
hierarchy-tree-rl-dashed-line-compact-card
hierarchy-tree-rl-dashed-line-ribbon-card
hierarchy-tree-rl-dashed-line-rounded-rect-node
hierarchy-tree-rl-distributed-origin-badge-card
hierarchy-tree-rl-distributed-origin-capsule-item
hierarchy-tree-rl-distributed-origin-compact-card
hierarchy-tree-rl-distributed-origin-ribbon-card
hierarchy-tree-rl-distributed-origin-rounded-rect-node
hierarchy-tree-rl-tech-style-badge-card
hierarchy-tree-rl-tech-style-capsule-item
hierarchy-tree-rl-tech-style-compact-card
hierarchy-tree-rl-tech-style-ribbon-card
hierarchy-tree-rl-tech-style-rounded-rect-node
hierarchy-tree-tech-style-badge-card
hierarchy-tree-tech-style-capsule-item
hierarchy-tree-tech-style-compact-card
hierarchy-tree-tech-style-ribbon-card
hierarchy-tree-tech-style-rounded-rect-node
```

### list (29)

```txt
list-column-done-list
list-column-simple-vertical-arrow
list-column-vertical-icon-arrow
list-grid-badge-card
list-grid-candy-card-lite
list-grid-circular-progress
list-grid-compact-card
list-grid-done-list
list-grid-horizontal-icon-arrow
list-grid-progress-card
list-grid-ribbon-card
list-grid-simple
list-pyramid-badge-card
list-pyramid-compact-card
list-pyramid-rounded-rect-node
list-row-circular-progress
list-row-horizontal-icon-arrow
list-row-horizontal-icon-line
list-row-simple-horizontal-arrow
list-row-simple-illus
list-sector-half-plain-text
list-sector-plain-text
list-sector-simple
list-waterfall-badge-card
list-waterfall-compact-card
list-zigzag-down-compact-card
list-zigzag-down-simple
list-zigzag-up-compact-card
list-zigzag-up-simple
```

### quadrant (3)

```txt
quadrant-quarter-circular
quadrant-quarter-simple-card
quadrant-simple-illus
```

### relation (18)

```txt
relation-circle-circular-progress
relation-circle-icon-badge
relation-dagre-flow-lr-animated-badge-card
relation-dagre-flow-lr-animated-capsule
relation-dagre-flow-lr-animated-compact-card
relation-dagre-flow-lr-animated-simple-circle-node
relation-dagre-flow-lr-badge-card
relation-dagre-flow-lr-compact-card
relation-dagre-flow-lr-simple-circle-node
relation-dagre-flow-tb-animated-badge-card
relation-dagre-flow-tb-animated-capsule
relation-dagre-flow-tb-animated-compact-card
relation-dagre-flow-tb-animated-simple-circle-node
relation-dagre-flow-tb-badge-card
relation-dagre-flow-tb-compact-card
relation-dagre-flow-tb-simple-circle-node
relation-network-icon-badge
relation-network-simple-circle-node
```

### sequence (83)

```txt
sequence-ascending-stairs-3d-simple
sequence-ascending-stairs-3d-underline-text
sequence-ascending-steps
sequence-circle-arrows-indexed-card
sequence-circular-simple
sequence-circular-underline-text
sequence-color-snake-steps-horizontal-icon-line
sequence-color-snake-steps-simple-illus
sequence-cylinders-3d-simple
sequence-filter-mesh-simple
sequence-filter-mesh-underline-text
sequence-funnel-simple
sequence-horizontal-zigzag-horizontal-icon-line
sequence-horizontal-zigzag-plain-text
sequence-horizontal-zigzag-simple
sequence-horizontal-zigzag-simple-horizontal-arrow
sequence-horizontal-zigzag-simple-illus
sequence-horizontal-zigzag-underline-text
sequence-interaction-compact-animated-badge-card
sequence-interaction-compact-animated-capsule-item
sequence-interaction-compact-animated-compact-card
sequence-interaction-compact-animated-rounded-rect-node
sequence-interaction-compact-badge-card
sequence-interaction-compact-capsule-item
sequence-interaction-compact-compact-card
sequence-interaction-compact-dashed-badge-card
sequence-interaction-compact-dashed-capsule-item
sequence-interaction-compact-dashed-compact-card
sequence-interaction-compact-dashed-rounded-rect-node
sequence-interaction-compact-rounded-rect-node
sequence-interaction-default-animated-badge-card
sequence-interaction-default-animated-capsule-item
sequence-interaction-default-animated-compact-card
sequence-interaction-default-animated-rounded-rect-node
sequence-interaction-default-badge-card
sequence-interaction-default-capsule-item
sequence-interaction-default-compact-card
sequence-interaction-default-dashed-badge-card
sequence-interaction-default-dashed-capsule-item
sequence-interaction-default-dashed-compact-card
sequence-interaction-default-dashed-rounded-rect-node
sequence-interaction-default-rounded-rect-node
sequence-interaction-wide-animated-badge-card
sequence-interaction-wide-animated-capsule-item
sequence-interaction-wide-animated-compact-card
sequence-interaction-wide-animated-rounded-rect-node
sequence-interaction-wide-badge-card
sequence-interaction-wide-capsule-item
sequence-interaction-wide-compact-card
sequence-interaction-wide-dashed-badge-card
sequence-interaction-wide-dashed-capsule-item
sequence-interaction-wide-dashed-compact-card
sequence-interaction-wide-dashed-rounded-rect-node
sequence-interaction-wide-rounded-rect-node
sequence-mountain-underline-text
sequence-pyramid-simple
sequence-roadmap-vertical-badge-card
sequence-roadmap-vertical-pill-badge
sequence-roadmap-vertical-plain-text
sequence-roadmap-vertical-quarter-circular
sequence-roadmap-vertical-quarter-simple-card
sequence-roadmap-vertical-simple
sequence-roadmap-vertical-underline-text
sequence-snake-steps-compact-card
sequence-snake-steps-pill-badge
sequence-snake-steps-simple
sequence-snake-steps-simple-illus
sequence-snake-steps-underline-text
sequence-stairs-front-compact-card
sequence-stairs-front-pill-badge
sequence-stairs-front-simple
sequence-steps-badge-card
sequence-steps-simple
sequence-steps-simple-illus
sequence-timeline-done-list
sequence-timeline-plain-text
sequence-timeline-rounded-rect-node
sequence-timeline-simple
sequence-timeline-simple-illus
sequence-zigzag-pucks-3d-indexed-card
sequence-zigzag-pucks-3d-simple
sequence-zigzag-pucks-3d-underline-text
sequence-zigzag-steps-underline-text
```

## Quick example

```svelte
<script lang="ts">
  import InfographicWrapper from './InfographicWrapper.svelte';

  const syntax = 'infographic sequence-steps-simple';
  const data = `data
    sequences
      - label Plan
      - label Build
      - label Ship`;
</script>

<InfographicWrapper {syntax} {data} />
```
