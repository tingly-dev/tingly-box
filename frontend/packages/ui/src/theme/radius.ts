// Corner radii as MUI `borderRadius` multipliers (theme.shape.borderRadius = 8).
// Three steps, so the same kind of surface has the same corners everywhere:
// cards, panels and dialogs 12px; rows, buttons and inputs 8px; small pills
// and inline code 6px. The theme's Card / Button / input / Chip / Dialog
// overrides use the same steps. `mark` is for data marks only (heatmap cells,
// crop guides) — too small for the chip step to stay square-ish.
export const radius = {
  card: 1.5, // 12px — cards, panels, dialogs
  row: 1, //    8px  — list rows, buttons, inputs, selectors, tabs
  chip: 0.75, // 6px  — pills, badges, inline code, thumbnails
  mark: 0.25, // 2px  — heatmap cells, crop guides, tiny data marks
  pill: 999, //  fully round — capsules, progress bars, scrollbar thumbs (dots use '50%')
} as const;

// The same steps in px, for styles that are not MUI `sx` (chart tooltips,
// canvas, inline `style`), where a bare number means px, not a multiplier.
export const radiusPx = {
  card: 12,
  row: 8,
  chip: 6,
  mark: 2,
  pill: 999,
} as const;
