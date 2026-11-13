import type { ThemeOptions } from '@mui/material/styles';
import { fontSans } from './fonts';

// One type scale, 14px-based, three weights (400 / 500 / 600). See
// `.design/visual-system.md` §2.1 for which variant each kind of text uses.
// Body text is primary ink; supporting text (body2, caption) is secondary;
// disabled ink is only for disabled things.
export const baseTypography = (textPrimary: string, textSecondary: string, _textDisabled: string): ThemeOptions['typography'] => ({
  fontFamily: fontSans,
  h1: { fontSize: '1.75rem', lineHeight: 1.25, fontWeight: 600, color: textPrimary },
  h2: { fontSize: '1.5rem', lineHeight: 1.3, fontWeight: 600, color: textPrimary },
  // Page title (PageHeader).
  h3: { fontSize: '1.25rem', lineHeight: 1.4, fontWeight: 600, color: textPrimary },
  // Section title.
  h4: { fontSize: '1rem', lineHeight: 1.5, fontWeight: 600, color: textPrimary },
  h5: { fontSize: '0.9375rem', lineHeight: 1.45, fontWeight: 600, color: textPrimary },
  // Card / row title.
  h6: { fontSize: '0.875rem', lineHeight: 1.45, fontWeight: 600, color: textPrimary },
  subtitle1: { fontSize: '0.875rem', lineHeight: 1.5, fontWeight: 400, color: textSecondary },
  subtitle2: { fontSize: '0.75rem', lineHeight: 1.5, fontWeight: 500, color: textSecondary },
  body1: { fontSize: '0.875rem', lineHeight: 1.6, color: textPrimary },
  body2: { fontSize: '0.8125rem', lineHeight: 1.55, color: textSecondary },
  // Meta text: timestamps, helper lines, table heads. 12px is the floor for
  // anything a user is meant to read.
  caption: { fontSize: '0.75rem', lineHeight: 1.5, color: textSecondary },
  button: { fontSize: '0.8125rem', fontWeight: 500, textTransform: 'none' },
  overline: { fontSize: '0.6875rem', lineHeight: 1.5, fontWeight: 600, letterSpacing: '0.05em', textTransform: 'uppercase', color: textSecondary },
});

export const baseShape: ThemeOptions['shape'] = {
  borderRadius: 8,
};

export const baseComponents: ThemeOptions['components'] = {
  // Native controls (buttons, tabs, list rows, menu items) keep the arrow
  // cursor; a hand cursor is a web-link affordance. Real links (`<a>`,
  // MuiLink) and disabled/drag states set their own cursor and are unaffected.
  MuiButtonBase: {
    styleOverrides: {
      root: {
        cursor: 'default',
      },
    },
  },
  MuiChip: {
    styleOverrides: {
      root: {
        fontWeight: 500,
        borderRadius: 6,
      },
    },
  },
  MuiCssBaseline: {
    styleOverrides: {
      '@keyframes spin': {
        '0%': { transform: 'rotate(0deg)' },
        '100%': { transform: 'rotate(360deg)' },
      },
    },
  },
};
