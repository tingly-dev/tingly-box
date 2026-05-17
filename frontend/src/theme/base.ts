import type { ThemeOptions } from '@mui/material/styles';

export const baseTypography = (textPrimary: string, textSecondary: string, textDisabled: string): ThemeOptions['typography'] => ({
  fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto", "Helvetica", "Arial", sans-serif',
  h1: { fontSize: '2rem', fontWeight: 600, color: textPrimary },
  h2: { fontSize: '1.5rem', fontWeight: 600, color: textPrimary },
  h3: { fontSize: '1.25rem', fontWeight: 600, color: textPrimary },
  h4: { fontSize: '1.125rem', fontWeight: 600, color: textPrimary },
  h5: { fontSize: '1rem', fontWeight: 600, color: textPrimary },
  h6: { fontSize: '0.875rem', fontWeight: 600, color: textPrimary },
  body1: { fontSize: '0.875rem', color: textSecondary },
  body2: { fontSize: '0.75rem', color: textSecondary },
  caption: { fontSize: '0.625rem', color: textDisabled },
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
