// A sidebar's nav rows: one geometry and one selected look for every list a
// sidebar shows (Tingly Box's own, the Projects sidebar), so rows are the same
// height and weight wherever they come from.

/** Row sizing. minHeight fits two text lines (primary 14px + secondary 11px)
 *  with room above and below, so rows with and without a subtitle line up.
 *  `mx` is part of the geometry (the inset from the sidebar's edge). */
export const NAV_ROW_SX = {
    mx: 1,
    minHeight: 52,
    borderRadius: 1.25,
    py: 1,
    px: 2,
} as const;

/** A thin accent bar on the edge of the selected item. `inset` is how far the
 *  item sits from the container edge, so the bar lands on the edge itself. */
const selectedBar = (inset: number) => ({
    '&::before': {
        content: '""',
        position: 'absolute',
        left: -inset,
        top: 10,
        bottom: 10,
        width: 3,
        borderRadius: 2,
        backgroundColor: 'primary.main',
    },
}) as const;

/** The selected row: a quiet tint with a thin accent bar, text and icon in the
 *  normal ink. A solid fill is left to the page's one primary action, so
 *  "where am I" and "what do I press" no longer share a colour. */
export const navRowSelectedSx = {
    backgroundColor: 'action.selected',
    color: 'text.primary',
    '&:hover': { backgroundColor: 'action.selected' },
    '& .MuiListItemIcon-root': { color: 'text.primary' },
    '& .MuiListItemText-primary': { color: 'text.primary', fontWeight: 600 },
    ...selectedBar(8),
} as const;

/** The selected item in the icon rail (same look, its own inset). */
export const navRailSelectedSx = {
    backgroundColor: 'action.selected',
    color: 'text.primary',
    '&:hover': { backgroundColor: 'action.selected', color: 'text.primary' },
    ...selectedBar(4),
} as const;

/** ListItemText slotProps for a nav row: a 14px name, an 11px subtitle. */
export const navRowTextSlotProps = (_active: boolean) => ({
    primary: { noWrap: true, variant: 'body2' as const, sx: { fontWeight: 500, lineHeight: 1.3, fontSize: '0.875rem' } },
    secondary: {
        variant: 'caption' as const,
        sx: {
            fontSize: '0.6875rem',
            lineHeight: 1.2,
            color: 'text.secondary',
        },
    },
});
