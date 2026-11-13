---
name: m9m Design System — The Control Room
version: 1.0.0
updated: 2026-11-12
status: normative
source_of_truth: n8n/packages/frontend/@n8n/design-system/src/css
tokens:
  color:
    brand: "oklch(78.07% 0.136 5.02)"
    brand-hover: "oklch(68.08% 0.148 5.02)"
    brand-active: "oklch(59.09% 0.1611 5.09)"
    brand-focus: "oklch(82.05% 0.1071 4.98)"
    accent: "oklch(69.96% 0.202 44.44)"
    secondary: "oklch(54.05% 0.2811 292.85)"
    text-primary: "oklch(20.46% 0 89.88)"
    text-secondary: "oklch(38.67% 0 89.88)"
    text-tertiary: "oklch(55.55% 0 89.88)"
    text-disabled: "oklch(68.3% 0 89.88)"
    surface: "oklch(100% 0 89.88)"
    surface-subtle: "oklch(97.02% 0 89.88)"
    canvas: "hsl(0, 0%, 96%)"
    canvas-dot: "oklch(68.3% 0 89.88)"
    border-subtle: "oklch(0% 0 0 / 0.05)"
    border: "oklch(0% 0 0 / 0.1)"
    border-strong: "oklch(0% 0 0 / 0.2)"
    border-stronger: "oklch(0% 0 0 / 0.3)"
    success-bg: "oklch(98.19% 0.0181 155.83)"
    success-border: "oklch(92.58% 0.0845 155.86)"
    success-text: "oklch(44.68% 0.1187 151.4)"
    warning-bg: "oklch(97.27% 0.0706 103.29)"
    warning-border: "oklch(94.53% 0.1293 101.4)"
    warning-text: "oklch(47.76% 0.1115 60.43)"
    danger-bg: "oklch(97.05% 0.0129 17.38)"
    danger-border: "oklch(88.34% 0.0616 18.39)"
    danger-text: "oklch(44.46% 0.1774 26.79)"
    info-bg: "oklch(97.05% 0.0142 254.6)"
    info-border: "oklch(88.2% 0.0588 253.97)"
    info-text: "oklch(42.42% 0.1982 265.5)"
    node-surface: "oklch(100% 0 89.88)"
    node-surface-dark: "hsl(0, 0%, 17%)"
    ai-highlight-1: "#5b60e8"
    ai-highlight-2: "#aa7bec"
    ai-highlight-3: "#ec7b8e"
  typography:
    sans: "Geist, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif"
    mono: "'Geist Mono', ui-monospace, Menlo, Consolas, 'DejaVu Sans Mono', monospace"
    scale:
      4xs: "0.5rem"
      3xs: "0.625rem"
      2xs: "0.75rem"
      xs: "0.8125rem"
      sm: "0.875rem"
      md: "1rem"
      lg: "1.125rem"
      xl: "1.25rem"
      2xl: "1.75rem"
    weight:
      regular: 450
      medium: 500
      bold: 600
    line-height:
      xs: 1
      sm: 1.25
      md: 1.3
      lg: 1.35
      xl: 1.5
    tracking:
      tightest: "-0.025em"
      tighter: "-0.015em"
      tight: "-0.007em"
      normal: "0"
      wide: "0.025em"
      wider: "0.05em"
      widest: "0.1em"
  spacing:
    5xs: "0.125rem"
    4xs: "0.25rem"
    3xs: "0.375rem"
    2xs: "0.5rem"
    xs: "0.75rem"
    sm: "1rem"
    md: "1.25rem"
    lg: "1.5rem"
    xl: "2rem"
    2xl: "3rem"
    3xl: "4rem"
    4xl: "8rem"
    5xl: "16rem"
  radius:
    4xs: "0.125rem"
    3xs: "0.25rem"
    2xs: "0.375rem"
    xs: "0.5rem"
    sm: "0.75rem"
    md: "0.875rem"
    lg: "1rem"
    xl: "1.125rem"
    2xl: "1.25rem"
    full: "9999px"
    base: "0.5rem"
  shadow:
    2xs: "0 1px oklch(0% 0 0 / 0.1)"
    xs: "0 1px 2px -1px oklch(0% 0 0 / 0.1)"
    sm: "0 4px 6px -1px oklch(0% 0 0 / 0.1), 0 2px 4px -2px oklch(0% 0 0 / 0.1)"
    md: "0 10px 15px -3px oklch(0% 0 0 / 0.1), 0 4px 6px -4px oklch(0% 0 0 / 0.1)"
    xl: "0 20px 25px -5px oklch(0% 0 0 / 0.1), 0 8px 10px -6px oklch(0% 0 0 / 0.1)"
    outline: "0 0 0 1px oklch(0% 0 0 / 0.1)"
  motion:
    duration:
      snappy: 200ms
      base: 400ms
      slow: 1000ms
      slowest: 1500ms
    easing:
      ease-out: "cubic-bezier(0.215, 0.61, 0.355, 1)"
      ease-in: "cubic-bezier(0.55, 0.055, 0.675, 0.19)"
      ease-in-out: "cubic-bezier(0.645, 0.045, 0.355, 1)"
      spring: "cubic-bezier(0.34, 1.56, 0.64, 1)"
      ease-out-quart: "cubic-bezier(0.165, 0.84, 0.44, 1)"
      ease-out-expo: "cubic-bezier(0.19, 1, 0.22, 1)"
  layout:
    nav-rail-collapsed: 42px
    nav-rail-expanded: 200px
    header-height: 65px
    content-max-width: 1280px
    banner-height: 48px
    canvas-grid: 16px
    node-size: "96x96"
    node-radius: 16px
    node-border-width: 1.5px
    node-creator-width: 385px
    ai-panel-width: 400px
    ndv-settings-min-width: 260px
    ndv-resize-grid: 8px
---

# m9m Design System

> **The Control Room** — an instrument, not a dashboard. Every surface exists to let an
> operator read system state at a glance and act without hesitation.

**Voice:** *Same skeleton, sharper instrument.* The layout skeleton stays recognizable to
n8n users — same rail, same header, same canvas proportions, same panel geometry. The
craft is where m9m goes further: a tighter type scale, a real semantic token layer, and
states that behave like calibrated instruments.

**Authority:** n8n's design system is the reference implementation. Where n8n defines a
value, m9m adopts it verbatim. Where n8n leaves a gap, m9m fills it under the rules in
this document.

---

## 1. Design Direction

**Creative North Star — The Control Room**

The editor is a control room, not a website. It is used for hours, under pressure,
often in the dark, by people who already know n8n. That imposes three rules:

1. **Recognizable skeleton.** An n8n operator switching to m9m must not hunt for
   anything. Rail, header, canvas, node creator on the right, NDV as a full-screen
   three-column modal, AI panel docked right — identical geometry to n8n.
2. **Instrument grade.** Color is reserved for state, not decoration. Depth is
   communicated by surface steps and hairline borders, not by stacked drop shadows.
3. **Quieter, sharper.** More whitespace discipline, fewer competing accents, one
   typeface family doing the structural work, and hierarchy carried by weight and
   spacing rather than size inflation.

**Product promise it serves:** *The n8n alternative without the bugs — faster, more
reliable workflow automation.* The UI must feel as dependable as the runtime: no
surprises, no jitter, no decorative motion, no state that cannot be explained.

**What "better" means here (agreed scope):** sharper type hierarchy. Every additional
lift (denser rhythm, refined motion, richer empty states) stays subordinate to n8n
parity — parity first, typography second, everything else later.

---

## 2. Color

### 2.1 Semantic layer

Colors are referenced by **role**, never by hue. Two layers:

- **Primitives** — raw ramps, oklch only (`--color--pink-500`, `--color--neutral-700`).
- **Semantics** — the names the UI actually consumes (`--background--brand`,
  `--text-color`, `--border-color`, `--canvas--color--background`, `--node--color--background`).

m9m's current `main.css` declares a semantic layer that **nothing consumes**. That layer
becomes the contract: every semantic variable must have real consumers, or it is deleted.

### 2.2 Brand — Signal Pink

| Role | Token | Value |
|---|---|---|
| Brand / primary action | `--background--brand` | `oklch(78.07% 0.136 5.02)` |
| Brand hover | `--background--brand--hover` | `oklch(68.08% 0.148 5.02)` |
| Brand active | `--background--brand--active` | `oklch(59.09% 0.1611 5.09)` |
| Brand focus | `--background--brand--focus` | `oklch(82.05% 0.1071 4.98)` |
| Brand disabled | `--background--brand--disabled` | `oklch(92.19% 0 89.88)` |

**Signal Pink** is a single saturated alert color. It marks *actionable intent* — primary
buttons, active nav, selected toggle, focus on brand surfaces. It is never used for
decoration, never for two competing elements on screen, never as a large field.

`--color--primary` (legacy, `oklch(69.96% 0.202 44.44)`) stays as the **accent for
inline emphasis**: links, slider fills, checked controls. It is not a button color.

### 2.3 Graphite Neutral

The neutral ramp is n8n's, renamed for m9m's character. It is a cool, near-achromatic
ramp with a faint magenta undertone (`hue 89.88`) — the reason surfaces feel like
paper under instrument light rather than blue-gray.

| Step | Token | Value | Use |
|---|---|---|---|
| white | `--color--neutral-white` | `oklch(100% 0 89.88)` | Surface |
| 100 | `--color--neutral-100` | `oklch(97.02% 0 89.88)` | Subtle surface, table head |
| 150 | `--color--neutral-150` | `oklch(95.51% 0 89.88)` | Hover on subtle |
| 200 | `--color--neutral-200` | `oklch(92.19% 0 89.88)` | Disabled fill, node selected |
| 300 | `--color--neutral-300` | `oklch(88.53% 0 89.88)` | Stronger divider |
| 400 | `--color--neutral-400` | `oklch(79.21% 0 89.88)` | Icon subtle |
| 500 | `--color--neutral-500` | `oklch(68.3% 0 89.88)` | Icon default, canvas dot |
| 600 | `--color--neutral-600` | `oklch(55.55% 0 89.88)` | Text tertiary |
| 700 | `--color--neutral-700` | `oklch(38.67% 0 89.88)` | Text secondary |
| 800 | `--color--neutral-800` | `oklch(31.71% 0 89.88)` | Icon strong |
| 900 | `--color--neutral-900` | `oklch(26.86% 0 89.88)` | Dark surface |
| 950 | `--color--neutral-950` | `oklch(20.46% 0 89.88)` | Text primary, dark canvas |

Borders are **alpha over the surface**, never solid hues:

| Role | Token | Value |
|---|---|---|
| Border subtle | `--border-color--subtle` | `oklch(0% 0 0 / 0.05)` |
| Border | `--border-color` | `oklch(0% 0 0 / 0.1)` |
| Border strong | `--border-color--strong` | `oklch(0% 0 0 / 0.2)` |
| Border stronger | `--border-color--stronger` | `oklch(0% 0 0 / 0.3)` |

Dark theme mirrors this with white alpha (`oklch(100% 0 89.88 / 0.15)` border,
`/ 0.05` subtle, `/ 0.3` stronger).

### 2.4 Status family

One family, four voices. Background, border, and text are always the same hue at three
different lightness steps — never a mixed-hue alert.

| State | Background | Border | Text |
|---|---|---|---|
| Success | `oklch(98.19% 0.0181 155.83)` (green-50) | `oklch(92.58% 0.0845 155.86)` (green-200) | `oklch(44.68% 0.1187 151.4)` (green-800) |
| Warning | `oklch(97.27% 0.0706 103.29)` (yellow-100) | `oklch(94.53% 0.1293 101.4)` (yellow-200) | `oklch(47.76% 0.1115 60.43)` (yellow-800) |
| Danger | `oklch(97.05% 0.0129 17.38)` (red-50) | `oklch(88.34% 0.0616 18.39)` (red-200) | `oklch(44.46% 0.1774 26.79)` (red-800) |
| Info | `oklch(97.05% 0.0142 254.6)` (blue-50) | `oklch(88.2% 0.0588 253.97)` (blue-200) | `oklch(42.42% 0.1982 265.5)` (blue-800) |

Danger is the only red allowed to use saturated red-500 (`oklch(63.78% 0.2373 25.44)`)
and only as a small indicator dot or icon, never as a large filled surface.

### 2.5 Canvas, nodes, sticky notes

| Role | Light | Dark |
|---|---|---|
| Canvas background | `hsl(0, 0%, 96%)` | `oklch(20.46% 0 89.88)` |
| Canvas dot | `oklch(68.3% 0 89.88)` | `oklch(38.67% 0 89.88)` |
| Node surface | `oklch(100% 0 89.88)` | `hsl(0, 0%, 17%)` |
| Node border | `oklch(0% 0 0 / 0.1)` | `oklch(100% 0 89.88 / 0.15)` |
| Sticky note body text | `oklch(20.46% 0 89.88)` | `oklch(97.02% 0 89.88)` |

Sticky notes use n8n's seven-colour palette and n8n's theme swap: the light
theme paints the pale fills with Graphite body text, the dark theme paints
the deep fills with Neutral body text. The note body always clears WCAG AA
(4.5:1) against its own fill — 12.92:1 … 16.68:1 light, 7.95:1 … 14.93:1 dark.

Light (fill / border): yellow `oklch(97.27% 0.0706 103.29)` /
`oklch(62% 0.1293 101.4)` · gold `oklch(92.41% 0.0463 78.84)` /
`oklch(63% 0.0975 76.95)` · red `oklch(93.65% 0.032 17.74)` /
`oklch(64% 0.0827 18.93)` · green `oklch(96.24% 0.0434 156.74)` /
`oklch(61% 0.149 154.62)` · blue `oklch(93.19% 0.0316 255.59)` /
`oklch(63% 0.0588 253.97)` · purple `oklch(89.46% 0.0562 293.06)` /
`oklch(64% 0.1055 293.7)` · neutral `oklch(97.02% 0 89.88)` /
`oklch(63% 0 89.88)`.

Dark (fill / border): yellow `oklch(42.15% 0.0952 57.94)` /
`oklch(52% 0.1115 57.94)` · gold `oklch(24.57% 0.0436 73.41)` /
`oklch(52% 0.0793 73.41)` · red `oklch(25.75% 0.0918 25.84)` /
`oklch(53% 0.1408 25.84)` · green `oklch(26.54% 0.0651 152.77)` /
`oklch(51% 0.0957 152.28)` · blue `oklch(37.96% 0.1453 265.47)` /
`oklch(53% 0.1982 265.47)` · purple `oklch(28.4% 0.1414 291.29)` /
`oklch(54% 0.232 292.71)` · neutral `oklch(26.86% 0 89.88)` /
`oklch(52% 0 89.88)`.

The borders are one step deeper (light) / lighter (dark) than n8n's own tint:
n8n's hairline only reaches 1.06–1.70:1 against the canvas, m9m's clears
3:1 (WCAG 1.4.11) so the note edge stays visible. A free-form hex note keeps
its colour in both themes and picks Graphite or Neutral text from its own
relative luminance, the way n8n's `getContrastTextColor` does.

Tokens: `--sticky--color--text`, `--sticky--color--text--on-light`,
`--sticky--color--text--on-dark`, `--sticky--color--background--variant-1…7`,
`--sticky--border-color--variant-1…7`.

Node card text uses the node surface, not the canvas: title
`--text-color` (17.93:1 light / 12.93:1 dark), subtitle and parameter text
`--text-color--subtle` (9.74:1 / 9.98:1), icon `--text-color` at 85% opacity
(11.41:1 / 9.77:1). A node's own note ("notes in flow") is painted on the
canvas directly below the card, in `--text-color--subtle` (8.92:1 light /
12.70:1 dark against the canvas).

### 2.6 AI accent

`--assistant--color--highlight-1` `#5b60e8` · `--assistant--color--highlight-2`
`#aa7bec` · `--assistant--color--highlight-3` `#ec7b8e`, applied as
`linear-gradient(105deg, #5b60e8 0%, #aa7bec 50%, #ec7b8e 100%)`. Dark theme:
`#8c90f2` / `#a977f0` / `#f0778b`. This gradient belongs to the assistant surface only.

### 2.7 Node icon palette

n8n assigns color per node type through 27 `--node--icon--color--*` tokens (neutral,
blue `#3a42e9`, orange `#ff965a`, green `mint-700`, violet `#9b6dd5`, and so on).
m9m currently paints nodes with emoji glyphs on a category-colored square — that is the
single largest visual break from n8n. Node color belongs to the **icon**, not to the
card, and never to a category stripe.

### 2.8 Implementation notes

- Replace Tailwind `slate-*` with a `neutral` scale generated from the values above;
  keep `slate` only for legacy third-party overrides.
- Replace `primary-*` (`#0ea5e9` sky) with `brand` = Signal Pink. No element keeps
  sky blue after migration.
- Wire the existing `:root` / `.dark` blocks in `web/src/assets/styles/main.css` to
  these semantic names, or delete them. Dead variables are worse than none.
- Dark-mode borders: one step everywhere. No `slate-700` vs `slate-600` split.

---

## 3. Typography

### 3.1 Families

- Sans: **Geist** (`font-weight: 100 900`, `font-feature-settings: 'ss01' 1`), bundled
  as variable woff2. Fallback: `-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto`.
- Mono: **Geist Mono**, bundled variable woff2. Fallback: `ui-monospace, Menlo, Consolas`.

Inter and JetBrains Mono are retired. Fonts are self-hosted — no Google Fonts request
on first paint.

### 3.2 Scale

| Token | Size | Line-height | Use |
|---|---|---|---|
| `--font-size--4xs` | 8px (`0.5rem`) | 1 | Micro marks only |
| `--font-size--3xs` | 10px (`0.625rem`) | 1.25 | Overline, metric labels |
| `--font-size--2xs` | 12px (`0.75rem`) | 1.25 | Label, badge, tag |
| `--font-size--xs` | 13px (`0.8125rem`) | 1.5 | Helper text, secondary body |
| `--font-size--sm` | 14px (`0.875rem`) | 1.5 | Body default |
| `--font-size--md` | 16px (`1rem`) | 1.3 | Body emphasis, inputs |
| `--font-size--lg` | 18px (`1.125rem`) | 1.35 | Node title, panel title |
| `--font-size--xl` | 20px (`1.25rem`) | 1.35 | Section title, editor header |
| `--font-size--2xl` | 28px (`1.75rem`) | 1.5 | Page display |

Weights: **regular 450**, **medium 500**, **bold 600**. There is no 700. `font-bold`
becomes `font-semibold` everywhere — a 700 in Geist reads heavier than n8n intends and
flattens hierarchy at small sizes.

Tracking scale: `tightest -0.025em` · `tighter -0.015em` · `tight -0.007em` ·
`normal 0` · `wide 0.025em` · `wider 0.05em` · `widest 0.1em`.

### 3.3 Role map

This is m9m's sharper contribution: n8n ships sizes but not roles. Roles are mandatory
in m9m — every text element picks a role, never a raw size.

| Role | Size | Weight | Tracking | Line-height | Used for |
|---|---|---|---|---|---|
| Display | 28px | 600 | `-0.015em` | 1.5 | Page title (Workflows, Settings) |
| Headline | 20px | 600 | `-0.007em` | 1.35 | Editor header, page section |
| Title | 18px | 600 | `-0.007em` | 1.35 | Node title, NDV header, panel title |
| Subtitle | 16px | 500 | 0 | 1.3 | Dialog title, field group heading |
| Body | 14px | 450 | 0 | 1.5 | Default UI text, list rows, buttons |
| Body small | 13px | 450 | 0 | 1.5 | Descriptions, helper text, table cells |
| Label | 12px | 500 | `0.025em` | 1.25 | Field labels, tab labels, badges |
| Overline | 10px | 500 | `0.05em` | 1.25 | Uppercase metric labels, section kickers |
| Code | 13px mono | 400 | `-0.007em` | 1.5 | JSON, parameter values, expressions |

Rules that make hierarchy read as an instrument:

- **One size jump per level.** Display → Headline → Title → Subtitle → Body. No
  skipping steps; no adjacent sizes differing by 1px.
- **Label + Overline are uppercase.** Overline only for non-interactive context
  (metric labels, column kickers). Interactive labels stay sentence case.
- **Weight carries emphasis, not color.** Emphasis = 450 → 600 within one size step.
  Text color stays in the neutral ramp.
- **Numbers align.** Tabular numerals for metrics, counts, durations, IDs.
- **Mono only for code-like content** — JSON, expressions, webhook paths, IDs. Never
  for prose or labels.

### 3.4 Implementation notes

- `@font-face` Geist + Geist Mono variable woff2 in `web/src/assets/styles/`, preloaded.
- Tailwind `fontFamily.sans` / `.mono` point at the new stacks; `fontSize` gains
  `13px` and `28px` steps plus the tracking and line-height scale.
- Every `font-bold` in `web/src/**` → `font-semibold`.
- Replace ad-hoc `text-[10px]` with the `3xs` step; replace `text-[13px]`-style
  arbitrary values with `xs`.

---

## 4. Spacing & Layout

### 4.1 Spacing scale

Thirteen steps, no exceptions: **2, 4, 6, 8, 12, 16, 20, 24, 32, 48, 64, 128, 256** px
(`5xs … 5xl`). Height scale: **8, 12, 16, 20, 24, 28, 32, 36, 40, 48, 56, 64, 96** px.

Tailwind's 4px base is replaced by this scale so 2px and 6px are first-class. No
arbitrary values (`p-[10px]`) outside genuine one-offs.

### 4.2 Global chrome

| Element | Value |
|---|---|
| Nav rail, collapsed | 42px |
| Nav rail, expanded | 200px |
| Header height | 65px |
| Banner height | 48px |
| Content container max width | 1280px |

m9m's current 256px always-on sidebar and 64px header are replaced by the n8n rail
(42px collapsed / 200px expanded) and 65px header.

### 4.3 Canvas geometry

| Element | Value |
|---|---|
| Grid size | 16px (`snap-grid [16, 16]`) |
| Default node | 96 × 96 |
| Configurable node | 256 × 96 |
| Configuration node | 80 × 80 (pill radius) |
| Trigger node corner radius | 36px |
| Node icon (canvas) | 40px (48px for the new icon type) |
| Node icon (list) | 24px (20px legacy) |
| Horizontal node step | 224px (96 + 128 spacing) |
| Default start position | x 176px, y 240px |
| Node creator width | 385px, right-anchored overlay |
| Node creator scrim | `oklch(100% 0 89.88 / 0.8)`, opacity 0.7 |

### 4.4 Panels

| Panel | Geometry |
|---|---|
| Node creator | Fixed right, top 65px, bottom 0, width 385px, scrim over the canvas |
| NDV | Full-screen modal, three resizable columns (Input / Settings / Output), settings min 260px, resize grid 8px, header 42px |
| AI assistant | Right column, 380–425px, default 400px, resizable |
| Execution logs | **Not in the editor.** Logs live in the NDV Output panel |

### 4.5 Breakpoints

600px · 768px · 992px · 1200px · 1920px. The node creator collapses to
`calc(100vw - 42px)` below 427px. The minimap is hidden at ≤767px.

---

## 5. Surfaces & Elevation

### 5.1 Tonal, not shadow-led

m9m today is shadow-led: nodes carry `shadow-md`, hover raises `shadow-lg`, six bespoke
`rgba(15,23,42,…)` values float in `BaseNode.vue`. n8n is tonal: hairline borders and
surface steps, shadows reserved for things that float above the page (dialog, menu,
toast, popover).

**Rule:** a surface that is *in* the layout gets a border. A surface that floats *over*
the layout gets a shadow.

### 5.2 Elevation scale

| Token | Value | Applied to |
|---|---|---|
| `--shadow--2xs` | `0 1px oklch(0% 0 0 / 0.1)` | Hairline lift, chips |
| `--shadow--xs` | `0 1px 2px -1px oklch(0% 0 0 / 0.1)` | Toolbar pill, canvas controls |
| `--shadow--sm` | `0 4px 6px -1px oklch(0% 0 0 / 0.1), 0 2px 4px -2px oklch(0% 0 0 / 0.1)` | Dropdown, popover |
| `--shadow--md` | `0 10px 15px -3px oklch(0% 0 0 / 0.1), 0 4px 6px -4px oklch(0% 0 0 / 0.1)` | Dialog |
| `--shadow--xl` | `0 20px 25px -5px oklch(0% 0 0 / 0.1), 0 8px 10px -6px oklch(0% 0 0 / 0.1)` | Modal |
| `--shadow--outline` | `0 0 0 1px oklch(0% 0 0 / 0.1)` | Resting focus ring |

Dark theme shadow color: `oklch(0% 0 0 / 0.25)`; outline becomes
`oklch(100% 0 89.88 / 0.1)`.

### 5.3 Canvas node

- Radius **16px** (`--radius--lg`), border **1.5px**, **no shadow at rest**.
- Trigger variant: `36px 16px 16px 36px` corners. Configuration variant: pill radius
  (`height / 2`).
- Icon **40px**, left of a single-line title — never an emoji block above the label.
- Selection is a **ring**, not a border change:
  `box-shadow: 0 0 0 calc(6px * var(--canvas-zoom-compensation-factor, 1)) hsla(220, 47%, 30%, 0.1)`.
- Status is encoded in the border: success green, error red, warning yellow, each at
  2px width; running uses an animated conic-gradient border (`border-rotate 1.5s linear
  infinite`), waiting at 4.5s.
- The node toolbar appears above the node on hover/focus, opacity `0 → 1` over 100ms.

### 5.4 Focus

`--focus--border-width: 2px`, `--focus--outline-color: oklch(0% 0 0 / 0.2)` (dark:
`oklch(100% 0 89.88 / 0.3)`), `--focus--border-color: oklch(0% 0 0)`.

Every interactive element uses `:focus-visible`, never bare `:focus`, so a mouse click
does not leave a ring behind. Focus rings are drawn with `outline`, not `box-shadow`, so
they never disturb layout.

---

## 6. Motion

| Token | Value | Use |
|---|---|---|
| `--duration--snappy` | 200ms | Hover, focus, press, dropdown |
| `--duration--base` | 400ms | Panel expand, toast |
| `--duration--slow` | 1000ms | Running-node border rotation |
| `--duration--slowest` | 1500ms | Waiting-state rotation |

Easings: `--easing--ease-out` `cubic-bezier(0.215, 0.61, 0.355, 1)` (default),
`--easing--ease-in` `cubic-bezier(0.55, 0.055, 0.675, 0.19)`,
`--easing--ease-in-out` `cubic-bezier(0.645, 0.045, 0.355, 1)`,
`--easing--spring` `cubic-bezier(0.34, 1.56, 0.64, 1)` (selection, pop-in only),
`--easing--ease-out-quart` `cubic-bezier(0.165, 0.84, 0.44, 1)`,
`--easing--ease-out-expo` `cubic-bezier(0.19, 1, 0.22, 1)`.

Rules:

- Motion explains a state change; it never announces itself.
- 100–200ms for anything the pointer triggered. Nothing under 100ms, nothing over 400ms
  outside deliberate panel transitions.
- Opacity and transform only. Animating `width`, `height`, `top`, or `left` is banned.
- No entrance animation on page load, no looping motion that is not tied to live state,
  no parallax, no decorative easing.
- `prefers-reduced-motion: reduce` collapses every duration to `1ms` and disables the
  running-border rotation to a static border.

---

## 7. Components

Component rules below are n8n's, with m9m's typography discipline applied.

### 7.1 Buttons

- **Primary** — fill `--background--brand`, white text, radius `--radius` (8px), no
  border, 14px/500 label, `px-4 py-2`. Hover → brand-600, active → brand-700.
- **Secondary** — surface fill, `--border-color--strong` border, text `--text-color`.
  Hover → brand-50 tint, active/focus → brand-100.
- **Ghost** — transparent fill, icon color `--icon-color`; hover uses
  `--background--hover` (`oklch(0% 0 0 / 0.05)`).
- **Icon button** — 32px hit target minimum, 16px icon, radius `--radius`.
- Every button has a visible `:focus-visible` ring and a disabled state at 50% opacity
  with `cursor: not-allowed`.
- m9m's `.btn-primary` currently paints sky blue and its variants have **no border** —
  both migrate.

### 7.2 Inputs

- Height 40px (`height--md`), `px-3`, radius `--radius`, 1px
  `--border-color--strong` border, text `--text-color`, placeholder `--text-color--disabled`.
- Focus: 2px `--focus--border-color` border + outline `--focus--outline-color`.
- Error: border `--border-color--danger`, plus an error message in `--text-color--danger`
  — never color alone.
- One shared input class covers text, textarea, and select; checkbox keeps a distinct
  treatment (`--color--primary` when checked).
- Labels: **Label role** (12px/500), required marker in danger.

### 7.3 Tags & chips

- Height 20px, radius `--radius` (8px), padding `0 var(--spacing--4xs)`, 12px font,
  background `--color--neutral-125` (light) / `--color--neutral-900` (dark), border
  `--border-color--subtle`.
- Tag color is applied at 12% tint background / 30% border, never full saturation.

### 7.4 Node creator

- Right-anchored overlay, 385px, scrim at 0.7 opacity over the canvas.
- Search field pinned at top; categories are collapsible rows; node rows show a 20px
  icon + name + one-line description.
- Opens on demand (toolbar button / `Tab`); closed by default. m9m's permanent 288px
  left dock is retired.

### 7.5 NDV (node details view)

- Full-screen modal `<dialog>`, three resizable columns: Input, Settings, Output.
- Settings column minimum 260px, resize grid 8px, header 42px.
- Output panel owns execution logs; the editor has no separate log strip.
- Parameter groups use Title + Label roles; raw parameter editors use the Code role.

### 7.6 AI assistant

- Right column, default 400px (380–425px resizable).
- Header uses the assistant gradient `linear-gradient(105deg, #5b60e8 0%, #aa7bec 50%, #ec7b8e 100%)`.
- User bubble: `--background--brand` fill, white text. Assistant bubble: subtle surface,
  `--text-color`.

### 7.7 Canvas controls & minimap

- Controls: horizontal pill, surface background, 1px border, `--shadow--xs`,
  `--radius--full`, 16px icons, `--spacing--5xs` padding/gap.
- Minimap: 120px tall, `--radius`, `--color--background--light-2` fill, 1px border,
  hidden at ≤767px.
- Canvas background: `hsl(0, 0%, 96%)`, dots `--canvas--dot--color`, gap 16px.

### 7.8 Feedback surfaces

- **Toast**: `--dialog--overlay` scrim-free corner placement, `--shadow--md`, radius
  `--radius`, status-tinted border and title from the status family.
- **Dialog**: `--shadow--xl`, radius `--radius--md`, overlay
  `oklch(100% 0 89.88 / 0.8)`.
- **Tooltip**: a real tooltip component (n8n has one) replaces m9m's native `title=`
  attributes; `--radius`, `--shadow--sm`, 12px text.
- **Table**: header on `--color--background`, rows alternating
  `--color--background--light-3` / `--color--background--light-2`, hover
  `--color--primary--tint-3`.

### 7.9 Shared primitives m9m must add

`Button`, `IconButton`, `Input`/`Select`/`Textarea`/`Checkbox`, `Dialog`, `Tooltip`,
`Tabs`, `Table`, `Tag`, `Toast`, `StatusBadge`. m9m currently hand-rolls icon buttons in
~15 files, has no dialog/tooltip/tabs/table component, and duplicates `.input` across
`ParameterField.vue` and `WorkflowList.vue`.

---

## 8. Do's & Don'ts

### Do

- Do match n8n's geometry exactly: 42/200px rail, 65px header, 16px grid, 96px node,
  16px node radius, 385px right node creator, 3-column NDV, 400px AI panel.
- Do reference colors by semantic role, from the token layer.
- Do pick a typography role before picking a size, and stay one step apart in scale.
- Do use hairline borders and surface steps for in-layout surfaces; shadows only for
  overlays.
- Do encode node state in the border/icon, never by repainting the whole card.
- Do support dark mode by swapping the same semantic tokens, not by hand-picking hexes.
- Do keep one primary action per view.
- Do test both themes for every new surface.

### Don't

- Don't ship a second brand color, a sky-blue accent, or violet→indigo gradients
  outside the assistant.
- Don't use emoji as node icons.
- Don't add a category stripe to nodes; n8n nodes have no stripe.
- Don't use `font-bold` (700) — the scale tops out at 600.
- Don't use 700px-wide cards, pill buttons, or gradients as general decoration.
- Don't stack a shadow + border + ring on the same element; one cue per state.
- Don't animate layout properties, and don't ignore `prefers-reduced-motion`.
- Don't leave dead CSS variables or dead Tailwind colors in the codebase.
- Don't introduce a layout that an n8n operator would have to relearn.

---

## Appendix — Authority map

| Concern | Source of truth |
|---|---|
| Primitives, semantic tokens, themes | `n8n/packages/frontend/@n8n/design-system/src/css/_primitives.scss`, `_tokens.scss` |
| Fonts | `n8n/packages/frontend/@n8n/design-system/src/css/fonts/geist.scss`, `geist-mono.scss` |
| Editor layout constants | `n8n/packages/frontend/editor-ui/src/app/css/_variables.scss`, `_global.scss` |
| Canvas node geometry | `n8n/packages/frontend/editor-ui/src/app/utils/nodeViewUtils.ts` |
| Canvas node visuals | `n8n/packages/frontend/editor-ui/src/components/canvas/render-types/_canvasNodeStyles.scss` |
| Node creator overlay | `n8n/packages/frontend/editor-ui/src/features/shared/nodeCreator/components/NodeCreator.vue` |
| NDV layout | `n8n/packages/frontend/editor-ui/src/features/ndv/shared/views/NodeDetailsView.vue` |
| AI panel width | `n8n/packages/frontend/editor-ui/src/features/ai/assistant/chatPanel.store.ts` |
| m9m implementation | `web/tailwind.config.js`, `web/src/assets/styles/main.css`, `web/src/**` |
