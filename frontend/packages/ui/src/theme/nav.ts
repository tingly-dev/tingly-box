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

/** The selected row: filled with the primary colour, its text and icon on it. */
export const navRowSelectedSx = {
    backgroundColor: 'primary.main',
    color: 'primary.contrastText',
    '&:hover': { backgroundColor: 'primary.main' },
    '& .MuiListItemIcon-root': { color: 'primary.contrastText' },
    '& .MuiListItemText-primary': { color: 'primary.contrastText' },
} as const;

/** ListItemText slotProps for a nav row: a 14px name, an 11px subtitle. */
export const navRowTextSlotProps = (active: boolean) => ({
    primary: { noWrap: true, variant: 'body2' as const, sx: { fontWeight: 500, lineHeight: 1.3, fontSize: '0.875rem' } },
    secondary: {
        variant: 'caption' as const,
        sx: {
            fontSize: '0.6875rem',
            lineHeight: 1.2,
            color: active ? 'rgba(255,255,255,0.7)' : 'text.secondary',
        },
    },
});
