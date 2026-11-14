import { Box } from '@mui/material';
import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Outlet, useLocation } from 'react-router-dom';
import { MoreHoriz } from '@/components/icons';
import { useFeatureFlags } from '@/contexts/FeatureFlagsContext';
import { useActivityItems } from '@/layout/useActivityItems';
import type { LayoutProps } from '@/layout/types';
import { rememberAgentPath } from '@/pages/scenario/lastAgent';
import { syncUiPrefs } from '@/services/uiPrefs';
import { MobileTabBar } from './MobileTabBar';
import { activeTab, buildTabs } from './mobileTabs';

// Workspace pages lay themselves out edge to edge; every other page is a
// padded, scrolling column. (Same split as the desktop Layout.)
const isWorkspacePath = (path: string) => path === '/workspace' || path === '/projects' || path.startsWith('/projects/');

/** The phone's frame: the page above, three tabs below. Pages without a phone version render as they are, in one column. */
export default function MobileShell({ children }: LayoutProps) {
    const { t } = useTranslation();
    const { pathname } = useLocation();
    const { enableProjects } = useFeatureFlags();
    const items = useActivityItems();

    useEffect(() => { void syncUiPrefs(); }, []);
    useEffect(() => { rememberAgentPath(pathname); }, [pathname]);

    const tabs = useMemo(
        () => buildTabs(items, { label: t('mobile.more.tab'), icon: <MoreHoriz sx={{ fontSize: 22 }} /> }, enableProjects),
        [items, t, enableProjects],
    );
    const active = activeTab(tabs, items, pathname, sessionStorage.getItem('layout.activeActivity') || localStorage.getItem('layout.activeActivity'));

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100dvh', overflow: 'hidden', bgcolor: 'background.default' }}>
            <Box
                component="main"
                sx={isWorkspacePath(pathname)
                    ? { flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }
                    : { flex: 1, minHeight: 0, overflowX: 'hidden', overflowY: 'auto', px: 2, py: 2 }}
            >
                {children ?? <Outlet />}
            </Box>
            <MobileTabBar tabs={tabs} active={active} />
        </Box>
    );
}
