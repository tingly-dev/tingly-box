import type { ThemeOptions } from '@mui/material/styles';

export const baseTypography = (textPrimary: string, textSecondary: string, textDisabled: string): ThemeOptions['typography'] => ({
  fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto", "Helvetica", "Arial", sans-serif',
  h1: { fontSize: '2rem', lineHeight: 1.2, fontWeight: 650, color: textPrimary },
  h2: { fontSize: '1.625rem', lineHeight: 1.25, fontWeight: 650, color: textPrimary },
  h3: { fontSize: '1.375rem', lineHeight: 1.3, fontWeight: 650, color: textPrimary },
  h4: { fontSize: '1.125rem', lineHeight: 1.35, fontWeight: 650, color: textPrimary },
  h5: { fontSize: '1rem', lineHeight: 1.4, fontWeight: 650, color: textPrimary },
  h6: { fontSize: '0.875rem', lineHeight: 1.45, fontWeight: 650, color: textPrimary },
  body1: { fontSize: '1rem', lineHeight: 1.6, color: textSecondary },
  body2: { fontSize: '0.875rem', lineHeight: 1.55, color: textSecondary },
  caption: { fontSize: '0.75rem', lineHeight: 1.45, color: textDisabled },
  button: { fontSize: '0.875rem', fontWeight: 600, textTransform: 'none' },
});

export const baseShape: ThemeOptions['shape'] = {
  borderRadius: 8,
};

export const baseComponents: ThemeOptions['components'] = {
  MuiChip: {
    styleOverrides: {
      root: {
        fontWeight: 500,
        borderRadius: 4,
      },
    },
  },
};
