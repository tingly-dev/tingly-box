/**
 * @deprecated Use toggleStyles.tsx instead for new components
 *
 * Legacy ToggleButton styles - kept for backward compatibility
 * New components should import from @/styles/toggleStyles
 */
export const ToggleButtonGroupStyle = {
    bgcolor: 'action.hover',
    '& .MuiToggleButton-root': {
        color: 'text.primary',
        padding: '4px 12px',
        '&:hover': {
            bgcolor: 'action.selected',
        },
    },
}

export const ToggleButtonStyle = {
    '&.Mui-selected': {
        bgcolor: 'primary.main',
        color: 'white',
        '&:hover': {
            bgcolor: 'primary.dark',
        },
    },
}

// Re-export new unified styles for convenience
// Components can import from either file
export {
    toggleButtonGroupStyle,
    toggleButtonStyle,
    switchControlLabelStyle,
    switchBaseStyle,
    switchSuccessStyle,
    switchWarningStyle,
    toggleButtonCompactStyle,
    switchSmallStyle,
} from './toggleStyles';
