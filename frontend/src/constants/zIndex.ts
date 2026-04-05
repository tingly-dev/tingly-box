/**
 * Centralized z-index management for the application
 * Higher number = higher z-index (on top)
 */
export const Z_INDEX = {
  // Background layers (lowest)
  background: 0,
  sunlitBackground: 1,

  // Main content layers
  main: 10,
  drawer: 100,

  // Overlay layers
  mobileToggle: 1100,
  modal: 1200,
  popover: 1300,
  tooltip: 1400,

  // Top-most layers
  notification: 9999,
} as const;
