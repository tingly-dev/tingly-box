// Shared literal shadows. Before this, the same box-shadow string was pasted
// into several files (Switch thumb + version badge, the two auth/loading
// dialogs). Theme-specific shadows still live in theme/components/*.

export const elevation = {
  // Small raised element: switch thumb, status badge.
  raised: '0 2px 4px rgba(0,0,0,0.2)',
  // Floating dialog paper that sits over the page.
  overlay: '0 8px 32px rgba(0,0,0,0.1)',
  // Dark hover tooltip / popover surface.
  popover: '0 4px 12px rgba(0, 0, 0, 0.3)',
} as const;
