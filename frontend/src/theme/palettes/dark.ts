import type { ThemePalette } from '../types';

// Dark-mode surface ladder — aligned with MUI's official dark theme
// (~7% / 12% lightness) but keeps a subtle slate hue instead of pure neutral
// gray.
//   default  #07090f  ~3.5% L  — app background (true near-black)
//   paper    #11141c  ~10%  L  — cards, dialogs, dense surfaces
//   raised   #181c26  ~13%  L  — menus, drawers, popovers (one step up)
export const darkBgDefault = '#07090f';
export const darkBgPaper = '#11141c';
export const darkBgRaised = '#181c26';

// Dark-mode input surface tokens — sit one elevation step above Paper so
// fields are recognizable without relying on the border alone.
export const darkInputBg = 'rgba(255, 255, 255, 0.05)';
export const darkInputBgHover = 'rgba(255, 255, 255, 0.08)';
export const darkInputBorder = 'rgba(255, 255, 255, 0.18)';
export const darkInputBorderHover = 'rgba(255, 255, 255, 0.32)';

export const darkPalette: ThemePalette = {
  mode: 'dark',
  // Primary blue: dark mode shifts one notch up (blue-500 instead of
  // blue-600) so text-variant Buttons reach WCAG AA against Paper while
  // staying visually aligned with the contained-Button gradient.
  primary: {
    main: '#3b82f6',
    light: '#60a5fa',
    dark: '#2563eb',
    contrastText: '#ffffff',
  },
  secondary: {
    main: '#94a3b8',
    light: '#cbd5e1',
    dark: '#475569',
    contrastText: '#ffffff',
  },
  success: {
    main: '#059669',
    light: '#10b981',
    dark: '#047857',
  },
  error: {
    // #dc2626 only reaches ~3.8:1 on Paper #11141c. Use #ef4444 (~4.9:1)
    // so Delete buttons in dialogs and error helper text meet WCAG AA.
    main: '#ef4444',
    light: '#f87171',
    dark: '#dc2626',
  },
  warning: {
    main: '#d97706',
    light: '#f59e0b',
    dark: '#b45309',
  },
  info: {
    main: '#0891b2',
  },
  background: {
    default: darkBgDefault,
    paper: darkBgPaper,
  },
  text: {
    primary: '#f3f5f9',
    secondary: 'rgba(255, 255, 255, 0.72)',
    disabled: 'rgba(255, 255, 255, 0.42)',
  },
  divider: 'rgba(255, 255, 255, 0.12)',
  action: {
    hover: 'rgba(255, 255, 255, 0.08)',
    selected: 'rgba(59, 130, 246, 0.28)',
    disabled: 'rgba(255, 255, 255, 0.05)',
    focus: 'rgba(255, 255, 255, 0.12)',
  },
  dashboard: {
    token: {
      input: { main: '#60A5FA', gradient: 'rgba(96, 165, 250, 0.8)' },
      output: { main: '#34D399', gradient: 'rgba(52, 211, 153, 0.8)' },
      cache: { main: '#94a3b8', gradient: 'rgba(148, 163, 184, 0.7)' },
    },
    chart: {
      grid: 'rgba(255, 255, 255, 0.06)',
      axis: 'rgba(255, 255, 255, 0.18)',
      tooltipBg: '#181c26',
      tooltipBorder: 'rgba(255, 255, 255, 0.12)',
    },
    statCard: {
      boxShadow: '0 2px 4px rgba(0, 0, 0, 0.2)',
      emptyIconBg: 'rgba(148, 163, 184, 0.1)',
    },
  },
  isSunlit: false,
};
