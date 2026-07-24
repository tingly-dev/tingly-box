// Shared sizing for vertical nav-style rows: the global Sidebar and the
// per-page PlatformSideNav (Overview/Remote) are visually the same list —
// same icon+label(+subtitle) row — just at different nesting levels. They
// used to each hardcode their own padding/minHeight/font sizes, which drifted
// out of pixel-parity the moment one of them was tweaked. Both now import
// from here so they can't diverge again, and so every row is the same
// height whether or not it happens to carry a subtitle.
export const NAV_ROW_SX = {
    minHeight: 52,
    borderRadius: 1.25,
    py: 1.25,
    px: 2,
} as const;

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
