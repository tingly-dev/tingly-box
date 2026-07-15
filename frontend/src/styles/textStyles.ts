import { type SxProps, type Theme } from '@mui/material/styles';
import { fontMono } from '@/theme/fonts';

// Shared style for clickable monospace text (URLs, tokens, commands).
export const copyableTextStyle: SxProps<Theme> = {
    fontFamily: fontMono,
    color: 'primary.main',
    cursor: 'pointer',
    boxSizing: 'border-box',
    display: 'block',
    maxWidth: '100%',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    '&:hover': {
        textDecoration: 'underline',
        backgroundColor: 'action.hover',
    },
    py: 1,
    borderRadius: 1,
    transition: 'all 0.2s ease-in-out',
};
