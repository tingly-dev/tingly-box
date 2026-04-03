import { Box, Drawer, IconButton, Popover, Typography } from '@mui/material';
import { Menu as MenuIcon } from '@mui/icons-material';
import { useEffect, useMemo, useState } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useVersion as useAppVersion } from '../contexts/VersionContext';
import { Z_INDEX } from '../constants/zIndex';
import { activityBarWidth, sidebarWidth } from './constants';
import { ActivityBar } from './ActivityBar';
import { Sidebar } from './Sidebar';
import { useActivityItems } from './useActivityItems.tsx';
import type { ActivityItem, LayoutProps } from './types';

const Layout = ({ children }: LayoutProps) => {
    const location = useLocation();
    const navigate = useNavigate();
    const { currentVersion } = useAppVersion();
    const [mobileOpen, setMobileOpen] = useState(false);
    const [easterEggAnchorEl, setEasterEggAnchorEl] = useState<HTMLElement | null>(null);

    const activityItems = useActivityItems();

    const isActive = (path: string) => location.pathname === path;
    const isChildActive = (children?: ActivityItem['children']) =>
        children?.some(item => item.type !== 'divider' && isActive(item.path)) ?? false;

    // Determine active activity from current path, falling back to sessionStorage
    const activeActivity = useMemo(() => {
        for (const item of activityItems) {
            if (item.path && isActive(item.path)) return item.key;
            if (item.children && isChildActive(item.children)) return item.key;
        }
        const saved = sessionStorage.getItem('layout.activeActivity');
        if (saved && activityItems.some(item => item.key === saved)) return saved;
        return 'dashboard';
    }, [activityItems, location.pathname]);

    // Persist active activity + last visited path
    useEffect(() => {
        sessionStorage.setItem('layout.activeActivity', activeActivity);
        sessionStorage.setItem(`layout.activityPath.${activeActivity}`, location.pathname);
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
        setMobileOpen(false);
        sessionStorage.setItem('layout.activeActivity', item.key);
        const firstNavChild = item.children?.find(c => c.type !== 'divider');
        const targetPath =
            item.path ||
            sessionStorage.getItem(`layout.activityPath.${item.key}`) ||
            firstNavChild?.path;
        if (targetPath) navigate(targetPath);
    };

    const navigationContent = (
        <Box sx={{ display: 'flex', height: '100%' }}>
            <ActivityBar
                activityItems={activityItems}
                activeActivity={activeActivity}
                onActivityClick={handleActivityClick}
                onUserClick={(e) => setEasterEggAnchorEl(e.currentTarget)}
            />
            {sidebarItems.length > 0 && (
                <Sidebar
                    sidebarItems={sidebarItems}
                    activeActivityLabel={activeActivityLabel}
                    onClose={() => setMobileOpen(false)}
                />
            )}
        </Box>
    );

    return (
        <Box sx={{ display: 'flex', height: '100vh', overflow: 'hidden', position: 'relative', zIndex: Z_INDEX.main }}>
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

            {/* Mobile toggle */}
            <IconButton
                color="primary"
                aria-label="Open navigation menu"
                onClick={() => setMobileOpen(!mobileOpen)}
                sx={{
                    display: { xs: 'flex', md: 'none' },
                    position: 'fixed',
                    top: 12,
                    left: 12,
                    zIndex: Z_INDEX.mobileToggle,
                    boxShadow: 3,
                    width: 44,
                    height: 44,
                    '&:hover': { bgcolor: 'action.hover', transform: 'scale(1.05)' },
                    transition: 'all 0.15s',
                }}
            >
                <MenuIcon />
            </IconButton>

            {/* Main content */}
            <Box
                component="main"
                sx={{ flexGrow: 1, height: '100vh', display: 'flex', flexDirection: 'column', overflowX: 'hidden', position: 'relative', zIndex: 1 }}
            >
                <Box
                    sx={{
                        flex: 1,
                        p: 3,
                        overflowY: 'auto',
                        scrollBehavior: 'smooth',
                        '&::-webkit-scrollbar': { width: 8 },
                        '&::-webkit-scrollbar-track': { backgroundColor: 'grey.100', borderRadius: 1 },
                        '&::-webkit-scrollbar-thumb': {
                            backgroundColor: 'grey.300',
                            borderRadius: 1,
                            '&:hover': { backgroundColor: 'grey.400' },
                        },
                    }}
                >
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
                sx={{ zIndex: Z_INDEX.popover, '& .MuiPopover-paper': { bgcolor: 'primary.main', color: 'white', borderRadius: 2, px: 2, py: 1, fontSize: '0.875rem' } }}
            >
                Hi, I'm Tingly-Box, Your Smart AI Orchestrator · {currentVersion}
            </Popover>
        </Box>
    );
};

export default Layout;
