import { Box, Drawer, IconButton, Popover, Tooltip, Stack } from '@mui/material';
import { Menu as IconMenu, Create as IconPencil } from '@/components/icons';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useVersion as useAppVersion } from '../contexts/VersionContext';
import { Z_INDEX } from '../constants/zIndex';
import { activityBarWidth, sidebarWidth } from './constants';
import { ActivityBar } from './ActivityBar.tsx';
import { Sidebar } from './Sidebar';
import { useActivityItems } from './useActivityItems.tsx';
import type { ActivityItem, LayoutProps } from './types';
import { FloatingStatusIndicators } from '../components/FloatingStatusIndicators';

const mobileContentSx = {
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

const MobileNavigationBar = ({ onMenuClick }: { onMenuClick: () => void }) => (
    <Box
        sx={{
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
        }}
    >
        <IconButton
            color="primary"
            aria-label="Open navigation menu"
            onClick={onMenuClick}
            sx={{
                width: 44,
                height: 44,
                '&:hover': { bgcolor: 'action.hover' },
            }}
        >
            <IconMenu sx={{ fontSize: 24 }} />
        </IconButton>
    </Box>
);

const Layout = ({ children }: LayoutProps) => {
    const { t } = useTranslation();
    const location = useLocation();
    const navigate = useNavigate();
    const { currentVersion } = useAppVersion();
    const [mobileOpen, setMobileOpen] = useState(false);
    const [easterEggAnchorEl, setEasterEggAnchorEl] = useState<HTMLElement | null>(null);

    const activityItems = useActivityItems();

    const isActive = (path: string) => location.pathname === path;
    const isChildActive = (children?: ActivityItem['children']) =>
        children?.some(item => item.type !== 'divider' && isActive(item.path)) ?? false;

    // Determine active activity from current path, falling back to localStorage
    const activeActivity = useMemo(() => {
        if (location.pathname === '/onboarding') return 'onboarding';
        for (const item of activityItems) {
            if (item.path && isActive(item.path)) return item.key;
            if (item.children && isChildActive(item.children)) return item.key;
        }
        // Check if saved activity is still valid
        const saved = sessionStorage.getItem('layout.activeActivity') || localStorage.getItem('layout.activeActivity');
        if (saved && activityItems.some(item => item.key === saved)) return saved;
        // Fallback to 'scenario' (which is valid - it's the agent activity)
        return 'scenario';
    }, [activityItems, location.pathname]);

    // Persist the active activity for cross-session boot. Each activity
    // re-opens at its defaultPath (the scenario activity opens its overview).
    useEffect(() => {
        sessionStorage.setItem('layout.activeActivity', activeActivity);
        localStorage.setItem('layout.activeActivity', activeActivity);
    }, [activeActivity, location.pathname]);

    const sidebarItems = useMemo(() => {
        const activity = activityItems.find(item => item.key === activeActivity);
        return activity?.children || [];
    }, [activityItems, activeActivity]);

    const activeActivityLabel = useMemo(() => {
        const activity = activityItems.find(item => item.key === activeActivity);
        return activity?.label || '';
    }, [activityItems, activeActivity]);

    const handleActivityClick = (item: ActivityItem) => {
        const hasSidebarItems = item.children?.some(child => child.type !== 'divider') ?? false;
        if (!hasSidebarItems) {
            setMobileOpen(false);
        }

        sessionStorage.setItem('layout.activeActivity', item.key);

        // Every activity opens at its defaultPath when clicked. The scenario
        // activity points at /agent (the overview) so users always land there
        // before drilling into a specific scenario.
        const firstNavChild = item.children?.find(c => c.type !== 'divider');
        let targetPath = item.defaultPath || item.path || firstNavChild?.path;

        // Ultimate fallback to prevent navigation to invalid paths
        if (!targetPath && firstNavChild) {
            targetPath = firstNavChild.path;
        }

        if (targetPath) navigate(targetPath);
    };

    // The scenario activity exposes a quick link to manage which agents are
    // visible (the overview page hosts the show/hide controls).
    const sidebarHeaderAction = activeActivity === 'scenario' ? (
        <Stack direction="row" spacing={0.5} alignItems="center">
            <Tooltip title={t('scenarioOverview.editTooltip', { defaultValue: 'Manage visible agents' })} arrow placement="right">
                <IconButton
                    size="small"
                    onClick={() => navigate('/agent')}
                    sx={{
                        color: 'text.secondary',
                        '&:hover': { color: 'primary.main' },
                    }}
                >
                    <IconPencil sx={{ fontSize: 16 }} />
                </IconButton>
            </Tooltip>
        </Stack>
    ) : undefined;

    const navigationContent = (
        <Box sx={{ display: 'flex', height: '100%' }}>
            <ActivityBar
                activityItems={activityItems}
                activeActivity={activeActivity}
                onActivityClick={handleActivityClick}
                onUserClick={(e) => setEasterEggAnchorEl(e.currentTarget)}
                onStandaloneNavigate={() => setMobileOpen(false)}
            />
            {sidebarItems.length > 0 && (
                <Sidebar
                    sidebarItems={sidebarItems}
                    activeActivityLabel={activeActivityLabel}
                    onClose={() => setMobileOpen(false)}
                    headerAction={sidebarHeaderAction}
                />
            )}
        </Box>
    );

    return (
        <Box sx={{ display: 'flex', height: '100vh', overflow: 'hidden', position: 'relative', zIndex: Z_INDEX.main }}>
            <FloatingStatusIndicators />

            {/* Desktop nav */}
            <Box component="nav" sx={{ display: { xs: 'none', md: 'flex' }, height: '100%', position: 'relative', zIndex: Z_INDEX.drawer + 1 }}>
                {navigationContent}
            </Box>

            {/* Mobile Drawer */}
            <Drawer
                variant="temporary"
                open={mobileOpen}
                onClose={() => setMobileOpen(false)}
                ModalProps={{ keepMounted: true }}
                sx={{
                    display: { xs: 'block', md: 'none' },
                    '& .MuiDrawer-paper': {
                        boxSizing: 'border-box',
                        width: sidebarItems.length > 0 ? activityBarWidth + sidebarWidth : activityBarWidth,
                        zIndex: Z_INDEX.drawer,
                    },
                }}
            >
                {navigationContent}
            </Drawer>

            <MobileNavigationBar onMenuClick={() => setMobileOpen(!mobileOpen)} />

            {/* Main content */}
            <Box
                component="main"
                sx={{ flexGrow: 1, height: '100vh', display: 'flex', flexDirection: 'column', overflowX: 'hidden', position: 'relative', zIndex: 1 }}
            >
                <Box sx={mobileContentSx}>
                    {children ?? <Outlet />}
                </Box>
            </Box>

            {/* Easter Egg Popover */}
            <Popover
                open={Boolean(easterEggAnchorEl)}
                anchorEl={easterEggAnchorEl}
                onClose={() => setEasterEggAnchorEl(null)}
                anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
                transformOrigin={{ vertical: 'bottom', horizontal: 'center' }}
                sx={{ zIndex: Z_INDEX.popover, '& .MuiPopover-paper': { bgcolor: 'primary.main', color: 'white', borderRadius: 2, px: 2, py: 1 } }}
            >
                {t('layout.easterEgg')} · {currentVersion}
            </Popover>
        </Box>
    );
};

export default Layout;
