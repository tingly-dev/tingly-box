import type { ThemeOptions } from '@mui/material/styles';
import { sunlitPrimary, sunlitPrimaryLight, sunlitPrimaryDark } from '../palettes/sunlit';

// Sunlit reusable tokens for component overrides
const sunlitTokens = {
  border: '1px solid rgba(14, 165, 233, 0.15)',
  borderSoft: '1px solid rgba(14, 165, 233, 0.1)',
  divider: 'rgba(14, 165, 233, 0.12)',
  paperBg: 'rgba(255, 255, 255, 0.82)',
  paperBgLight: 'rgba(255, 255, 255, 0.65)',
  paperBgMedium: 'rgba(255, 255, 255, 0.72)',
  paperBgStrong: 'rgba(255, 255, 255, 0.92)',
  paperBgSolid: 'rgba(255, 255, 255, 0.96)',
  inputBg: 'rgba(255, 255, 255, 0.6)',
  inputBgHover: 'rgba(255, 255, 255, 0.72)',
  inputBgFocus: 'rgba(255, 255, 255, 0.82)',
  inputBgDisabled: 'rgba(255, 255, 255, 0.4)',
  borderInput: 'rgba(14, 165, 233, 0.25)',
  borderInputHover: 'rgba(14, 165, 233, 0.4)',
  borderInputDisabled: 'rgba(14, 165, 233, 0.12)',
  hover: 'rgba(14, 165, 233, 0.08)',
  selected: 'rgba(14, 165, 233, 0.16)',
  selectedHover: 'rgba(14, 165, 233, 0.22)',
  rowHover: 'rgba(14, 165, 233, 0.04)',
  tableHeadBg: 'rgba(14, 165, 233, 0.06)',
  scrollbarTrack: 'rgba(14, 165, 233, 0.05)',
  scrollbarThumb: 'rgba(14, 165, 233, 0.25)',
  scrollbarThumbHover: 'rgba(14, 165, 233, 0.4)',
  scrollbarThumbInner: 'rgba(14, 165, 233, 0.2)',
  scrollbarThumbInnerHover: 'rgba(14, 165, 233, 0.35)',
};

const cardShadow = '0 2px 16px rgba(14, 165, 233, 0.12), 0 1px 6px rgba(0, 0, 0, 0.04)';
const buttonHoverShadow = '0 2px 8px rgba(14, 165, 233, 0.2)';

export const sunlitComponents: ThemeOptions['components'] = {
  MuiCard: {
    styleOverrides: {
      root: {
        boxShadow: cardShadow,
        borderRadius: 12,
        border: sunlitTokens.border,
        backgroundColor: sunlitTokens.paperBg,
        backgroundImage: 'none',
        backdropFilter: 'blur(12px)',
      },
    },
  },
  MuiListItemButton: {
    styleOverrides: {
      root: {
        '&.nav-item-active': {
          backgroundColor: sunlitPrimary,
          color: '#ffffff',
          '&:hover': { backgroundColor: sunlitPrimaryDark },
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
      contained: {
        background: `linear-gradient(135deg, ${sunlitPrimary} 0%, ${sunlitPrimaryDark} 100%)`,
        '&:hover': {
          background: `linear-gradient(135deg, ${sunlitPrimaryDark} 0%, #0369a1 100%)`,
        },
      },
      outlined: {
        borderColor: 'rgba(14, 165, 233, 0.3)',
        color: '#0369a1',
        '&:hover': {
          borderColor: 'rgba(14, 165, 233, 0.5)',
          backgroundColor: sunlitTokens.hover,
        },
      },
    },
  },
  MuiOutlinedInput: {
    styleOverrides: {
      root: {
        borderRadius: 6,
        backgroundColor: sunlitTokens.inputBg,
        transition: 'background-color 120ms ease, border-color 120ms ease',
        '& .MuiOutlinedInput-notchedOutline': { borderColor: sunlitTokens.borderInput },
        '&:hover': { backgroundColor: sunlitTokens.inputBgHover },
        '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: sunlitTokens.borderInputHover },
        '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
          borderColor: sunlitPrimary,
          borderWidth: 1.5,
        },
        '&.Mui-disabled': {
          backgroundColor: sunlitTokens.inputBgDisabled,
          '& .MuiOutlinedInput-notchedOutline': {
            borderColor: sunlitTokens.borderInputDisabled,
          },
        },
        '&.Mui-error .MuiOutlinedInput-notchedOutline': {
          borderColor: '#ef4444',
        },
      },
      input: {
        '&::placeholder': { color: '#64748b', opacity: 1 },
      },
    },
  },
  MuiFilledInput: {
    styleOverrides: {
      root: {
        backgroundColor: sunlitTokens.inputBg,
        '&:hover': { backgroundColor: sunlitTokens.inputBgHover },
        '&.Mui-focused': { backgroundColor: sunlitTokens.inputBgFocus },
      },
    },
  },
  MuiInputBase: {
    styleOverrides: {
      input: {
        color: '#0f172a',
        '&::placeholder': { color: '#64748b', opacity: 1 },
      },
    },
  },
  MuiInputLabel: {
    styleOverrides: {
      root: {
        color: '#475569',
        '&.Mui-focused': { color: sunlitPrimary },
      },
    },
  },
  MuiFormHelperText: {
    styleOverrides: {
      root: {
        color: '#64748b',
        '&.Mui-error': { color: '#ef4444' },
      },
    },
  },
  MuiSelect: {
    styleOverrides: {
      icon: { color: '#475569' },
    },
  },
  MuiMenu: {
    styleOverrides: {
      paper: {
        backgroundColor: sunlitTokens.paperBgSolid,
        backgroundImage: 'none',
        border: sunlitTokens.border,
        boxShadow: '0 10px 24px rgba(15, 23, 42, 0.08)',
      },
    },
  },
  MuiMenuItem: {
    styleOverrides: {
      root: {
        '&:hover': { backgroundColor: sunlitTokens.hover },
        '&.Mui-selected': {
          backgroundColor: sunlitTokens.selected,
          '&:hover': { backgroundColor: sunlitTokens.selectedHover },
        },
      },
    },
  },
  MuiAlert: {
    styleOverrides: {
      root: {
        borderRadius: 6,
        backgroundColor: sunlitTokens.paperBgStrong,
      },
    },
  },
  MuiDrawer: {
    styleOverrides: {
      paper: {
        borderRight: sunlitTokens.border,
        backgroundColor: sunlitTokens.paperBgMedium,
        backgroundImage: 'none',
        backdropFilter: 'blur(8px)',
        willChange: 'auto',
      },
    },
  },
  MuiAppBar: {
    styleOverrides: {
      root: {
        backgroundColor: sunlitTokens.paperBgMedium,
        backgroundImage: 'none',
        color: '#0f172a',
        borderBottom: sunlitTokens.border,
        boxShadow: 'none',
      },
    },
  },
  MuiDialog: {
    styleOverrides: {
      paper: {
        backgroundColor: sunlitTokens.paperBgStrong,
        backgroundImage: 'none',
      },
    },
  },
  MuiPopover: {
    styleOverrides: {
      paper: {
        backgroundColor: sunlitTokens.paperBgSolid,
        backgroundImage: 'none',
      },
    },
  },
  MuiTooltip: {
    styleOverrides: {
      tooltip: {
        backgroundColor: '#ffffff',
        color: '#1e293b',
        fontSize: '0.75rem',
        border: '1px solid #e2e8f0',
        boxShadow: '0 4px 12px rgba(15, 23, 42, 0.08)',
      },
      arrow: { color: '#ffffff' },
    },
  },
  MuiDivider: {
    styleOverrides: {
      root: { borderColor: sunlitTokens.divider },
    },
  },
  MuiTabs: {
    styleOverrides: {
      indicator: {
        height: 4,
        borderRadius: 2,
        backgroundColor: sunlitPrimary,
      },
    },
  },
  MuiPaper: {
    styleOverrides: {
      root: {
        backgroundColor: sunlitTokens.paperBgLight,
        backdropFilter: 'blur(8px)',
        willChange: 'auto',
      },
    },
  },
  MuiTableCell: {
    styleOverrides: {
      root: {
        borderBottom: sunlitTokens.borderSoft,
      },
      head: {
        backgroundColor: sunlitTokens.tableHeadBg,
        fontWeight: 600,
      },
    },
  },
  MuiTableRow: {
    styleOverrides: {
      root: {
        '&:hover': {
          backgroundColor: sunlitTokens.rowHover,
        },
      },
    },
  },
  MuiSwitch: {
    styleOverrides: {
      switchBase: {
        '&.Mui-checked': {
          color: sunlitPrimary,
          '& + .MuiSwitch-track': {
            backgroundColor: sunlitPrimary,
            opacity: 0.6,
          },
        },
      },
      track: {
        backgroundColor: 'rgba(14, 165, 233, 0.3)',
      },
    },
  },
  MuiSlider: {
    styleOverrides: {
      root: { color: sunlitPrimary },
      thumb: {
        '&:hover, &.Mui-focusVisible': {
          boxShadow: '0 0 0 8px rgba(14, 165, 233, 0.16)',
        },
      },
      track: {
        background: `linear-gradient(90deg, ${sunlitPrimaryLight} 0%, ${sunlitPrimary} 100%)`,
      },
    },
  },
  MuiLinearProgress: {
    styleOverrides: {
      root: {
        backgroundColor: 'rgba(14, 165, 233, 0.15)',
        borderRadius: 4,
      },
      bar: {
        background: `linear-gradient(90deg, ${sunlitPrimaryLight} 0%, ${sunlitPrimary} 100%)`,
        borderRadius: 4,
      },
    },
  },
  MuiCircularProgress: {
    styleOverrides: {
      root: { color: sunlitPrimary },
    },
  },
  MuiToggleButton: {
    styleOverrides: {
      root: {
        borderColor: sunlitTokens.borderInput,
        '&.Mui-selected': {
          backgroundColor: 'rgba(14, 165, 233, 0.15)',
          color: sunlitPrimary,
          '&:hover': { backgroundColor: sunlitTokens.selectedHover },
        },
      },
    },
  },
  MuiBadge: {
    styleOverrides: {
      badge: {
        background: `linear-gradient(135deg, ${sunlitPrimary} 0%, ${sunlitPrimaryDark} 100%)`,
      },
    },
  },
  MuiSkeleton: {
    styleOverrides: {
      root: {
        backgroundColor: sunlitTokens.hover,
      },
    },
  },
  MuiCssBaseline: {
    styleOverrides: {
      body: {
        '&::-webkit-scrollbar': { width: 8, height: 8 },
        '&::-webkit-scrollbar-track': {
          backgroundColor: sunlitTokens.scrollbarTrack,
          borderRadius: 4,
        },
        '&::-webkit-scrollbar-thumb': {
          backgroundColor: sunlitTokens.scrollbarThumb,
          borderRadius: 4,
          '&:hover': { backgroundColor: sunlitTokens.scrollbarThumbHover },
        },
        '*::-webkit-scrollbar': { width: 6, height: 6 },
        '*::-webkit-scrollbar-track': {
          backgroundColor: sunlitTokens.scrollbarTrack,
          borderRadius: 3,
        },
        '*::-webkit-scrollbar-thumb': {
          backgroundColor: sunlitTokens.scrollbarThumbInner,
          borderRadius: 3,
          '&:hover': { backgroundColor: sunlitTokens.scrollbarThumbInnerHover },
        },
      },
    },
  },
};
