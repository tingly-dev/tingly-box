// Fixed colours of the figure drawing. Figures are rendered onto a canvas that
// is exported to the image model, so they cannot follow the UI theme.

/** Greys for the figures in a scene: distinct enough that overlapping people stay countable. */
export const FIGURE_SHADE_BODIES = ['#c3c7cc', '#a4aab1', '#d8dce1'] as const;

/** The selected figure's cool tint. */
export const SELECTED_BODY = '#b3bccf';

/** Outline of a flat-rendered figure. */
export const FIGURE_OUTLINE = '#5b6066';

/** Interaction handles (overlay only, never exported). */
export const HANDLE_FILL = '#2563eb';
export const HANDLE_STROKE = '#ffffff';
