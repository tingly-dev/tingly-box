import type { ThemeOptions } from '@mui/material/styles';
import { dsPrimary, dsPrimaryLight, dsPrimaryDark, dsBackgroundGradient } from '../palettes/ds';
import { primaryGradientButton } from './buttonVariants';

// DS reusable tokens for component overrides
const dsTokens = {
  border: '1px solid rgba(103, 153, 254, 0.15)',
  borderSoft: '1px solid rgba(103, 153, 254, 0.1)',
  divider: 'rgba(103, 153, 254, 0.12)',
  paperBg: 'rgba(255, 255, 255, 0.88)',
  paperBgLight: 'rgba(255, 255, 255, 0.78)',
  paperBgMedium: 'rgba(255, 255, 255, 0.82)',
  paperBgStrong: 'rgba(255, 255, 255, 0.94)',
  paperBgSolid: 'rgba(255, 255, 255, 0.97)',
  inputBg: 'rgba(255, 255, 255, 0.75)',
  inputBgHover: 'rgba(255, 255, 255, 0.85)',
  inputBgFocus: 'rgba(255, 255, 255, 0.92)',
  inputBgDisabled: 'rgba(255, 255, 255, 0.5)',
  borderInput: 'rgba(103, 153, 254, 0.25)',
  borderInputHover: 'rgba(103, 153, 254, 0.4)',
  borderInputDisabled: 'rgba(103, 153, 254, 0.12)',
  hover: 'rgba(103, 153, 254, 0.08)',
  selected: 'rgba(103, 153, 254, 0.16)',
  selectedHover: 'rgba(103, 153, 254, 0.22)',
  rowHover: 'rgba(103, 153, 254, 0.04)',
  tableHeadBg: 'rgba(103, 153, 254, 0.06)',
  scrollbarTrack: 'rgba(103, 153, 254, 0.05)',
  scrollbarThumb: 'rgba(103, 153, 254, 0.25)',
  scrollbarThumbHover: 'rgba(103, 153, 254, 0.4)',
  scrollbarThumbInner: 'rgba(103, 153, 254, 0.2)',
  scrollbarThumbInnerHover: 'rgba(103, 153, 254, 0.35)',
};

const cardShadow = '0 2px 16px rgba(103, 153, 254, 0.12), 0 1px 6px rgba(0, 0, 0, 0.04)';
const buttonHoverShadow = '0 2px 8px rgba(103, 153, 254, 0.2)';

export const dsComponents: ThemeOptions['components'] = {
  MuiCard: {
    styleOverrides: {
      root: {
        boxShadow: cardShadow,
        borderRadius: 12,
        border: dsTokens.border,
        backgroundColor: dsTokens.paperBg,
        backgroundImage: 'none',
      },
    },
  },
  MuiListItemButton: {
    styleOverrides: {
      root: {
        '&.nav-item-active': {
          backgroundColor: dsPrimary,
          color: '#ffffff',
          '&:hover': { backgroundColor: dsPrimaryDark },
          '& .MuiListItemIcon-root': { color: '#ffffff' },
          '& .MuiListItemText-primary': {
            color: '#ffffff',
            fontWeight: 600,
          },
        },
      },
    },
  },
  MuiButton: {
    styleOverrides: {
      root: {
        textTransform: 'none',
        fontWeight: 500,
        borderRadius: 6,
        boxShadow: 'none',
        '&:hover': { boxShadow: buttonHoverShadow },
      },
      outlined: {
        borderColor: 'rgba(103, 153, 254, 0.3)',
        color: dsPrimaryDark,
        '&:hover': {
          borderColor: 'rgba(103, 153, 254, 0.5)',
          backgroundColor: dsTokens.hover,
        },
      },
    },
    variants: primaryGradientButton(dsPrimary, dsPrimaryDark, dsPrimaryDark, dsPrimaryDark),
  },
  MuiOutlinedInput: {
    styleOverrides: {
      root: {
        borderRadius: 6,
        backgroundColor: dsTokens.inputBg,
        transition: 'background-color 120ms ease, border-color 120ms ease',
        '&.MuiInputBase-sizeSmall .MuiInputAdornment-root .MuiSvgIcon-root': {
          fontSize: 20,
        },
        '& .MuiOutlinedInput-notchedOutline': { borderColor: dsTokens.borderInput },
        '&:hover': { backgroundColor: dsTokens.inputBgHover },
        '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: dsTokens.borderInputHover },
        '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
          borderColor: dsPrimary,
          borderWidth: 1.5,
        },
        '&.Mui-disabled': {
          backgroundColor: dsTokens.inputBgDisabled,
          '& .MuiOutlinedInput-notchedOutline': {
            borderColor: dsTokens.borderInputDisabled,
          },
        },
        '&.Mui-error .MuiOutlinedInput-notchedOutline': {
          borderColor: '#ef4444',
        },
      },
      input: {
        '&::placeholder': { color: '#565f70', opacity: 1 },
      },
    },
  },
  MuiFilledInput: {
    styleOverrides: {
      root: {
        backgroundColor: dsTokens.inputBg,
        '&:hover': { backgroundColor: dsTokens.inputBgHover },
        '&.Mui-focused': { backgroundColor: dsTokens.inputBgFocus },
      },
    },
  },
  MuiInputBase: {
    styleOverrides: {
      input: {
        color: '#1e232c',
        '&::placeholder': { color: '#565f70', opacity: 1 },
      },
    },
  },
  MuiInputLabel: {
    styleOverrides: {
      root: {
        color: '#565f70',
        '&.Mui-focused': { color: dsPrimary },
      },
    },
  },
  MuiFormHelperText: {
    styleOverrides: {
      root: {
        color: '#565f70',
        '&.Mui-error': { color: '#ef4444' },
      },
    },
  },
  MuiSelect: {
    styleOverrides: {
      icon: { color: '#565f70' },
    },
  },
  MuiMenu: {
    styleOverrides: {
      paper: {
        backgroundColor: dsTokens.paperBgSolid,
        backgroundImage: 'none',
        border: dsTokens.border,
        boxShadow: '0 10px 24px rgba(30, 35, 44, 0.1)',
      },
    },
  },
  MuiMenuItem: {
    styleOverrides: {
      root: {
        '&:hover': { backgroundColor: dsTokens.hover },
        '&.Mui-selected': {
          backgroundColor: dsTokens.selected,
          '&:hover': { backgroundColor: dsTokens.selectedHover },
        },
      },
    },
  },
  MuiAlert: {
    styleOverrides: {
      root: {
        borderRadius: 6,
        backgroundColor: dsTokens.paperBgStrong,
      },
    },
  },
  MuiDrawer: {
    styleOverrides: {
      paper: {
        borderRight: dsTokens.border,
        backgroundColor: dsTokens.paperBgMedium,
        backgroundImage: 'none',
      },
    },
  },
  MuiAppBar: {
    styleOverrides: {
      root: {
        backgroundColor: dsTokens.paperBgMedium,
        backgroundImage: 'none',
        color: '#1e232c',
        borderBottom: dsTokens.border,
        boxShadow: 'none',
      },
    },
  },
  MuiDialog: {
    styleOverrides: {
      paper: {
        backgroundColor: dsTokens.paperBgStrong,
        backgroundImage: 'none',
      },
    },
  },
  MuiPopover: {
    styleOverrides: {
      paper: {
        backgroundColor: dsTokens.paperBgSolid,
        backgroundImage: 'none',
      },
    },
  },
  MuiTooltip: {
    styleOverrides: {
      tooltip: {
        backgroundColor: '#ffffff',
        color: '#1e232c',
        fontSize: '0.75rem',
        border: '1px solid #e2e6f5',
        boxShadow: '0 4px 12px rgba(30, 35, 44, 0.08)',
      },
      arrow: { color: '#ffffff' },
    },
  },
  MuiDivider: {
    styleOverrides: {
      root: { borderColor: dsTokens.divider },
    },
  },
  MuiTabs: {
    styleOverrides: {
      indicator: {
        height: 4,
        borderRadius: 2,
        backgroundColor: dsPrimary,
      },
    },
  },
  MuiPaper: {
    styleOverrides: {
      root: {
        backgroundColor: dsTokens.paperBgLight,
      },
    },
  },
  MuiTableCell: {
    styleOverrides: {
      root: {
        borderBottom: dsTokens.borderSoft,
      },
      head: {
        backgroundColor: dsTokens.tableHeadBg,
        fontWeight: 600,
      },
    },
  },
  MuiTableRow: {
    styleOverrides: {
      root: {
        '&:hover': {
          backgroundColor: dsTokens.rowHover,
        },
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
        backgroundColor: 'rgba(103, 153, 254, 0.3)',
      },
    },
  },
  MuiSlider: {
    styleOverrides: {
      root: { color: dsPrimary },
      thumb: {
        '&:hover, &.Mui-focusVisible': {
          boxShadow: '0 0 0 8px rgba(103, 153, 254, 0.16)',
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
        backgroundColor: 'rgba(103, 153, 254, 0.15)',
        borderRadius: 4,
      },
      bar: {
        background: `linear-gradient(90deg, ${dsPrimaryLight} 0%, ${dsPrimary} 100%)`,
        borderRadius: 4,
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
        borderColor: dsTokens.borderInput,
        '&.Mui-selected': {
          backgroundColor: 'rgba(103, 153, 254, 0.15)',
          color: dsPrimary,
          '&:hover': { backgroundColor: dsTokens.selectedHover },
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
        backgroundColor: dsTokens.hover,
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
          backgroundColor: dsTokens.scrollbarTrack,
          borderRadius: 4,
        },
        '&::-webkit-scrollbar-thumb': {
          backgroundColor: dsTokens.scrollbarThumb,
          borderRadius: 4,
          '&:hover': { backgroundColor: dsTokens.scrollbarThumbHover },
        },
        '*::-webkit-scrollbar': { width: 6, height: 6 },
        '*::-webkit-scrollbar-track': {
          backgroundColor: dsTokens.scrollbarTrack,
          borderRadius: 3,
        },
        '*::-webkit-scrollbar-thumb': {
          backgroundColor: dsTokens.scrollbarThumbInner,
          borderRadius: 3,
          '&:hover': { backgroundColor: dsTokens.scrollbarThumbInnerHover },
        },
      },
    },
  },
};
