// Centralized style objects for the layout chrome. Numeric sizing lives in
// `./constants.ts`; this file holds the sx objects / factory functions that
// were previously inlined across Layout.tsx, Sidebar.tsx, ActivityBar.tsx and
// constants.ts. Grouped by surface.

import {
    activityBottomItemMinHeight,
    activityContainerPaddingY,
    activityItemGap,
    activityItemMinHeight,
    activityItemPaddingX,
    activityItemPaddingY,
    activityItemRadius,
    headerHeight,
} from './constants';
import { Z_INDEX } from '../constants/zIndex';
import type { SxProps } from '@mui/material';
import type { Theme } from '@mui/material/styles';

// ---------------------------------------------------------------------------
// Layout — main content area
// ---------------------------------------------------------------------------

export const mobileContentSx = {
    flex: 1,
    px: { xs: 2, md: 3 },
    pt: { xs: 9, md: 3 },
    pb: 3,
    overflowY: 'auto',
    scrollBehavior: 'smooth',
    '&::-webkit-scrollbar': { width: 8 },
    '&::-webkit-scrollbar-track': { backgroundColor: 'grey.100', borderRadius: 1 },
    '&::-webkit-scrollbar-thumb': {
        backgroundColor: 'grey.300',
        borderRadius: 1,
        '&:hover': { backgroundColor: 'grey.400' },
    },
} as const;

export const mobileNavigationBarSx = {
    display: { xs: 'flex', md: 'none' },
    position: 'fixed',
    top: 0,
    left: 0,
    right: 0,
    height: 56,
    zIndex: Z_INDEX.mobileToggle,
    alignItems: 'center',
    px: 1,
    bgcolor: 'background.paper',
    borderBottom: '1px solid',
    borderColor: 'divider',
} as const;

export const mobileMenuButtonSx = {
    width: 44,
    height: 44,
    '&:hover': { bgcolor: 'action.hover' },
} as const;

// ---------------------------------------------------------------------------
// ActivityBar — primary icon rail
// ---------------------------------------------------------------------------

/** A primary activity entry in the ActivityBar (icon + label stacked). */
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

/** Shared sizing for the ActivityBar's icon-only bottom buttons
 *  (feedback / language / theme / user / collapse toggle). Every bottom
 *  button is the same height; only the active/hover tint differs, which the
 *  caller passes via `extra`. */
export const activityBottomItemSx = (extra?: Record<string, unknown>) => ({
    minHeight: activityBottomItemMinHeight,
    mx: 0.5,
    px: activityItemPaddingX,
    py: 0.75,
    flexDirection: 'column' as const,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 0.25,
    position: 'relative' as const,
    color: 'text.secondary',
    borderRadius: activityItemRadius,
    cursor: 'pointer',
    ...extra,
});

/** Wrapper Box for a bottom-of-rail button cluster (feedback / language / theme / collapse). */
export const activityBottomClusterSx: SxProps<Theme> = {
    py: 0.5,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    flexShrink: 0,
};

/** Scrollable activity list. The native scrollbar is hidden on purpose: on
 *  platforms with non-overlay scrollbars it eats ~15px of the 88px rail, which
 *  shifts the centered icons left and misaligns them with the fixed bottom
 *  cluster. Overflow is hinted instead by scroll shadows (Lea Verou's
 *  `background-attachment: local` trick) that only appear at an edge when
 *  there is more content beyond it. */
export const activityIconsScrollSx: SxProps<Theme> = (theme) => {
    const paper = theme.palette.background.paper;
    const shadow = theme.palette.mode === 'dark' ? 'rgba(0,0,0,0.5)' : 'rgba(0,0,0,0.12)';
    return {
        flex: 1,
        minHeight: 0,
        py: activityContainerPaddingY,
        overflowY: 'auto',
        overflowX: 'hidden',
        scrollbarWidth: 'none',
        '&::-webkit-scrollbar': { display: 'none' },
        background: `
            linear-gradient(${paper} 30%, transparent) center top,
            linear-gradient(transparent, ${paper} 70%) center bottom,
            radial-gradient(farthest-side at 50% 0, ${shadow}, transparent) center top,
            radial-gradient(farthest-side at 50% 100%, ${shadow}, transparent) center bottom
        `,
        backgroundRepeat: 'no-repeat',
        backgroundSize: '100% 24px, 100% 24px, 100% 10px, 100% 10px',
        backgroundAttachment: 'local, local, scroll, scroll',
    };
};

export const activityRailSx: SxProps<Theme> = {
    position: 'relative',
    height: '100%',
    display: 'flex',
    flexDirection: 'column',
    bgcolor: 'background.paper',
    borderRight: '1px solid',
    borderColor: 'divider',
};

/** Expand handle on the ActivityBar — only shown when the Sidebar is collapsed.
 *  Sits at the right edge of the logo cell (just under the top divider), so it
 *  vertically aligns with the collapse button in the Sidebar header: the two
 *  controls form a symmetric pair at the same height. */
export const activityExpandHandleSx: SxProps<Theme> = {
    position: 'absolute',
    top: headerHeight / 2,
    right: -13,
    transform: 'translateY(-50%)',
    width: 26,
    height: 26,
    minHeight: 26,
    border: '1px solid',
    borderColor: 'divider',
    bgcolor: 'background.paper',
    color: 'text.secondary',
    zIndex: Z_INDEX.main,
    '&:hover': {
        // Keep an opaque paper background — the handle straddles the rail
        // border, so MUI's default translucent hover overlay would show the
        // content behind it and read as the button going transparent.
        backgroundColor: 'background.paper',
        color: 'primary.main',
        borderColor: 'primary.main',
    },
};

/** The logo / brand cell at the top of the ActivityBar (fixed at headerHeight). */
export const activityLogoCellSx: SxProps<Theme> = {
    height: headerHeight,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    borderBottom: '1px solid',
    borderColor: 'divider',
};

export const activityLogoButtonSx: SxProps<Theme> = {
    width: 36,
    height: 36,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    textDecoration: 'none',
    cursor: 'pointer',
    transition: 'opacity 0.18s ease-out',
    '&:hover': { opacity: 0.82 },
};

// ---------------------------------------------------------------------------
// Sidebar — secondary nav list
// ---------------------------------------------------------------------------

/** Shared sizing for the sidebar's nav-style rows, kept here so every row is
 *  the same height whether or not it happens to carry a subtitle. `mx` is part
 *  of the row geometry (inset from the rail edge), so it lives here too.
 *  minHeight is sized to comfortably fit two text lines (primary 14px +
 *  secondary 11px ≈ 31px) with breathing room above and below. */
export const NAV_ROW_SX = {
    mx: 1,
    minHeight: 52,
    borderRadius: 1.25,
    py: 1,
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

export const sidebarContainerSx: SxProps<Theme> = {
    height: '100%',
    display: 'flex',
    flexDirection: 'column',
    bgcolor: 'background.paper',
    borderRight: '1px solid',
    borderColor: 'divider',
    overflow: 'hidden',
};

export const sidebarHeaderSx: SxProps<Theme> = {
    height: headerHeight,
    px: 2,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 1,
    borderBottom: '1px solid',
    borderColor: 'divider',
};

export const sidebarListScrollSx: SxProps<Theme> = {
    flex: 1,
    py: 1,
    overflowY: 'auto',
    '&::-webkit-scrollbar': { width: 6 },
    '&::-webkit-scrollbar-track': { backgroundColor: 'transparent' },
    '&::-webkit-scrollbar-thumb': {
        backgroundColor: 'grey.300',
        borderRadius: 1,
        '&:hover': { backgroundColor: 'grey.400' },
    },
};
