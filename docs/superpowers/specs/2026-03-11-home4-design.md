# home4 Design Spec — Top-Nav with Dropdown Menus

**Date:** 2026-03-11
**Route:** `ChenWeb/web/src/routes/home4/+page.svelte`
**Theme:** My AI Assistant — daily work tool, neutral professional, Option C layout

---

## 1. Goals

- Same goals as home3: polished, professional, efficient for daily work
- Differentiator: horizontal top navigation with hover-triggered multi-level dropdown menus
- Maximises vertical content space — no permanent sidebar consuming horizontal real-estate
- Right context drawer slides in over content (no layout shift)

---

## 2. Visual System

> **Design principle — configurable variables:** All values in this section MUST be expressed as named variables at the top of each `.svelte` file. See home3 spec Section 2 for the full code pattern — home4 uses identical variable names and the same `$derived` pattern keyed off `darkMode`. Hard-coding visual values in markup is forbidden.

### 2a. Colour tokens

Same as home3 Section 2a, plus one additional token for the top nav bar background:

| Token | Variable name | Light | Dark |
|---|---|---|---|
| Nav bar background | `navBarBg` | `#ECEEF2` | `#1A1E2C` |
| (all other tokens) | (same as home3) | — | — |

The `navBarBg` variable is derived in `top-nav.svelte`:

```svelte
  let navBarBg = $derived(darkMode ? '#1A1E2C' : '#ECEEF2'); // top nav bar background
```

### 2b–2d. Typography, layout dimensions, depth, animation tokens

Same as home3 Section 2b–2d, with these home4-specific additions in `+page.svelte` and `top-nav.svelte`:

```svelte
  // --- Layout dimensions (home4-specific) ---
  const HERO_HEADER_HEIGHT = 200;  // hero header height in px — same as home3 for visual consistency
  const NAV_BAR_HEIGHT     = 44;   // top nav bar height in px
  const DRAWER_WIDTH       = 300;  // right context drawer width in px (fixed in home4)
  const CONTENT_OFFSET     = HERO_HEADER_HEIGHT + NAV_BAR_HEIGHT; // used in CSS calc() for sticky offsets

  // --- Dropdown behaviour timings (adjust here to tune menu responsiveness) ---
  const DROPDOWN_SHOW_DELAY = 100; // ms delay before dropdown appears on hover (prevents accidental triggers)
  const DROPDOWN_HIDE_DELAY = 150; // ms delay before dropdown hides after cursor leaves (prevents flicker)

  // --- Drawer animation ---
  const DRAWER_ANIM_DURATION = '250ms'; // right drawer slide-in/out duration
```

---

## 3. Layout

```
┌──────────────────────────────────────────────────────────────┐
│                    Hero Header (200px)                        │
├──────────────────────────────────────────────────────────────┤
│  Dashboard │ Agents ▾ │ Skills ▾ │ Apps ▾ │ ... │ ⚙ 👤       │  ← top nav (44px)
├──────────────────────────────────────────────────────────────┤
│                                                              │
│              Content Area (scrolls)                          │
│              near full-width                                 │
│                                                              │
│              ─── AppFooter ───                               │
└──────────────────────────────────────────────────────────────┘
                                               ▲
                               Right Drawer slides in from here (300px)
```

- Hero header + top nav bar are sticky (don't scroll away)
- Content area fills remaining viewport height, scrolls vertically
- Right drawer overlays the content (position: fixed/absolute right) — no layout shift

---

## 4. Hero Header

**Height:** `HERO_HEADER_HEIGHT` constant (default **200px**) — same height as home3 for visual consistency. Declared at the top of `hero-header.svelte` as a configurable constant.

**Background visual:** Same animated aurora mesh as home3 — soft indigo/violet/cyan drifting colour blobs via CSS `@keyframes`. Full width, more space to breathe vs home3 due to absence of a sidebar.

**Left zone:**
- Logo icon (`BrainCircuit`, accent colour, 28px)
- Wordmark "MyAI**Assistant**" — "MyAI" in `accent`, "Assistant" in `textPrimary`
- Version badge `v4.0` (identifies which variant the user is looking at)
- Below wordmark: tagline "Your intelligent AI workspace"

**Centre zone — hero image:**
- Same inline SVG illustration spec as home3 Section 4 (stylised neural constellation or glowing orb with CSS-animated rings and nodes)
- Identical configurable constants:
  ```svelte
  const HERO_IMAGE_OPACITY = 0.85;   // overall opacity of the centre illustration
  const HERO_IMAGE_ANIMATE = true;   // set false to freeze animation
  ```
- home4 gets slightly more horizontal space for the SVG (no rail on the left), so the illustration can be 10–15% wider

**Right zone:**
- AI model pills: `Claude Sonnet 4.6` + `GPT-4o`
- Activity bell with notification dot
- Dark/light toggle button

**Status strip** (below the three zones, full width, 28px tall, subtle top border — same as home3):
- "3 agents active" • "12 tasks running" • "All systems nominal"

---

## 5. Top Navigation Bar

**Height:** 44px, sticky below the hero header.
**Background:** `Surface 2` colour with 1px bottom border.

### Left section — primary nav items
Items: Dashboard, Agents, Skills, Applications, Coding Assistant, Personal Assistant, Knowledge Base

- **Leaf items** (Dashboard): plain button, no indicator
- **Parent items** (Agents, Skills, etc.): label + small ChevronDown icon

**Active item styling:** accent-coloured 2px underline + accent text colour (not filled background — keeps the bar clean)

**Hover behaviour:**
- On `mouseenter` of a parent item: show dropdown after 100ms (prevents accidental triggers)
- On `mouseleave` of item + dropdown combined bounding box: hide after 150ms delay (prevents flickering when moving mouse toward the dropdown)
- Dropdown stays open while mouse is anywhere inside it

### Dropdown panel
- Appears directly below the nav bar (top: 44px, aligned with triggering item's left edge)
- Background: `Card bg` + box shadow: `0 8px 24px rgba(0,0,0,0.12)`
- Border radius: 12px on all corners, with a 2px gap between the nav bar bottom edge and the dropdown top edge (creates a clean visual separation)
- Min width: 200px

**Dropdown item:**
- Icon (16px, muted) + label (14px)
- Hover: accent tint background (`rgba(99,102,241,0.08)`)
- Active (currently selected): accent text + accent tint background
- Padding: 10px 16px
- Separator lines between logical groups if needed

**Keyboard support:** Arrow keys navigate within open dropdown; Escape closes it; Tab moves to next nav item.

### Right section — utility nav
- Settings icon button (gear) — opens Settings section
- About icon button (info)
- Divider
- User avatar (32px) + three-dots dropdown menu (DropdownMenu.Root, shadcn): User Info, Account, separator, Log Out
- Context shelf toggle button: PanelRight icon, toggles right drawer

### Command bar hint
- Small `⌘K` pill at the far right of the nav bar (before user section)
- Clicking (or pressing ⌘K) opens a command palette modal (search all sections, quick actions)
- Command palette: full-screen overlay with centred input, filtered results list — uses shadcn Command component

**Keyboard shortcut registration:** The `keydown` listener for `⌘K` / `Ctrl+K` is registered in `+page.svelte` on `document`, inside a `$effect` that cleans up on destroy. It fires regardless of which element has focus, except when focus is inside an `<input>`, `<textarea>`, or `contenteditable` element (to avoid interfering with text editing). The listener sets `commandPaletteOpen = true`.

---

## 6. Content Area

Scrollable, `min-height: calc(100vh - 244px)` (= viewport minus `HERO_HEADER_HEIGHT + NAV_BAR_HEIGHT`).

**Width when drawer is closed:** full viewport width minus any scrollbar.
**Width when drawer is open:** the content area itself does NOT reflow — the drawer overlays the content. However, the content area gains `padding-right: 300px` so that readable text is not occluded by the drawer. This means content reflows to avoid the drawer but without a layout jump (the padding transition is animated at the same 250ms as the drawer slide).

### Default: Dashboard
Same four-zone widget layout as home3:
1. **Stats row**: 4 KPI cards (Active Agents, Running Tasks, Skills Loaded, Knowledge Docs)
2. **Activity feed** (left 60%) + **Quick Launch** (right 40%)
3. **Agent status** mini-list (bottom)

Slightly wider cards vs home3 because there is no permanent sidebar.

### Section views
Same section content as home3. Top of each section shows a breadcrumb trail: `Home > Agents > My Agents` using the current nav selection.

### AppFooter
Inside the content scroll area, `margin-top: 48px`, same four-column footer as home3.

---

## 7. Right Drawer (Context Shelf)

- **Width:** 300px (fixed, not resizable in home4 — the overlay model makes drag-resize complex and is not in scope for this iteration)
- **Position:** Fixed, right edge, top: `(HERO_HEADER_HEIGHT + NAV_BAR_HEIGHT) = 244px`, height: `calc(100vh - 244px)`
- **Open/close:** slides in from the right with 250ms ease transition; overlays content
- **Content padding response:** content area animates `padding-right` from 0 to 300px simultaneously (same 250ms) so readable text is not occluded
- **Close button:** × icon in the top-right corner of the drawer
- **Backdrop:** none
- **Toggle:** PanelRight icon button in the top nav bar (right section)
- **Scroll:** independently scrollable within the drawer

Content: same as home3 context shelf — section-aware cards (system health, AI models, upcoming, agent stats, etc.)

---

## 8. Backend REST API

Same endpoints as home3 — shares the same `aiassistanthandler` and `routes.go` registrations. See home3 spec Section 10 for the full table including the new PUT/DELETE stub routes.

home4 uses identical API calls; no additional routes beyond those defined for home3.

---

## 9. File Structure

```
ChenWeb/web/src/routes/home4/
  +page.svelte                   ← root layout, active section state

ChenWeb/web/src/lib/components/home4/
  hero-header.svelte             ← aurora header (taller)
  top-nav.svelte                 ← sticky nav bar with dropdown menus
  dropdown-menu-panel.svelte     ← reusable dropdown panel for nav items
  command-palette.svelte         ← ⌘K modal (shadcn Command)
  content-panel.svelte           ← scrollable main content + footer
  right-drawer.svelte            ← fixed right context drawer
  dashboard-view.svelte          ← dashboard widgets
  app-footer.svelte              ← rich footer
```

---

## 10. State

Managed in `+page.svelte`:

- `darkMode: boolean` — toggled in header
- `activeMenu: ActiveSelection | null` — driven by top-nav
- `drawerOpen: boolean` — right drawer visibility
- `commandPaletteOpen: boolean` — ⌘K modal

Nav hover state (which dropdown is open) is managed locally inside `top-nav.svelte` using timers, not in the root.

### `ActiveSelection` type

```typescript
type ActiveSelection = {
  itemId: string;       // top-level nav id, e.g. 'agents', 'dashboard'
  childId?: string;     // sub-item id, e.g. 'agents-my', 'coding-review'
  itemTitle: string;    // display name of the parent, e.g. 'Agents'
  childTitle?: string;  // display name of the child, e.g. 'My Agents'
};
```

Valid `itemId` values: `'dashboard' | 'agents' | 'skills' | 'applications' | 'coding' | 'personal' | 'knowledge' | 'settings' | 'about'`

Special synthetic `itemId` values from the user dropdown: `'__user_info__' | '__account__' | '__logout__'`

### Section-specific context shelf content (deferred detail)
The context shelf in home4 (same as home3) shows section-aware cards. For this iteration the following are fully specified: dashboard (system health, AI models, upcoming). The following are deferred to a follow-up spec and will show a "coming soon" placeholder card: personal assistant, about. All other sections (agents, skills, applications, coding, knowledge, settings) use the same card structures described in home3 Section 7.
