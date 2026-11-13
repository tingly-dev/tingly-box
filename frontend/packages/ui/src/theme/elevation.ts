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
  // A card lifted off the page (the `elevated` UnifiedCard).
  lifted: '0 8px 24px rgba(15, 23, 42, 0.10)',
  // Barely-there lift for a focused composer.
  faint: '0 2px 12px rgba(0, 0, 0, 0.04)',
} as const;
