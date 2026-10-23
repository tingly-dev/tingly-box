// Corner radii as MUI `borderRadius` multipliers (theme.shape.borderRadius = 8).
// Three steps, so the same kind of surface has the same corners everywhere:
// cards and panels 12px, rows and buttons 8px, small pills and inline code 6px.
export const radius = {
  card: 1.5, // 12px — cards, panels, inputs, dialogs
  row: 1, //    8px  — list rows, buttons, selectors, tabs
  chip: 0.75, // 6px  — pills, badges, inline code
  pill: 999, //  fully round — capsules (dots use '50%')
} as const;
