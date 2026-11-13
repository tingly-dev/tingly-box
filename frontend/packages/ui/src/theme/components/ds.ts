import type { ThemeOptions } from '@mui/material/styles';
import { dsPrimary, dsPrimaryLight, dsPrimaryDark, dsBackgroundGradient } from '../palettes/ds';

// What the DeepSeek theme adds on top of the shared components (factory.ts):
// the misty page background, brand-blue controls and translucent alerts.
// Everything that is only a colour lives in `dsTokens` instead.
export const dsExtraComponents: ThemeOptions['components'] = {
  MuiAlert: {
    styleOverrides: {
      root: {
        borderRadius: 6,
        backgroundColor: 'rgba(255, 255, 255, 0.94)',
      },
    },
  },
  MuiSwitch: {
    styleOverrides: {
      switchBase: {
        '&.Mui-checked': {
          color: dsPrimary,
          '& + .MuiSwitch-track': {
            backgroundColor: dsPrimary,
            opacity: 0.6,
          },
        },
      },
      track: {
        backgroundColor: 'rgba(85, 128, 250, 0.35)',
      },
    },
  },
  MuiSlider: {
    styleOverrides: {
      root: { color: dsPrimary },
      thumb: {
        '&:hover, &.Mui-focusVisible': {
          boxShadow: '0 0 0 8px rgba(85, 128, 250, 0.22)',
        },
      },
      track: {
        background: `linear-gradient(90deg, ${dsPrimaryLight} 0%, ${dsPrimary} 100%)`,
      },
    },
  },
  MuiLinearProgress: {
    styleOverrides: {
      root: {
        backgroundColor: 'rgba(85, 128, 250, 0.2)',
      },
      bar: {
        background: `linear-gradient(90deg, ${dsPrimaryLight} 0%, ${dsPrimary} 100%)`,
      },
    },
  },
  MuiCircularProgress: {
    styleOverrides: {
      root: { color: dsPrimary },
    },
  },
  MuiToggleButton: {
    styleOverrides: {
      root: {
        borderColor: 'rgba(85, 128, 250, 0.32)',
        '&.Mui-selected': {
          backgroundColor: 'rgba(85, 128, 250, 0.2)',
          color: dsPrimary,
          '&:hover': { backgroundColor: 'rgba(85, 128, 250, 0.28)' },
        },
      },
    },
  },
  MuiBadge: {
    styleOverrides: {
      badge: {
        background: `linear-gradient(135deg, ${dsPrimary} 0%, ${dsPrimaryDark} 100%)`,
      },
    },
  },
  MuiSkeleton: {
    styleOverrides: {
      root: {
        backgroundColor: 'rgba(85, 128, 250, 0.12)',
      },
    },
  },
  MuiCssBaseline: {
    styleOverrides: {
      html: {
        minHeight: '100%',
      },
      body: {
        minHeight: '100vh',
        // The deepseek.com-style misty background this theme is named for —
        // painted on <body> since every surface above it (Paper/Card/Drawer/
        // AppBar) is translucent, unlike the other themes' opaque ones.
        //
        // deepseek.com's real hero is a near-white page (`dsBackgroundGradient.base`)
        // with an animated <canvas> "flow field" drawing moving cloud wisps in
        // `dsBackgroundGradient.wash`/`.accent`/white on top. We can't reasonably run
        // that canvas behind every dashboard/table page here, so this approximates a
        // freeze-frame of it: several large, softly-overlapping radial blobs — a
        // deeper saturated patch top-right, broad pale washes top-left and center,
        // a brighter near-white highlight breaking through the middle, like the
        // real hero's blurred, uneven cloud light rather than one flat gradient band.
        // Every stop is a percentage, so it scales with the viewport (fixed-attached,
        // so percentages resolve against it) instead of a fixed-size image that would
        // stretch or tile awkwardly at very wide or narrow widths.
        backgroundColor: dsBackgroundGradient.base,
        backgroundImage: [
          'radial-gradient(42% 45% at 90% 4%, rgba(122, 157, 220, 0.5) 0%, rgba(122, 157, 220, 0) 70%)',
          'radial-gradient(55% 42% at 46% 6%, rgba(255, 255, 255, 0.6) 0%, rgba(255, 255, 255, 0) 68%)',
          'radial-gradient(60% 50% at 12% -6%, rgba(156, 193, 231, 0.5) 0%, rgba(156, 193, 231, 0) 72%)',
          'radial-gradient(50% 45% at 76% 32%, rgba(138, 163, 214, 0.35) 0%, rgba(138, 163, 214, 0) 75%)',
          'radial-gradient(55% 40% at 8% 42%, rgba(156, 193, 231, 0.25) 0%, rgba(156, 193, 231, 0) 75%)',
          `linear-gradient(180deg, ${dsBackgroundGradient.wash}4d 0%, rgba(249, 248, 248, 0) 55%)`,
        ].join(', '),
        backgroundAttachment: 'fixed',
        backgroundRepeat: 'no-repeat',
        '&::-webkit-scrollbar': { width: 8, height: 8 },
        '&::-webkit-scrollbar-track': {
          backgroundColor: 'rgba(85, 128, 250, 0.05)',
          borderRadius: 4,
        },
        '&::-webkit-scrollbar-thumb': {
          backgroundColor: 'rgba(85, 128, 250, 0.32)',
          borderRadius: 4,
          '&:hover': { backgroundColor: 'rgba(85, 128, 250, 0.48)' },
        },
        '*::-webkit-scrollbar': { width: 6, height: 6 },
        '*::-webkit-scrollbar-track': {
          backgroundColor: 'rgba(85, 128, 250, 0.05)',
          borderRadius: 3,
        },
        '*::-webkit-scrollbar-thumb': {
          backgroundColor: 'rgba(85, 128, 250, 0.26)',
          borderRadius: 3,
          '&:hover': { backgroundColor: 'rgba(85, 128, 250, 0.42)' },
        },
      },
    },
  },
};
