import type { ThemeOptions } from '@mui/material/styles';
import type { ThemeTokens } from '../tokens';
import { softSelectedColors } from '../selection';

const solid = (color: string) => `1px solid ${color}`;

/**
 * Every theme's MUI component overrides, written once. A theme only supplies
 * colours (`ThemeTokens`); geometry, transitions and structure are shared, so
 * a change here lands in every theme at once.
 */
export const createComponents = (t: ThemeTokens): ThemeOptions['components'] => ({
  MuiCard: {
    styleOverrides: {
      root: {
        // Surfaces on the page are flat: a border, no shadow. Shadows are for
        // things that float (menus, popovers, dialogs, tooltips).
        boxShadow: 'none',
        borderRadius: 12,
        border: solid(t.border.card),
        backgroundColor: t.surface.card,
        backgroundImage: 'none',
      },
    },
  },
  MuiButton: {
    styleOverrides: {
      root: {
        textTransform: 'none',
        fontWeight: 500,
        borderRadius: 8,
        boxShadow: 'none',
        '&:hover': {
          boxShadow: 'none',
        },
      },
      outlined: {
        borderColor: t.button.outlinedBorder,
        color: t.button.outlinedText,
        '&:hover': {
          borderColor: t.button.outlinedHoverBorder,
          backgroundColor: t.button.outlinedHoverBg,
        },
      },
    },
  },
  // A filled primary chip is a selected in-page option (a slot, a filter):
  // the soft selected look, not a solid block. See selection.ts.
  MuiChip: {
    variants: [
      {
        props: { variant: 'filled', color: 'primary' },
        style: ({ theme }) => {
          const c = softSelectedColors(theme);
          return {
            backgroundColor: c.bg,
            color: c.ink,
            '& .MuiChip-icon, & .MuiChip-deleteIcon': { color: 'inherit' },
            '&.MuiChip-clickable:hover, &.Mui-focusVisible': { backgroundColor: c.bgHover },
          };
        },
      },
    ],
  },
  MuiOutlinedInput: {
    styleOverrides: {
      root: {
        borderRadius: 8,
        backgroundColor: t.input.bg,
        transition: 'background-color 120ms ease, border-color 120ms ease',
        // MUI keeps adornment icons at 24px inside size="small" inputs, which
        // visually inflates the field — shrink them app-wide at the theme level.
        '&.MuiInputBase-sizeSmall .MuiInputAdornment-root .MuiSvgIcon-root': {
          fontSize: 20,
        },
        '& .MuiOutlinedInput-notchedOutline': { borderColor: t.input.border },
        ...(t.input.hoverBg && { '&:hover': { backgroundColor: t.input.hoverBg } }),
        '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: t.input.hoverBorder },
        '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
          borderColor: t.accent,
          borderWidth: 1.5,
        },
        '&.Mui-disabled': {
          ...(t.input.disabledBg && { backgroundColor: t.input.disabledBg }),
          '& .MuiOutlinedInput-notchedOutline': {
            borderColor: t.input.disabledBorder,
          },
        },
        '&.Mui-error .MuiOutlinedInput-notchedOutline': {
          borderColor: t.error,
        },
      },
      input: {
        '&::placeholder': { color: t.placeholder, opacity: 1 },
      },
    },
  },
  MuiFilledInput: {
    styleOverrides: {
      root: {
        backgroundColor: t.input.filledBg,
        '&:hover': { backgroundColor: t.input.filledHoverBg },
        '&.Mui-focused': { backgroundColor: t.input.filledFocusBg },
      },
    },
  },
  MuiInputBase: {
    styleOverrides: {
      input: {
        color: t.text,
        '&::placeholder': { color: t.placeholder, opacity: 1 },
      },
    },
  },
  MuiInputLabel: {
    styleOverrides: {
      root: {
        color: t.textMuted,
        '&.Mui-focused': { color: t.accent },
      },
    },
  },
  MuiFormHelperText: {
    styleOverrides: {
      root: {
        color: t.textHelper,
        '&.Mui-error': { color: t.error },
      },
    },
  },
  MuiSelect: {
    styleOverrides: {
      icon: { color: t.textMuted },
    },
  },
  MuiMenu: {
    styleOverrides: {
      paper: {
        backgroundColor: t.surface.menu,
        backgroundImage: 'none',
        border: solid(t.border.menu),
        boxShadow: t.shadow.menu,
      },
    },
  },
  MuiMenuItem: {
    styleOverrides: {
      root: {
        '&:hover': { backgroundColor: t.menuItem.hover },
        '&.Mui-selected': {
          backgroundColor: t.menuItem.selected,
          '&:hover': { backgroundColor: t.menuItem.selectedHover },
        },
      },
    },
  },
  MuiDrawer: {
    styleOverrides: {
      paper: {
        borderRight: solid(t.border.chrome),
        ...(t.surface.drawer && { backgroundColor: t.surface.drawer }),
        backgroundImage: 'none',
        willChange: 'auto',
      },
    },
  },
  MuiAppBar: {
    styleOverrides: {
      root: {
        backgroundColor: t.surface.appBar,
        backgroundImage: 'none',
        color: t.text,
        borderBottom: solid(t.border.chrome),
        boxShadow: 'none',
      },
    },
  },
  MuiDialog: {
    styleOverrides: {
      paper: {
        borderRadius: 12,
        backgroundColor: t.surface.dialog,
        backgroundImage: 'none',
        ...(t.border.dialog && { border: solid(t.border.dialog) }),
      },
    },
  },
  MuiPopover: {
    styleOverrides: {
      paper: {
        backgroundColor: t.surface.popover,
        backgroundImage: 'none',
        ...(t.border.popover && { border: solid(t.border.popover) }),
      },
    },
  },
  MuiTooltip: {
    styleOverrides: {
      tooltip: {
        backgroundColor: t.surface.tooltip,
        color: t.text,
        fontSize: '0.75rem',
        border: solid(t.border.tooltip),
        boxShadow: t.shadow.tooltip,
      },
      arrow: { color: t.surface.tooltip },
    },
  },
  MuiDivider: {
    styleOverrides: {
      root: { borderColor: t.border.divider },
    },
  },
  MuiTabs: {
    styleOverrides: {
      indicator: {
        height: 2,
        borderRadius: 1,
        backgroundColor: t.accent,
      },
    },
  },
  MuiPaper: {
    styleOverrides: {
      root: {
        // MUI auto-lightens dark-mode Paper at higher elevations via a white
        // overlay gradient. Disable it so each theme's surface ladder stays
        // what it says — elevation reads as shadow, not as a brighter fill.
        ...(t.surface.paper && { backgroundColor: t.surface.paper }),
        backgroundImage: 'none',
        willChange: 'auto',
      },
    },
  },
  ...(t.table && {
    MuiTableCell: {
      styleOverrides: {
        ...(t.table.cellBorder && { root: { borderBottom: solid(t.table.cellBorder) } }),
        ...(t.table.headBg && { head: { backgroundColor: t.table.headBg, fontWeight: 600 } }),
      },
    },
    ...(t.table.rowHover && {
      MuiTableRow: {
        styleOverrides: {
          root: { '&:hover': { backgroundColor: t.table.rowHover } },
        },
      },
    }),
  }),
  MuiLinearProgress: {
    styleOverrides: {
      root: { borderRadius: 4 },
      bar: { borderRadius: 4 },
    },
  },
});
