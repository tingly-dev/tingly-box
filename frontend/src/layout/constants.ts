export const activityBarWidth = 88;
export const sidebarWidth = 200;
export const headerHeight = 60;
export const footerHeight = 60;

// The main content area's own padding (Layout.tsx's mobileContentSx) —
// exported so a page-level secondary nav (SecondaryNavLayout) can cancel it
// with matching negative margins and sit flush against the real Sidebar,
// instead of guessing/duplicating these numbers.
export const contentPaddingX = { xs: 2, md: 3 } as const;
export const contentPaddingTop = { xs: 9, md: 3 } as const;
export const contentPaddingBottom = 3;

// --- Activity Bar Item Styles ---
export const activityItemMinHeight = 64;
export const activityItemGap = 0.5;
export const activityItemRadius = 1.25;
export const activityItemPaddingX = 1;
export const activityItemPaddingY = 1.25;
export const activityContainerPaddingY = 1;

export const activityItemSx = (extra?: Record<string, unknown>) => ({
    minHeight: activityItemMinHeight,
    mx: 0.5,
    px: activityItemPaddingX,
    py: activityItemPaddingY,
    flexDirection: 'column' as const,
    alignItems: 'center',
    justifyContent: 'center',
    gap: activityItemGap,
    position: 'relative' as const,
    color: 'text.secondary',
    borderRadius: activityItemRadius,
    cursor: 'pointer',
    ...extra,
});
