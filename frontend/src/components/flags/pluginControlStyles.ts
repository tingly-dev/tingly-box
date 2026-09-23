import {alpha, type Theme} from '@mui/material/styles';

// Keep page-header plugin choices aligned with selected routing controls:
// a soft tint marks an explicit setting without dominating the page.
export const pluginControlStateStyles = (theme: Theme, selected: boolean) => ({
    bgcolor: selected ? alpha(theme.palette.primary.main, 0.12) : 'transparent',
    color: selected ? theme.palette.primary.main : theme.palette.text.primary,
    fontWeight: selected ? 600 : 400,
    border: '1px solid',
    borderColor: selected ? alpha(theme.palette.primary.main, 0.48) : theme.palette.divider,
    '&:hover': {
        bgcolor: selected ? alpha(theme.palette.primary.main, 0.18) : theme.palette.action.selected,
        ...(selected && {borderColor: theme.palette.primary.main}),
    },
});
