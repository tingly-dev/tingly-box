import { type SxProps, type Theme } from '@mui/material/styles';

/**
 * Shared styles for clickable monospace text that displays URLs, tokens, or commands.
 * Used across configuration cards and pages for copyable text elements.
 */
export const copyableTextStyle: SxProps<Theme> = {
    fontFamily: 'monospace',
    color: 'primary.main',
    cursor: 'pointer',
    '&:hover': {
        textDecoration: 'underline',
        backgroundColor: 'action.hover',
    },
    padding: 1,
    borderRadius: 1,
    transition: 'all 0.2s ease-in-out',
};
