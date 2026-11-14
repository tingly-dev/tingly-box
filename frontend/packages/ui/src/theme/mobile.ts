// Sizing for the phone shell (`.design/mobile-ui.md`). Numbers here, not in
// components, so the phone's touch targets and bars stay one decision.

export const mobile = {
  /** Smallest touch target, px. */
  tapMin: 44,
  /** Primary and docked actions (approve, send), px. */
  tapPrimary: 48,
  /** The bottom tab bar, px, above the safe area. */
  bottomNavHeight: 58,
  /** Top corners of a bottom sheet, as a MUI `borderRadius` multiplier (24px). */
  sheetRadius: 3,
} as const;

/** The home-indicator inset, for anything docked to the bottom edge. */
export const safeAreaBottom = 'env(safe-area-inset-bottom, 0px)';

/** Below MUI's `sm` and a touch pointer: a phone, not a narrow desktop window. */
export const PHONE_QUERY = '(max-width: 599.95px) and (pointer: coarse)';
