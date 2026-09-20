# DS (DeepSeek) theme palette

## Problem

The `ds` theme's `primary` was deliberately softened from deepseek.com's real
brand blue (`#4D6BFE`) to `#6799FE`, to avoid a harsh neon ring on large
outlined surfaces (card hover borders, etc. — see the comment in
`palettes/ds.ts`). That trade-off went too far: button fills, the active nav
item, selected menu items, and hover borders all read as low-contrast /
hard-to-notice against the theme's soft misty background.

## Options considered

Three directions were compared with an actual palette-swatch + component
mockup page (nav item, buttons, card hover/selected) before picking one:

- **A — brand blue direct**: promote the real `#4D6BFE` to `primary.main`.
  Most faithful to deepseek.com, but risks reintroducing the original neon-ring
  problem on large bordered surfaces.
- **B — same hue, darker states only**: keep `primary.main` at `#6799FE`,
  only raise the opacity of hover/selected/focus/divider tokens (~1.6-2x).
  Lowest risk, but the blue itself still doesn't stand out.
- **C — balanced (chosen)**: move `primary.main` to a more saturated
  mid-point (`#5580FA`, between the old main and the real brand blue) *and*
  raise the state-layer opacities. More presence than B, more restrained
  than A.

## Chosen values (Option C)

```
primary.main   #5580FA   (was #6799FE)
primary.light  #82A3FF   (was #93B4FF)
primary.dark   #4D6BFE   (unchanged — the real brand blue, used for hover/pressed)

secondary.main   #109C8C   (was #14B8A6)
secondary.light  #3FCDBA   (was #5EEAD4)
secondary.dark   #0C7A70   (was #0F9488)
```

State-layer alphas (all keyed off the new primary RGB `85, 128, 250`,
formerly `103, 153, 254`), raised roughly 1.4-1.6x across the board:

| token                        | before | after |
|-------------------------------|--------|-------|
| `action.hover`                | 0.08   | 0.12  |
| `action.selected`              | 0.15   | 0.20  |
| `action.focus`                 | 0.12   | 0.16  |
| `divider`                      | 0.12   | 0.18  |
| card/menu `border`             | 0.15   | 0.26  |
| `borderSoft`                   | 0.10   | 0.16  |
| table head bg / row hover      | 0.06 / 0.04 | 0.09 / 0.06 |
| scrollbar thumb / hover        | 0.25 / 0.40 | 0.32 / 0.48 |

The background gradient (`dsBackgroundGradient` — the misty hero wash) is
untouched; it's independent of the brand-blue accent and wasn't part of the
readability complaint.

## Where this lives

- `frontend/src/theme/palettes/ds.ts` — palette tokens (`dsPrimary*`,
  `dsSecondary*`, `action`, `dashboard`).
- `frontend/src/theme/components/ds.ts` — `dsTokens` (component-level
  border/hover/selected/scrollbar alphas) plus a handful of literal
  `rgba(85, 128, 250, …)` overrides (outlined button, switch track, slider
  thumb glow, linear progress track, toggle button selected).

A live swatch comparison lives on the System settings page (theme section)
so future palette tweaks can be checked in place rather than by eyeballing
hex codes — see `frontend/src/pages/system/System.tsx`.
