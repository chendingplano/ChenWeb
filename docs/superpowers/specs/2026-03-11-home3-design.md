# home3 Design Spec — Icon Rail + Popout Sidebar

**Date:** 2026-03-11
**Route:** `ChenWeb/web/src/routes/home3/+page.svelte`
**Theme:** My AI Assistant — daily work tool, neutral professional, Option B layout

---

## 1. Goals

- First page customers see; must be visually polished and immediately trustworthy
- Daily driver: work efficiency and ease of use are as important as aesthetics
- Support light and dark mode, both equally refined
- Neutral professional aesthetic (Notion/Linear quality level), NOT marketing/sales

---

## 2. Visual System

> **Design principle — configurable variables:** All values in this section MUST be expressed as named variables at the top of each `.svelte` file that uses them, with a one-line comment explaining each variable's purpose. Hard-coding visual values in component markup is forbidden. This makes it easy for developers to adjust the look and feel without hunting through templates.

### 2a. Colour tokens (per dark/light mode)

| Token | Variable name | Light | Dark |
|---|---|---|---|
| Page background | `pageBg` | `#F2F4F7` | `#171B26` |
| Card surface | `cardBg` | `#FFFFFF` | `#1F2333` |
| Secondary surface | `surface2` | `#ECEEF2` | `#252A3A` |
| Border / divider | `borderColor` | `#E4E6EB` | `#2D3348` |
| Primary accent | `accent` | `#6366F1` | `#818CF8` |
| Accent on hover | `accentHover` | `#4F46E5` | `#A5B4FC` |
| Accent at 10% opacity (tint) | `accentTint` | `rgba(99,102,241,0.10)` | `rgba(129,140,248,0.15)` |
| Primary text | `textPrimary` | `#111827` | `#E2E8F0` |
| Secondary text | `textSecondary` | `#6B7280` | `#94A3B8` |
| Muted / disabled text | `textMuted` | `#9CA3AF` | `#64748B` |
| Success colour | `colorSuccess` | `#10B981` | `#34D399` |
| Warning colour | `colorWarning` | `#F59E0B` | `#FBBF24` |
| Danger colour | `colorDanger` | `#EF4444` | `#F87171` |

Each component derives its colour variables via a `$derived` block at the top of its `<script>` section, keyed off the `darkMode` prop:

```svelte
<script lang="ts">
  // --- Design tokens (adjust here to restyle the component) ---
  let pageBg      = $derived(darkMode ? '#171B26'  : '#F2F4F7');  // page background
  let cardBg      = $derived(darkMode ? '#1F2333'  : '#FFFFFF');  // card surface
  let surface2    = $derived(darkMode ? '#252A3A'  : '#ECEEF2');  // secondary surface / sidebar
  let borderColor = $derived(darkMode ? '#2D3348'  : '#E4E6EB');  // border / divider lines
  let accent      = $derived(darkMode ? '#818CF8'  : '#6366F1');  // primary accent (indigo)
  let accentHover = $derived(darkMode ? '#A5B4FC'  : '#4F46E5');  // accent on hover
  let accentTint  = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)'); // subtle accent fill
  let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827'); // headings and labels
  let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280'); // body text
  let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF'); // placeholder / disabled
  let colorSuccess  = $derived(darkMode ? '#34D399' : '#10B981'); // success states
  let colorWarning  = $derived(darkMode ? '#FBBF24' : '#F59E0B'); // warning states
  let colorDanger   = $derived(darkMode ? '#F87171' : '#EF4444'); // error / danger states
</script>
```

### 2b. Typography tokens

Declared once in the root `+page.svelte` and passed to child components or declared locally. Each component that uses fonts declares them as constants at the top of its `<script>`:

```svelte
  // --- Typography (adjust here to change fonts) ---
  const fontUI   = "system-ui, -apple-system, 'Inter', sans-serif"; // all UI text
  const fontMono = "'Fira Code', 'Cascadia Code', monospace";        // data, code, version badges
```

Type scale (px): 11 (label), 12 (small), 13 (body-sm), 14 (body), 16 (title), 20 (heading), 24 (hero).
Weights: 400 (body), 500 (medium), 600 (semibold), 700 (bold).

### 2c. Layout dimension tokens

Declared in `+page.svelte` as reactive state so they can be changed at runtime (drag-resize) and at design-time:

```svelte
  // --- Layout dimensions (adjust here to change panel sizes) ---
  const HERO_HEADER_HEIGHT  = 200;  // hero header height in px — increase for more visual impact
  const RAIL_WIDTH_COLLAPSED = 56;  // collapsed icon-rail width in px
  const RAIL_WIDTH_DEFAULT   = 240; // default expanded rail width in px
  const RAIL_WIDTH_MIN       = 180; // minimum draggable rail width in px
  const RAIL_WIDTH_MAX       = 320; // maximum draggable rail width in px
  const SHELF_WIDTH_DEFAULT  = 280; // default context shelf width in px
  const SHELF_WIDTH_MIN      = 220; // minimum draggable shelf width in px
  const SHELF_WIDTH_MAX      = 480; // maximum draggable shelf width in px
```

### 2d. Depth and animation tokens

```svelte
  // --- Shadows (adjust here to change depth/elevation) ---
  const shadowCard      = '0 1px 3px rgba(0,0,0,0.08), 0 1px 2px rgba(0,0,0,0.04)'; // resting card
  const shadowCardDark  = '0 1px 3px rgba(0,0,0,0.30)';                              // resting card (dark)
  const shadowHover     = '0 4px 12px rgba(0,0,0,0.10)';                             // card on hover
  const shadowDropdown  = '0 8px 24px rgba(0,0,0,0.12)';                             // floating panels

  // --- Border radii (adjust here to change roundedness) ---
  const radiusCard   = '12px'; // cards, panels
  const radiusButton = '8px';  // buttons, inputs, tags
  const radiusChip   = '6px';  // small status chips, badges

  // --- Animation durations (adjust here to change transition feel) ---
  const durationHover  = '150ms'; // hover colour transitions
  const durationPanel  = '200ms'; // panel expand/collapse (rail, shelf)
  const durationAurora = '18s';   // aurora blob drift cycle (slower = calmer)
```

---

## 3. Layout

```
┌──────────────────────────────────────────────────────────────┐
│                        Hero Header (200px)                    │
├────┬─────────────────────────────────────────┬───────────────┤
│    │                                         │               │
│Rail│         Content Panel (scrolls)         │ Context Shelf │
│    │         min-height: viewport - header   │  (280px,      │
│    │                                         │   toggleable) │
│    │    ─── AppFooter (inside scroll) ───    │               │
└────┴─────────────────────────────────────────┴───────────────┘
 56px (collapsed) / 240px (expanded)
```

- Full-page height, `overflow: hidden` on outer container
- Content panel and Context Shelf scroll independently
- Rail and hero header are sticky / fixed height

---

## 4. Hero Header

**Height:** `HERO_HEADER_HEIGHT` constant (default **200px**). Controlled by the constant at the top of `hero-header.svelte` — increase for more visual presence, decrease to reclaim vertical space.

**Background visual:** Animated aurora mesh — two or three soft colour blobs (indigo, violet, cyan) that drift slowly across the header using CSS `@keyframes`. Very subtle in light mode (pastel washes over `#F2F4F7`); richer saturation in dark mode. No wire diagrams, no particle systems, no `.gif` files.

**Left zone:**
- Logo icon (`BrainCircuit`, accent colour, 28px)
- Wordmark "MyAI**Assistant**" — "MyAI" in `accent`, "Assistant" in `textPrimary`
- Version badge: `v3.0` small rounded pill (accent tint background)
- Below wordmark: tagline "Your intelligent AI workspace"

**Centre zone — hero image:**
- A visually prominent AI/tech-themed **inline SVG illustration** occupying roughly the centre third of the header width, vertically centred
- The SVG depicts abstract futuristic elements: a stylised neural constellation (nodes + arcs), orbiting rings, or a glowing orb — designed to be inspiring and "techy" without being clichéd
- The SVG uses **CSS animations** (`@keyframes`) for subtle motion: slow rotation of outer rings, gentle opacity pulse on nodes, drifting arc paths
- **No `.gif`, no external image files, no Lottie, no canvas** — pure inline SVG with `<style>` animations
- The SVG adapts its colour palette to `darkMode` via Svelte reactive attributes on `fill` and `stroke`
- Two configurable constants at the top of `hero-header.svelte`:
  ```svelte
  const HERO_IMAGE_OPACITY = 0.85;   // overall opacity of the centre illustration (0–1)
  const HERO_IMAGE_ANIMATE = true;   // set false to freeze animation (e.g. for reduced-motion users)
  ```

**Right zone:**
- AI model chips: `Claude Sonnet 4.6` + `GPT-4o` (small monospace pills)
- Activity bell icon with notification dot
- Dark/light toggle button

**Status strip** (below the three zones, full width, 28px tall, subtle top border):
- "3 agents active" • "12 tasks running" • "All systems nominal"
- Keeps the three-zone layout uncluttered while preserving live status visibility

---

## 5. Navigation Rail (Left Panel)

### Collapsed state (56px)
- Only icons, vertically centred in 40px hit targets
- Tooltip on hover (item name, 150ms delay)
- Active item: filled indigo pill background (40×40), icon white
- Pin button (ChevronRight) at the very top

### Expanded state (240px, pinned or hover-triggered)
- Icon (20px) + label side by side, 12px gap
- Parent items with children: ChevronRight that rotates to 90° when open
- Sub-items: indented 16px, smaller text (13px), dot prefix
- Expansion/collapse: smooth 200ms CSS transition on width + opacity

### Item groups
**Main nav (top):**
- Dashboard (leaf)
- Agents → My Agents, Agent Store, Agent Builder
- Skills → My Skills, Skill Store, Create Skill
- Applications → Installed Apps, Marketplace, Build App
- Coding Assistant → Code Review, Code Generation, Debug Assistant, Refactoring
- Personal Assistant → Daily Briefing, Tasks & Calendar, Email Assistant, Meeting Notes
- Knowledge Base → Documents, Search, Import Sources, Graph View

**Separator**

**Bottom nav:**
- Settings → General, AI Models, Integrations, Security
- About (leaf)

### User section (bottom of rail)
- Collapsed: avatar only (32px rounded-lg)
- Expanded: avatar + name + email (truncated) + three-dots button (MoreHorizontal icon)
- Three-dots dropdown (shadcn DropdownMenu, side="top"): User Info, Account, separator, Log Out

---

## 6. Content Panel

Scrollable, `flex-1`, `min-width: 0`.

### Default: Dashboard
Four-zone widget layout:
1. **Stats row** (top): 4 KPI cards in a grid — Active Agents, Running Tasks, Skills Loaded, Knowledge Docs. Each card: large number, label, small trend indicator.
2. **Activity feed** (left, 60%): Recent events list with type icon, message, timestamp, and status badge (success/warning/info). Scrollable if long.
3. **Quick Launch** (right, 40%): Grid of 6 quick-access buttons for the most-used sections (agents, skills, coding, knowledge, personal, settings).
4. **Agent status** (bottom): Mini horizontal list of agents with name, model, and status dot.

### Section views (one per nav item)
Each section has:
- Page header: title (20px semibold) + subtitle + primary action button (CTA aligned right)
- Content specific to the section (cards, lists, tables as appropriate)
- Proper data, not just Lorem Ipsum placeholders

Sections:
- **Agents**: card grid, each card has agent name, model badge, status pill, description, action buttons (Run / Edit / Delete)
- **Skills**: card grid with icon, name, category badge, usage count
- **Applications**: card grid with app icon, name, category, connection status toggle
- **Coding Assistant**: code editor area placeholder + sub-section tabs (Review / Generate / Debug / Refactor)
- **Personal Assistant**: task list with checkboxes + calendar mini-widget
- **Knowledge Base**: search bar + document table (name, size, date, type icon)
- **Settings**: grouped setting rows per sub-section
- **About**: version info card

### AppFooter
Inside the content scroll area, below all content, with `margin-top: 48px`.

---

## 7. Context Shelf (Right Panel)

**Width:** 280px. Toggled by a ChevronLeft/Right button in the top-right of the content area.
**Collapse animation:** 200ms width transition.
**Scroll:** independently scrollable.

Content varies by active section. Dashboard default:
- **System Health card**: CPU / Memory / API quota progress bars
- **AI Models card**: per-model status dot, name, latency, usage %
- **Upcoming card**: next 3 calendar items with time + label

Section-specific additions (agents → agent stats 2×2 grid; coding → code metrics; knowledge → KB stats).

---

## 8. Resizable Dividers

- Left divider: between rail and content panel — only visible and draggable when `railPinned === true`. Hidden when rail is collapsed (56px icon mode).
- Right divider: between content panel and context shelf — only visible when `shelfOpen === true`
- On hover: highlight accent colour at 30% opacity, show 4 vertically stacked grab dots
- Drag logic: mousedown → mousemove → mouseup on document
- Rail resize range: min 180px, max 320px (applies only when `railPinned === true`)
- Right panel resize range: min 220px, max 480px

**Pin/unpin behaviour:** When the rail is unpinned (collapses to 56px), `railWidth` is preserved at its last dragged value. Re-pinning restores that saved width — the user's panel sizing preference is remembered for the duration of the session.

---

## 9. AppFooter

Four link columns: **Product**, **Resources**, **Support**, **Legal** (5 links each).
Left: brand logo + tagline + system status mini-widget.
Bottom bar: copyright + "Built with SvelteKit + Go + Claude API" + version + GitHub link.

---

## 10. Backend REST API

Reuse existing `aiassistanthandler` routes under `/api/v1/ai-assistant/`:

| Method | Path | Handler | Purpose |
|---|---|---|---|
| GET | `/api/v1/ai-assistant/dashboard` | `GetDashboard` | Dashboard KPIs + activity |
| GET | `/api/v1/ai-assistant/agents` | `GetAgents` | Agent list |
| POST | `/api/v1/ai-assistant/agents` | `CreateAgent` | Create agent |
| PUT | `/api/v1/ai-assistant/agents/:id` | `UpdateAgent` | Update agent (stub) |
| DELETE | `/api/v1/ai-assistant/agents/:id` | `DeleteAgent` | Delete agent (stub) |
| GET | `/api/v1/ai-assistant/skills` | `GetSkills` | Skill list |
| POST | `/api/v1/ai-assistant/skills` | `CreateSkill` | Create skill (stub) |
| DELETE | `/api/v1/ai-assistant/skills/:id` | `DeleteSkill` | Delete skill (stub) |
| GET | `/api/v1/ai-assistant/applications` | `GetApplications` | App integrations |
| GET | `/api/v1/ai-assistant/knowledge-base` | `GetKnowledgeBase` | KB metadata + docs |
| POST | `/api/v1/ai-assistant/knowledge-base/documents` | `ImportDocument` | Import document (stub) |
| DELETE | `/api/v1/ai-assistant/knowledge-base/documents/:id` | `DeleteDocument` | Delete document (stub) |
| GET | `/api/v1/ai-assistant/user-info` | `GetUserInfo` | Logged-in user |
| GET | `/api/v1/ai-assistant/settings` | `GetSettings` | Settings |
| PUT | `/api/v1/ai-assistant/settings` | `UpdateSettings` | Save settings |

All handlers are stubs. The existing stubs in `aiassistanthandler/handler.go` cover the GET/POST routes. The new PUT/DELETE routes need stub handlers added to the same file and registered in `routes.go`.

**Note on UI action buttons (Run / Edit / Delete on agent cards):** The Edit and Delete buttons are wired to the `PUT /agents/:id` and `DELETE /agents/:id` endpoints respectively. In this iteration the handlers return stub responses. The Run button is frontend-only (no API call yet).

---

## 11. File Structure

```
ChenWeb/web/src/routes/home3/
  +page.svelte                   ← root layout, state, drag logic

ChenWeb/web/src/lib/components/home3/
  hero-header.svelte             ← aurora header
  nav-rail.svelte                ← collapsed/expanded sidebar
  content-panel.svelte           ← scrollable main content + footer
  context-shelf.svelte           ← right panel
  dashboard-view.svelte          ← dashboard widgets
  app-footer.svelte              ← rich footer
```

---

## 12. State

Managed in `+page.svelte`, passed as props:

- `darkMode: boolean` — toggled in header
- `activeMenu: ActiveSelection | null` — driven by nav-rail
- `railPinned: boolean` — pin/unpin rail
- `shelfOpen: boolean` — context shelf visibility
- `railWidth: number` — saved expanded width (default 240, range 180–320)
- `shelfWidth: number` — saved shelf width (default 280, range 220–480)

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

Special synthetic `itemId` values dispatched from the user dropdown: `'__user_info__' | '__account__' | '__logout__'`
