/**
 * Unified Toggle/Switch Styles
 *
 * Design Principles:
 * 1. ToggleButtonGroup - For multi-choice selections (3+ options)
 * 2. Switch - For binary on/off toggles
 * 3. Consistent visual language across all components
 */

import type { SxProps, Theme } from '@mui/material/styles';
import { radius } from '@tingly/ui/theme/radius';

// ============================================================================
// ToggleButtonGroup Styles (Multi-Choice Selection)
// ============================================================================

/**
 * Style for ToggleButtonGroup container
 * Provides a subtle background with rounded pill shape
 */
export const toggleButtonGroupStyle: SxProps<Theme> = {
    backgroundColor: 'action.hover',
    borderRadius: radius.row,
    display: 'flex',
    border: '1px solid',
    borderColor: 'divider',
    '& .MuiToggleButton-root': {
        border: '1px solid',
        borderColor: 'divider',
        textTransform: 'none',
        px: 2,
        py: 1,
        height: 32,
    },
    '& .MuiToggleButton-root.Mui-selected': {
        bgcolor: 'primary.main',
        color: 'primary.contrastText',
        border: 'none',
        '&:hover': {
            bgcolor: 'primary.dark',
        },
    },
};

// ============================================================================
// Switch Styles (Binary On/Off Toggles)
// ============================================================================

/**
 * Style container for Switch + Label combo
 * Provides consistent spacing and alignment
 */
export const switchControlLabelStyle: SxProps<Theme> = {
    mx: 0,
    alignItems: 'center',
    '& .MuiFormControlLabel-label': {
        color: 'text.primary',
        fontWeight: 500,
    },
};
