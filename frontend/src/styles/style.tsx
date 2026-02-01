export const ToggleButtonGroupStyle = {
    bgcolor: 'action.hover',
    '& .MuiToggleButton-root': {
        color: 'text.primary',
        padding: '4px 12px',
        fontSize: '0.875rem',
        // textTransform: 'none' as const,
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
