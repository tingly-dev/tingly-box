import { Box } from '@mui/material';
import type { ReactNode } from 'react';
import { sidebarWidth, contentPaddingX, contentPaddingTop, contentPaddingBottom } from '@/layout/constants';

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
// real Sidebar, so it reads as a direct continuation of it. The content
// side keeps the normal page padding restored.
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
                bgcolor: { md: 'background.paper' },
                borderRight: { md: '1px solid' },
                borderBottom: { xs: '1px solid', md: 'none' },
                borderColor: 'divider',
                px: { xs: contentPaddingX.xs, md: 1.5 },
                pt: contentPaddingTop,
                pb: { xs: 1.5, md: contentPaddingBottom },
            }}
        >
            {nav}
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
