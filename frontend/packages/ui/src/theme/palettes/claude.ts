import type { ThemePalette } from '../types';
import type { ThemeTokens } from '../tokens';

// Claude theme color tokens — sourced from Anthropic brand guidelines:
//   Dark #141413 · Light #FAF9F5 · Mid Gray #B0AEA5 · Light Gray #E8E6DC
//   Orange #D97757 (primary accent) · Blue #6A9BCC · Green #788C5D
export const claudePrimary = '#D97757';
export const claudePrimaryLight = '#E89572';
export const claudePrimaryDark = '#C26146';

export const claudePalette: ThemePalette = {
  mode: 'light',
  primary: {
    main: claudePrimary,
    light: claudePrimaryLight,
    dark: claudePrimaryDark,
    contrastText: '#ffffff',
  },
  secondary: {
    main: '#B0AEA5',
    light: '#C7C5BC',
    dark: '#8E8C84',
    contrastText: '#141413',
  },
  success: {
    main: '#788C5D',
    light: '#94A87A',
    dark: '#5E7047',
  },
  error: {
    main: '#BF4434',
    light: '#D26659',
    dark: '#9A3527',
  },
  warning: {
    main: '#C2873B',
    light: '#DDA45A',
    dark: '#9A6A2A',
  },
  info: {
    main: '#6A9BCC',
  },
  background: {
    default: '#FAF9F5',
    paper: '#FFFFFF',
  },
  text: {
    primary: '#141413',
    secondary: '#6B6863',
    disabled: '#B0AEA5',
  },
  divider: '#E8E6DC',
  action: {
    hover: '#F2EFE3',
    selected: '#E8E6DC',
    // `disabled` is the *foreground* MUI uses for disabled Button/IconButton/
    // Chip text — reuse `text.disabled` above so labels stay legible, rather
    // than the near-background tint this used to hold. That tint moved to
    // `disabledBackground`, the token MUI actually reads for a disabled
    // contained Button's fill. See `.design/theme.md`.
    disabled: '#B0AEA5',
    disabledBackground: '#F2EFE3',
    focus: '#E8E6DC',
  },
  dashboard: {
    token: {
      input: { main: claudePrimary, gradient: 'rgba(217, 119, 87, 0.8)' },
      output: { main: '#788C5D', gradient: 'rgba(120, 140, 93, 0.8)' },
      cache: { main: '#B0AEA5', gradient: 'rgba(176, 174, 165, 0.7)' },
    },
    chart: {
      grid: '#F2EFE3',
      axis: '#E8E6DC',
      tooltipBg: '#FFFFFF',
      tooltipBorder: '#E8E6DC',
    },
    statCard: {
      boxShadow: '0 2px 4px rgba(20, 20, 19, 0.08)',
      emptyIconBg: 'rgba(217, 119, 87, 0.1)',
    },
  },
};

export const claudeTokens: ThemeTokens = {
  accent: claudePrimary,
  text: '#141413',
  textMuted: '#6B6863',
  textHelper: '#6B6863',
  placeholder: '#B0AEA5',
  error: '#BF4434',
  surface: {
    card: '#FFFFFF',
    appBar: '#FAF9F5',
    menu: '#FFFFFF',
    popover: '#FFFFFF',
    dialog: '#FFFFFF',
    tooltip: '#FFFFFF',
  },
  border: {
    card: '#E8E6DC',
    chrome: '#E8E6DC',
    menu: '#E8E6DC',
    tooltip: '#E8E6DC',
    divider: '#E8E6DC',
  },
  shadow: {
    menu: '0 10px 24px rgba(20, 20, 19, 0.08)',
    tooltip: '0 4px 12px rgba(20, 20, 19, 0.08)',
  },
  input: {
    bg: 'transparent',
    border: '#D6D3C7',
    hoverBorder: '#B0AEA5',
    disabledBorder: '#E8E6DC',
    filledBg: 'rgba(20, 20, 19, 0.04)',
    filledHoverBg: 'rgba(20, 20, 19, 0.06)',
    filledFocusBg: 'rgba(20, 20, 19, 0.06)',
  },
  button: {
    outlinedBorder: '#D6D3C7',
    outlinedText: '#141413',
    outlinedHoverBorder: '#B0AEA5',
    outlinedHoverBg: '#F2EFE3',
  },
  menuItem: {
    hover: '#F2EFE3',
    selected: '#E8E6DC',
    selectedHover: '#DCD9CB',
  },
};
