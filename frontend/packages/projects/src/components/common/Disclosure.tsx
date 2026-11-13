import type { ReactNode } from 'react';
import { Box } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import { ChevronRight } from '@tingly/ui/icons';
import { radius } from '@tingly/ui/theme/radius';

/** The one disclosure mark: points right when folded and turns down when open. */
export function DisclosureChevron({ open, fontSize = 16, sx }: { open: boolean; fontSize?: number; sx?: SxProps<Theme> }) {
    return (
        <ChevronRight
            aria-hidden
            sx={[
                { fontSize, flexShrink: 0, transform: open ? 'rotate(90deg)' : 'none', transition: 'transform 120ms' },
                ...(Array.isArray(sx) ? sx : [sx]),
            ]}
        />
    );
}

/**
 * Everything in Projects that folds open uses this row: the whole row is the
 * toggle (click, Enter, Space), the chevron sits at its trailing end, and the
 * row greys to text.secondary and lifts on hover.
 *
 * `header` fills the top of a card (the card keeps its own border and radius);
 * `inline` is a quiet line inside running content. Either way the row spans the full width and the chevron is pinned to its end. Without `onToggle` the row
 * is plain: no role, no chevron, no hover (a group that cannot fold).
 */
export function DisclosureRow({ open, onToggle, variant = 'header', chevron = true, color, hoverColor, children, sx, ...rest }: {
    open: boolean;
    onToggle?: () => void;
    variant?: 'header' | 'inline';
    /** False when the content places its own DisclosureChevron (a two-line header). */
    chevron?: boolean;
    /** The row's resting colour (default text.secondary); the chevron follows it. */
    color?: (theme: Theme) => string;
    hoverColor?: (theme: Theme) => string;
    children: ReactNode;
    sx?: SxProps<Theme>;
    'aria-label'?: string;
}) {
    const toggleable = onToggle != null;
    return (
        <Box
            role={toggleable ? 'button' : undefined}
            tabIndex={toggleable ? 0 : undefined}
            aria-expanded={toggleable ? open : undefined}
            onClick={onToggle}
            onKeyDown={toggleable ? (e) => {
                if (e.target === e.currentTarget && (e.key === 'Enter' || e.key === ' ')) {
                    e.preventDefault();
                    onToggle();
                }
            } : undefined}
            {...rest}
            sx={[
                {
                    display: 'flex', alignItems: 'center', gap: 0.75, minWidth: 0, outline: 'none', userSelect: 'none',
                    color: color ?? 'text.secondary',
                    cursor: toggleable ? 'pointer' : 'default',
                    ...(variant === 'header'
                        ? { px: 1.5, py: 0.75 }
                        : { py: 0.25, px: 0.5, mx: -0.5, borderRadius: radius.row }),
                    ...(toggleable && {
                        '&:hover': { bgcolor: 'action.hover', color: hoverColor ?? 'text.primary' },
                        '&:focus-visible': { boxShadow: (t: Theme) => `inset 0 0 0 2px ${t.palette.primary.main}` },
                    }),
                },
                ...(Array.isArray(sx) ? sx : [sx]),
            ]}
        >
            {children}
            {toggleable && chevron && <DisclosureChevron open={open} sx={{ ml: 'auto' }} />}
        </Box>
    );
}
