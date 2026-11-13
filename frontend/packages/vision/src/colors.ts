// Fixed colours the canvas code paints with. They are pixels on a canvas or in
// an exported image, so they cannot follow the UI theme.

/** Sketch paper. */
export const SKETCH_BACKGROUND = '#ffffff';

// A small, opinionated palette: black for outlines plus a handful of clearly
// distinct hues that a model can name back ("the red circle"). Not a colour
// picker — a picker is a decision the user did not come here to make.
export const SKETCH_COLORS = ['#111827', '#dc2626', '#2563eb', '#16a34a', '#f59e0b', '#9333ea'] as const;

/** Mask brush as the user sees it while painting. */
export const MASK_PAINT_COLOR = '#f43f5e';

/** Solid pixels of the exported mask. Only alpha is read, so the RGB is arbitrary. */
export const MASK_FILL = '#000000';

/** What transparent pixels are flattened onto when an export has no alpha (video, picture prep). */
export const EXPORT_BACKGROUND = '#ffffff';
