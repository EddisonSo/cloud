# Instrument · Ice — Dashboard Design System

**Date:** 2026-06-09
**Status:** Approved (direction chosen from rendered studies: Instrument language,
Ice accent, bare-views switcher, trace-style charts)
**Scope:** edd-cloud-interface/frontend — full visual overhaul, zero functional change

## Philosophy

Operate it like equipment. Industrial precision: flat graphite surfaces, hairline
borders, machined mono micro-labels, one pale ice-blue accent. Succinct, bespoke,
and **information-first** — style never costs data.

## Hard rules (professional-grade guarantees)

1. **No information loss.** Every column, count, ID, timestamp, and message the
   current UI shows survives the restyle.
2. **Density up, not down.** Tighter row heights + tabular mono numerals → more
   scannable data per screen.
3. **State is never color-alone.** Status = dot + text label, always.
   Contrast ≥ 4.5:1 for data/body text, ≥ 3:1 for secondary labels.
4. **Charts keep their instruments.** Axes, units, legends, tooltips with exact
   values all stay. The trace style restyles; it never removes.
5. **Affordances stay explicit.** Buttons look like buttons; destructive stays
   red; inputs keep labels, placeholders, and validation text; focus rings,
   hover, and disabled states on everything interactive.

## Tokens (globals.css `@theme`)

```css
/* Instrument dark (default) */
--color-background: #111315;       /* app bg — warm graphite ink */
--color-card:       #16191c;       /* panel */
--color-popover:    #1a1d20;       /* raised panel / hover wash */
--color-border:     #26292d;       /* hairline */
--color-input:      #26292d;
--color-foreground: #e8e6e1;       /* warm off-white text */
--color-muted:      #1a1d20;
--color-muted-foreground: #8b8f94; /* dim */
/* faint (#5b5f64) used inline for micro-labels via muted-foreground/70 */
--color-primary:    #b7d9f2;       /* ICE accent */
--color-primary-foreground: #0e1820;
--color-secondary:  #1d2023;
--color-secondary-foreground: #e8e6e1;
--color-accent:     #1a1d20;       /* hover wash, NOT the brand accent */
--color-accent-foreground: #e8e6e1;
--color-destructive:#e5544b;  --color-destructive-foreground:#16181a;
--color-success:    #3ecf6e;  --color-warning: #e8b43a;
--color-ring:       #b7d9f2;
--radius: 0px;                     /* machined corners — sharp */
--font-sans: "Archivo", system-ui, sans-serif;
--font-mono: "IBM Plex Mono", "JetBrains Mono", monospace;
```

Light theme (`.light`) keeps the same structure on paper-grey: bg `#f4f3f0`,
card `#fbfaf8`, border `#e0ded8`, text `#1d2024`, dim `#6b7077`, ice accent
darkened to `#2e6f9e` for contrast, same semantics. (Toggle keeps working;
dark is canonical.)

## Recurring elements

- **Micro-label**: mono, 10–11px, uppercase, tracking `.2em`, faint color.
  Used for table headers, nav section labels, card titles, breadcrumbs.
- **Data**: anything machine-generated (IDs, images, ports, durations, sizes,
  versions) renders in mono at 12–13px, dim.
- **Status**: 7px dot (+ subtle same-color glow) + mono uppercase label.
  running/ok = success, degraded = warning, failed = destructive,
  stopped/idle = faint (no glow).
- **Accent discipline**: ice appears ONLY at — active nav tick (2px), active
  view `›` tick, primary button fill, focus ring/border, chart trace, link
  hover. Never as large surfaces or washes.
- **Active nav**: 2px ice left-tick + brightened text + `popover` wash.
  Sub-items hang on a 1px hairline rail; active sub-item's rail segment
  becomes a 2px ice tick.
- **View switcher (in-page)**: bare mono uppercase labels with counts —
  `› CONTAINERS 3   SSH KEYS 2` — active gets ice `›` + bright text. No
  underline-tab chrome.
- **Buttons**: primary = ice fill, ink text, mono uppercase 11.5px tracking
  `.14em`, square corners. Secondary/ghost = transparent, hairline border.
  Destructive = red fill, ink text. Hover = brightness, not color shifts.
- **Cards/panels**: flat `card` on `background`, 1px border, NO shadows, NO
  rounded corners, NO gradients/glass/blur.
- **Tables**: hairline outer border; header row = micro-labels with 1px
  bottom border; row dividers use `--line2`-equivalent (border at 60%);
  row hover = popover wash; ~40px row height.
- **Status strip**: thin mono uppercase strip pinned to page/panel bottom
  border for ambient stats (replaces stat-card grids everywhere).
- **Charts (recharts)**: dotted hairline grid; mono 9.5–10px axis ticks; main
  series = 1.5–2px ice line with 12%→0 vertical gradient fill; secondary
  series = dim grey; semantic series use semantic colors; tooltips = panel bg,
  hairline border, mono values; end-of-line live value annotation where cheap.
- **Motion**: restraint — 120–160ms ease-out on hover/focus; one staggered
  fade-slide (≤200ms) on page mount; no springy/bouncy effects.

## Anti-patterns (delete on sight)

Stat-card grids ("Running 3/3 · CPU 4%" tiles), rounded-2xl, drop shadows,
gradient buttons, glassmorphism/backdrop-blur, pill badges with pastel
backgrounds, emoji in UI chrome, skeleton shimmer (use steady faint blocks),
Google-blue `#8ab4f8` anywhere.

## Typography

- Archivo (400/500/600/700/900) replaces Plus Jakarta Sans. Page titles 24–26px
  /700, section headers 14–15px/600, body 13.5–14px.
- IBM Plex Mono (400/500/600) replaces JetBrains Mono in UI (terminal keeps
  JetBrains Mono via xterm config). `font-variant-numeric: tabular-nums` on
  all data tables/metrics.

## Implementation surface

- `src/styles/globals.css` — tokens, fonts, base styles, scrollbars, helpers
- `src/components/ui/*` (19 primitives) — restyle, API-compatible (no prop
  changes; pages must not need logic edits)
- `src/components/layout/*` — Sidebar (tree + ticks + brand `EDD/CLOUD`),
  TopBar, AppLayout, ThemeToggle
- Feature components + 11 pages — conformance sweep: remove stat-card grids
  (fold into status strips or table meta), kill hardcoded colors/radii/shadows,
  apply micro-label/data/mono conventions, switchers → bare views
- recharts usages (health/observability) — trace styling per above

Functional behavior, routes, hooks, and APIs are untouched.
