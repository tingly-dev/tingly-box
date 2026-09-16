import type { ThemeOptions } from '@mui/material/styles';
import { dsPrimary, dsPrimaryLight, dsPrimaryDark, dsBackgroundGradient } from '../palettes/ds';
import { primaryGradientButton } from './buttonVariants';

// DS reusable tokens for component overrides
const dsTokens = {
  border: '1px solid rgba(79, 110, 247, 0.15)',
  borderSoft: '1px solid rgba(79, 110, 247, 0.1)',
  divider: 'rgba(79, 110, 247, 0.12)',
  paperBg: 'rgba(255, 255, 255, 0.88)',
  paperBgLight: 'rgba(255, 255, 255, 0.78)',
  paperBgMedium: 'rgba(255, 255, 255, 0.82)',
  paperBgStrong: 'rgba(255, 255, 255, 0.94)',
  paperBgSolid: 'rgba(255, 255, 255, 0.97)',
  inputBg: 'rgba(255, 255, 255, 0.75)',
  inputBgHover: 'rgba(255, 255, 255, 0.85)',
  inputBgFocus: 'rgba(255, 255, 255, 0.92)',
  inputBgDisabled: 'rgba(255, 255, 255, 0.5)',
  borderInput: 'rgba(79, 110, 247, 0.25)',
  borderInputHover: 'rgba(79, 110, 247, 0.4)',
  borderInputDisabled: 'rgba(79, 110, 247, 0.12)',
  hover: 'rgba(79, 110, 247, 0.08)',
  selected: 'rgba(79, 110, 247, 0.16)',
  selectedHover: 'rgba(79, 110, 247, 0.22)',
  rowHover: 'rgba(79, 110, 247, 0.04)',
  tableHeadBg: 'rgba(79, 110, 247, 0.06)',
  scrollbarTrack: 'rgba(79, 110, 247, 0.05)',
  scrollbarThumb: 'rgba(79, 110, 247, 0.25)',
  scrollbarThumbHover: 'rgba(79, 110, 247, 0.4)',
  scrollbarThumbInner: 'rgba(79, 110, 247, 0.2)',
  scrollbarThumbInnerHover: 'rgba(79, 110, 247, 0.35)',
};

const cardShadow = '0 2px 16px rgba(79, 110, 247, 0.12), 0 1px 6px rgba(0, 0, 0, 0.04)';
const buttonHoverShadow = '0 2px 8px rgba(79, 110, 247, 0.2)';

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
        borderColor: 'rgba(79, 110, 247, 0.3)',
        color: '#3b54d4',
        '&:hover': {
          borderColor: 'rgba(79, 110, 247, 0.5)',
          backgroundColor: dsTokens.hover,
        },
      },
    },
    variants: primaryGradientButton(dsPrimary, dsPrimaryDark, dsPrimaryDark, '#3b54d4'),
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
        '&::placeholder': { color: '#565f7e', opacity: 1 },
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
        color: '#1e2338',
        '&::placeholder': { color: '#565f7e', opacity: 1 },
      },
    },
  },
  MuiInputLabel: {
    styleOverrides: {
      root: {
        color: '#565f7e',
        '&.Mui-focused': { color: dsPrimary },
      },
    },
  },
  MuiFormHelperText: {
    styleOverrides: {
      root: {
        color: '#565f7e',
        '&.Mui-error': { color: '#ef4444' },
      },
    },
  },
  MuiSelect: {
    styleOverrides: {
      icon: { color: '#565f7e' },
    },
  },
  MuiMenu: {
    styleOverrides: {
      paper: {
        backgroundColor: dsTokens.paperBgSolid,
        backgroundImage: 'none',
        border: dsTokens.border,
        boxShadow: '0 10px 24px rgba(30, 35, 56, 0.1)',
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
        color: '#1e2338',
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
        color: '#1e2338',
        fontSize: '0.75rem',
        border: '1px solid #e2e6f5',
        boxShadow: '0 4px 12px rgba(30, 35, 56, 0.08)',
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
        backgroundColor: 'rgba(79, 110, 247, 0.3)',
      },
    },
  },
  MuiSlider: {
    styleOverrides: {
      root: { color: dsPrimary },
      thumb: {
        '&:hover, &.Mui-focusVisible': {
          boxShadow: '0 0 0 8px rgba(79, 110, 247, 0.16)',
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
        backgroundColor: 'rgba(79, 110, 247, 0.15)',
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
          backgroundColor: 'rgba(79, 110, 247, 0.15)',
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
        // with an animated <canvas> "flow field" drawing moving wisps in
        // `dsBackgroundGradient.wash`/`.accent`/white on top. We can't reasonably run
        // that canvas behind every dashboard/table page here, so this approximates a
        // freeze-frame of it: several soft, overlapping radial blobs near the top
        // (their real streak colors, just static) fading into the near-white base,
        // instead of one flat linear band.
        backgroundColor: dsBackgroundGradient.base,
        backgroundImage: [
          'radial-gradient(52% 40% at 20% -8%, rgba(156, 193, 231, 0.55) 0%, rgba(156, 193, 231, 0) 72%)',
          'radial-gradient(46% 36% at 58% -6%, rgba(138, 163, 214, 0.5) 0%, rgba(138, 163, 214, 0) 72%)',
          'radial-gradient(58% 38% at 90% 2%, rgba(156, 193, 231, 0.4) 0%, rgba(156, 193, 231, 0) 74%)',
          `linear-gradient(180deg, ${dsBackgroundGradient.wash}59 0%, rgba(249, 248, 248, 0) 45%)`,
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
