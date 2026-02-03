/**
 * Unified Toggle/Switch Styles
 *
 * Design Principles:
 * 1. ToggleButtonGroup - For multi-choice selections (3+ options)
 * 2. Switch - For binary on/off toggles
 * 3. Consistent visual language across all components
 */

import { SxProps, Theme } from '@mui/material/styles';

// ============================================================================
// ToggleButtonGroup Styles (Multi-Choice Selection)
// ============================================================================

/**
 * Style for ToggleButtonGroup container
 * Provides a subtle background with rounded pill shape
 */
export const toggleButtonGroupStyle: SxProps<Theme> = {
    backgroundColor: 'action.hover',
    borderRadius: 2,
    p: 0.5,
    gap: 0.5,
    display: 'flex',
    border: '1px solid',
    borderColor: 'divider',
};

/**
 * Style for individual ToggleButton
 * Selected state: Primary color with subtle shadow
 * Unselected: Transparent with hover effect
 */
export const toggleButtonStyle: SxProps<Theme> = {
    borderRadius: 1.5,
    px: 2,
    py: 0.75,
    fontSize: '0.875rem',
    fontWeight: 500,
    textTransform: 'none',
    border: 'none',
    color: 'text.secondary',
    transition: 'all 0.2s ease-in-out',
    backgroundColor: 'transparent',
    boxShadow: 'none',
    '&:hover': {
        backgroundColor: 'action.selected',
        color: 'text.primary',
    },
    '&.Mui-selected': {
        backgroundColor: 'background.paper',
        color: 'primary.main',
        fontWeight: 600,
        boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
        '&:hover': {
            backgroundColor: 'background.paper',
            color: 'primary.dark',
        },
    },
    '&.Mui-disabled': {
        color: 'text.disabled',
        backgroundColor: 'transparent',
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
        fontSize: '0.875rem',
        color: 'text.primary',
        fontWeight: 500,
    },
};

/**
 * Base Switch style - can be customized via color prop
 * Default uses primary color, can override with success, error, etc.
 */
export const switchBaseStyle: SxProps<Theme> = {
    padding: 8,
    '& .MuiSwitch-track': {
        borderRadius: 12,
        borderWidth: 1,
        borderColor: 'divider',
    },
    '& .MuiSwitch-thumb': {
        boxShadow: '0 2px 4px rgba(0,0,0,0.2)',
    },
    '& .MuiSwitch-switchBase': {
        '&.Mui-checked': {
            transform: 'translateX(20px)',
            '& + .MuiSwitch-track': {
                opacity: 1,
            },
        },
    },
};

// ============================================================================
// Compact Variants (For dense layouts)
// ============================================================================

/**
 * Compact ToggleButton - for smaller spaces
 */
export const toggleButtonCompactStyle: SxProps<Theme> = {
    ...toggleButtonStyle,
    px: 1.5,
    py: 0.5,
    fontSize: '0.8125rem',
    minHeight: 32,
};

/**
 * Small Switch - for dense table rows
 */
export const switchSmallStyle: SxProps<Theme> = {
    ...switchBaseStyle,
    '& .MuiSwitch-switchBase': {
        padding: 4,
        '&.Mui-checked': {
            transform: 'translateX(16px)',
        },
    },
    '& .MuiSwitch-thumb': {
        width: 16,
        height: 16,
    },
    '& .MuiSwitch-track': {
        height: 20,
        borderRadius: 10,
    },
};

// ============================================================================
// Color Variants for Switch
// ============================================================================

/**
 * Success colored Switch (for enable/disable contexts)
 */
export const switchSuccessStyle: SxProps<Theme> = {
    ...switchBaseStyle,
    '& .MuiSwitch-switchBase': {
        '&.Mui-checked': {
            color: 'success.main',
            '& + .MuiSwitch-track': {
                backgroundColor: 'success.main',
                opacity: 0.5,
            },
        },
    },
};

/**
 * Warning colored Switch (for caution contexts)
 */
export const switchWarningStyle: SxProps<Theme> = {
    ...switchBaseStyle,
    '& .MuiSwitch-switchBase': {
        '&.Mui-checked': {
            color: 'warning.main',
            '& + .MuiSwitch-track': {
                backgroundColor: 'warning.main',
                opacity: 0.5,
            },
        },
    },
};
