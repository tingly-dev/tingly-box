import { Box } from '@mui/material';
import type { ReactNode } from 'react';
import { sidebarWidth, headerHeight, contentPaddingX, contentPaddingTop, contentPaddingBottom } from '@/layout/constants';

interface SecondaryNavLayoutProps {
    /** The page-local nav (e.g. PlatformSideNav) — rendered as a white,
     * bordered column flush against the real Sidebar's right edge. */
    nav: ReactNode;
    children: ReactNode;
}

// A page that needs its own second-level nav (Overview/Remote's platform
// picker today) renders inside the main content area, which has its own
// padding — so a plain flex row next to that nav reads as a separate
// floating widget, not part of the app's navigation chrome. This cancels
// that padding on the left/top/bottom with matching negative margins and
// gives the nav column the same white background + right border as the
// real Sidebar, so it reads as a direct continuation of it (desktop only —
// on mobile the real Sidebar collapses into a drawer, so there's nothing to
// align with; the nav just stacks above the content instead).
const SecondaryNavLayout: React.FC<SecondaryNavLayoutProps> = ({ nav, children }) => (
    <Box
        sx={{
            display: 'flex',
            flexDirection: { xs: 'column', md: 'row' },
            minHeight: '100%',
            mx: { xs: -contentPaddingX.xs, md: -contentPaddingX.md },
            mt: { xs: -contentPaddingTop.xs, md: -contentPaddingTop.md },
            mb: -contentPaddingBottom,
        }}
    >
        <Box
            sx={{
                width: { xs: '100%', md: sidebarWidth },
                flexShrink: 0,
                // Fixed in place while the content column scrolls, same as
                // the real Sidebar (which doesn't scroll at all — it's
                // outside the scrolling container entirely).
                position: { md: 'sticky' },
                top: { md: 0 },
                alignSelf: { md: 'flex-start' },
                bgcolor: { md: 'background.paper' },
                // Explicit longhand, not the `border` shorthand: mixing a
                // colorless shorthand with a separate borderColor risks the
                // shorthand's implicit "currentcolor" winning over the
                // intended divider tone, which read as a stray black edge.
                borderRightWidth: { md: 1 },
                borderRightStyle: { md: 'solid' },
                borderRightColor: 'divider',
                borderBottomWidth: { xs: 1, md: 0 },
                borderBottomStyle: { xs: 'solid', md: 'none' },
                borderBottomColor: 'divider',
            }}
        >
            {/* Matches the real Sidebar's header row height, so this list's
                rows land on the exact same horizontal lines as the Sidebar's
                — the two read as one continuous menu, not two menus that
                happen to be next to each other. */}
            <Box sx={{ display: { xs: 'none', md: 'block' }, height: headerHeight, borderBottom: '1px solid', borderColor: 'divider' }} />
            <Box sx={{ px: { xs: contentPaddingX.xs, md: 1.5 }, pt: { xs: contentPaddingTop.xs, md: 1.5 }, pb: { xs: 1.5, md: contentPaddingBottom } }}>
                {nav}
            </Box>
        </Box>
        <Box
            sx={{
                flex: 1,
                minWidth: 0,
                px: contentPaddingX,
                pt: { xs: 2, md: contentPaddingTop },
                pb: contentPaddingBottom,
            }}
        >
            {children}
        </Box>
    </Box>
);

export default SecondaryNavLayout;
