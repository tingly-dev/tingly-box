import type { ThemeOptions } from '@mui/material/styles';
import {
  darkBgPaper,
  darkBgRaised,
  darkInputBg,
  darkInputBgHover,
  darkInputBorder,
  darkInputBorderHover,
} from '../palettes/dark';

export const darkComponents: ThemeOptions['components'] = {
  MuiCard: {
    styleOverrides: {
      root: {
        boxShadow: '0 1px 2px 0 rgba(0, 0, 0, 0.6), 0 4px 12px 0 rgba(0, 0, 0, 0.3)',
        borderRadius: 12,
        border: '1px solid rgba(255, 255, 255, 0.08)',
        backgroundColor: darkBgPaper,
        backgroundImage: 'none',
      },
    },
  },
  MuiListItemButton: {
    styleOverrides: {
      root: {
        '&.nav-item-active': {
          backgroundColor: '#2563eb',
          color: '#ffffff',
          '&:hover': { backgroundColor: '#1d4ed8' },
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
        '&:hover': {
          boxShadow: '0 1px 2px 0 rgba(0, 0, 0, 0.3)',
        },
      },
      contained: {
        background: 'linear-gradient(135deg, #3b82f6 0%, #2563eb 100%)',
        '&:hover': {
          background: 'linear-gradient(135deg, #60a5fa 0%, #3b82f6 100%)',
        },
      },
      outlined: {
        borderColor: 'rgba(255, 255, 255, 0.23)',
        color: '#e2e8f0',
        '&:hover': {
          borderColor: 'rgba(255, 255, 255, 0.4)',
          backgroundColor: 'rgba(255, 255, 255, 0.08)',
        },
      },
    },
  },
  MuiOutlinedInput: {
    styleOverrides: {
      root: {
        borderRadius: 6,
        backgroundColor: darkInputBg,
        transition: 'background-color 120ms ease, border-color 120ms ease',
        '& .MuiOutlinedInput-notchedOutline': { borderColor: darkInputBorder },
        '&:hover': { backgroundColor: darkInputBgHover },
        '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: darkInputBorderHover },
        '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
          borderColor: '#3b82f6',
          borderWidth: 1.5,
        },
        '&.Mui-disabled': {
          backgroundColor: 'rgba(255, 255, 255, 0.02)',
          '& .MuiOutlinedInput-notchedOutline': {
            borderColor: 'rgba(148, 163, 184, 0.18)',
          },
        },
        '&.Mui-error .MuiOutlinedInput-notchedOutline': {
          borderColor: '#f87171',
        },
      },
      input: {
        '&::placeholder': { color: '#94a3b8', opacity: 1 },
      },
    },
  },
  MuiFilledInput: {
    styleOverrides: {
      root: {
        backgroundColor: darkInputBg,
        '&:hover': { backgroundColor: darkInputBgHover },
        '&.Mui-focused': { backgroundColor: darkInputBgHover },
      },
    },
  },
  MuiInputBase: {
    styleOverrides: {
      input: {
        color: '#f3f5f9',
        '&::placeholder': { color: '#94a3b8', opacity: 1 },
      },
    },
  },
  MuiInputLabel: {
    styleOverrides: {
      root: {
        color: '#cbd5e1',
        '&.Mui-focused': { color: '#3b82f6' },
      },
    },
  },
  MuiFormHelperText: {
    styleOverrides: {
      root: {
        color: '#94a3b8',
        '&.Mui-error': { color: '#f87171' },
      },
    },
  },
  MuiSelect: {
    styleOverrides: {
      icon: { color: '#cbd5e1' },
    },
  },
  MuiMenu: {
    styleOverrides: {
      paper: {
        backgroundColor: darkBgRaised,
        backgroundImage: 'none',
        border: '1px solid rgba(255, 255, 255, 0.1)',
        boxShadow: '0 10px 32px rgba(0, 0, 0, 0.7), 0 2px 8px rgba(0, 0, 0, 0.5)',
      },
    },
  },
  MuiMenuItem: {
    styleOverrides: {
      root: {
        '&:hover': { backgroundColor: 'rgba(255, 255, 255, 0.08)' },
        '&.Mui-selected': {
          backgroundColor: 'rgba(59, 130, 246, 0.24)',
          '&:hover': { backgroundColor: 'rgba(59, 130, 246, 0.32)' },
        },
      },
    },
  },
  MuiDrawer: {
    styleOverrides: {
      paper: {
        borderRight: '1px solid rgba(255, 255, 255, 0.08)',
        backgroundColor: darkBgPaper,
        backgroundImage: 'none',
        willChange: 'auto',
      },
    },
  },
  MuiAppBar: {
    styleOverrides: {
      root: {
        backgroundColor: darkBgPaper,
        backgroundImage: 'none',
        color: '#f3f5f9',
        borderBottom: '1px solid rgba(255, 255, 255, 0.08)',
        boxShadow: 'none',
      },
    },
  },
  MuiDialog: {
    styleOverrides: {
      paper: {
        backgroundColor: darkBgRaised,
        backgroundImage: 'none',
        border: '1px solid rgba(255, 255, 255, 0.08)',
      },
    },
  },
  MuiPopover: {
    styleOverrides: {
      paper: {
        backgroundColor: darkBgRaised,
        backgroundImage: 'none',
        border: '1px solid rgba(255, 255, 255, 0.1)',
      },
    },
  },
  MuiTooltip: {
    styleOverrides: {
      tooltip: {
        backgroundColor: darkBgRaised,
        color: '#f3f5f9',
        fontSize: '0.75rem',
        border: '1px solid rgba(255, 255, 255, 0.1)',
        boxShadow: '0 4px 12px rgba(0, 0, 0, 0.4)',
      },
      arrow: { color: darkBgRaised },
    },
  },
  MuiDivider: {
    styleOverrides: {
      root: { borderColor: 'rgba(255, 255, 255, 0.1)' },
    },
  },
  MuiTabs: {
    styleOverrides: {
      indicator: {
        height: 4,
        borderRadius: 2,
        backgroundColor: '#3b82f6',
      },
    },
  },
  MuiPaper: {
    styleOverrides: {
      root: {
        // MUI auto-lightens Paper at higher elevations via a white overlay
        // gradient. Disable it so our explicit dark surface ladder stays
        // consistent — elevation reads as shadow, not as a brighter fill.
        backgroundImage: 'none',
        willChange: 'auto',
      },
    },
  },
  MuiTableCell: {
    styleOverrides: {
      head: {
        backgroundColor: 'rgba(255, 255, 255, 0.03)',
        fontWeight: 600,
      },
    },
  },
  MuiLinearProgress: {
    styleOverrides: {
      root: { borderRadius: 4 },
      bar: { borderRadius: 4 },
    },
  },
};
