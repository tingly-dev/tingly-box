// Accent used by the routing graph and the model cards that mirror it.
// It was defined twice (nodes/styles.tsx and model-select/cardStyles.ts) with
// the same light values; the dark-mode steps differ on purpose and are kept
// as separate names here.

export const routeGraphColors = {
  active: '#4F6F9F',
  activeBg: '#F7F9FC',
  // dark mode, graph nodes
  activeDark: '#D4E3FF',
  controlFillDark: '#4F6F9F',
  controlFillHoverDark: '#5F82BA',
  // dark mode, model cards
  modelCardActiveDark: '#8EA7CF',
} as const;

// Drop shadows of the active node and card. Light themes use a cool slate
// shadow, dark ones a heavier black; the ring around them is built from the
// accent at the call site.
export const routeGraphShadows = {
  nodeLift: ['0 14px 34px rgba(31, 41, 55, 0.14)', '0 3px 10px rgba(31, 41, 55, 0.08)'],
  nodeLiftDark: '0 18px 38px rgba(0, 0, 0, 0.50)',
  cardSelected: '0 8px 24px rgba(31, 41, 55, 0.10)',
  cardSelectedDark: '0 14px 30px rgba(0, 0, 0, 0.34)',
  cardHover: '0 8px 24px rgba(31, 41, 55, 0.08)',
  cardHoverDark: '0 12px 24px rgba(0, 0, 0, 0.30)',
} as const;

// Scroll tracks inside graph nodes.
export const nodeScrollbar = {
  light: { thumb: '#ccc', thumbHover: '#999' },
  dark: { thumb: '#555', thumbHover: '#777' },
} as const;
