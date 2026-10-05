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
