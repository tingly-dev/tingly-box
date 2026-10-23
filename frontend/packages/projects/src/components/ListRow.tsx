import type { HTMLAttributes, ReactNode } from 'react';
import { Box, Typography } from '@mui/material';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'title' | 'onClick'> {
    /** The state or kind of the row, in a fixed slot at the left. */
    mark?: ReactNode;
    title: ReactNode;
    /** The line under the title: what it waits on, what it did. */
    subtitle?: ReactNode;
    /** Small pills about its work, before the meta. */
    badges?: ReactNode;
    /** When it last moved, at the right. */
    meta?: ReactNode;
    /** Controls at the far right (a menu, a chevron). */
    end?: ReactNode;
    selected?: boolean;
    /** Changed since the user last looked: the title goes bolder. */
    unseen?: boolean;
    /** Draw it as a bordered card (a column of the Overview band). */
    outlined?: boolean;
    onClick?: () => void;
}

/**
 * The one row of Projects: a mark slot, a title with an optional subtitle,
 * then badges, meta and trailing controls. Threads, sessions, subagents and
 * background tasks all use it, so the same level of information sits in the
 * same place with the same type.
 */
export function ListRow({ mark, title, subtitle, badges, meta, end, selected, unseen, outlined, onClick, sx, ...rest }: Props & { sx?: object }) {
    return (
        <Box
            role={onClick ? 'button' : undefined}
            tabIndex={onClick ? 0 : undefined}
            aria-current={selected ? 'page' : undefined}
            onClick={onClick}
            onKeyDown={onClick ? (e) => { if (e.target === e.currentTarget && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); onClick(); } } : undefined}
            {...rest}
            sx={{
                display: 'flex', alignItems: 'center', gap: 1, minWidth: 0, px: 1.5, py: 1, borderRadius: radius.row, outline: 'none',
                cursor: onClick ? 'pointer' : 'default',
                ...(outlined && { px: 1.25, py: 0.75, border: 1, borderColor: 'divider' }),
                bgcolor: selected ? 'action.selected' : undefined,
                '&:hover': { bgcolor: selected ? 'action.selected' : 'action.hover' },
                '&:focus-visible': { boxShadow: (t) => `inset 0 0 0 2px ${t.palette.primary.main}` },
                ...sx,
            }}
        >
            {mark != null && <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: 16, flexShrink: 0, color: 'text.secondary' }}>{mark}</Box>}
            <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography noWrap component="div" sx={{ fontSize: fontSizes.base, fontWeight: unseen ? 650 : 500, color: 'text.primary' }}>{title}</Typography>
                {subtitle != null && <Typography noWrap component="div" sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{subtitle}</Typography>}
            </Box>
            {badges}
            {meta != null && <Typography component="span" sx={{ flexShrink: 0, fontSize: fontSizes.sm, color: 'text.disabled', minWidth: 28, textAlign: 'right' }}>{meta}</Typography>}
            {end}
        </Box>
    );
}
