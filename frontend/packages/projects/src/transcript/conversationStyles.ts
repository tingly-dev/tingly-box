import { alpha, type Theme } from '@mui/material/styles';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

// The transcript is the reference for conversation presentation. Keep both
// Markdown parsers and their hosts independent while sharing the reading surface.
export const messageSpacing = 3;
export const userMessageWidth = '80%';

export const conversationText = {
    color: 'text.primary', fontSize: fontSizes.base, lineHeight: 1.65, whiteSpace: 'pre-wrap', wordBreak: 'break-word',
} as const;

export const userBubble = {
    maxWidth: userMessageWidth, bgcolor: (theme: Theme) => alpha(theme.palette.primary.main, theme.palette.mode === 'dark' ? 0.16 : 0.08),
    border: 1, borderColor: (theme: Theme) => alpha(theme.palette.primary.main, theme.palette.mode === 'dark' ? 0.2 : 0.1),
    px: 2, py: 1.25, borderRadius: radius.card, borderBottomRightRadius: 4,
} as const;

export const conversationMarkdown = {
    color: 'text.primary',
    fontSize: fontSizes.base,
    lineHeight: 1.65,
    wordBreak: 'break-word',
    '& p': { m: 0, mb: 1.25 },
    '& p:last-child': { mb: 0 },
    '& h1, & h2, & h3, & h4': { mt: 2, mb: 1, lineHeight: 1.35, fontWeight: 650, color: 'text.primary' },
    '& h1': { fontSize: '1.25rem' },
    '& h2': { fontSize: '1.125rem' },
    '& h3, & h4': { fontSize: '1rem' },
    '& ul, & ol': { m: 0, mb: 1.25, pl: 3 },
    '& li': { my: 0.25 },
    '& a': { color: 'primary.main' },
    '& blockquote': { m: 0, mb: 1.25, pl: 1.5, borderLeft: 3, borderColor: 'divider', color: 'text.secondary' },
    '& hr': { border: 0, borderTop: 1, borderColor: 'divider', my: 2 },
    '& table': { borderCollapse: 'collapse', mb: 1.25, fontSize: fontSizes.base, display: 'block', overflowX: 'auto' },
    '& th, & td': { border: 1, borderColor: 'divider', px: 1.25, py: 0.5, textAlign: 'left' },
    '& th': { bgcolor: 'action.hover', fontWeight: 600 },
} as const;
