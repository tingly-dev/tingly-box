import { Suspense, useState } from 'react';
import { Box, Drawer, IconButton, LinearProgress, Tooltip, useMediaQuery, useTheme } from '@mui/material';
import { Outlet } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { SidebarExpand } from '../components/icons';
import { readLocal, writeLocal } from '../utils/storage';
import { Sidebar } from './Sidebar';

export function Layout() {
    const theme = useTheme();
    const { t } = useTranslation();
    const mobile = useMediaQuery(theme.breakpoints.down('md'));
    const [collapsed, setCollapsed] = useState(() => readLocal('sidebarCollapsed') === '1');
    const [mobileOpen, setMobileOpen] = useState(false);

    const setCollapsedPersist = (v: boolean) => {
        setCollapsed(v);
        writeLocal('sidebarCollapsed', v ? '1' : '0');
    };

    const showExpand = mobile || collapsed;
    return (
        <Box sx={{ display: 'flex', height: '100dvh', bgcolor: 'background.default', overflow: 'hidden' }}>
            {mobile ? (
                <Drawer open={mobileOpen} onClose={() => setMobileOpen(false)}>
                    <Sidebar onNavigate={() => setMobileOpen(false)} />
                </Drawer>
            ) : (
                !collapsed && <Sidebar onCollapse={() => setCollapsedPersist(true)} />
            )}
            <Box component="main" sx={{ flex: 1, minWidth: 0, position: 'relative', display: 'flex', flexDirection: 'column' }}>
                {showExpand && (
                    <Tooltip title={t('nav.expand')}>
                        <IconButton
                            onClick={() => (mobile ? setMobileOpen(true) : setCollapsedPersist(false))}
                            sx={{ position: 'absolute', top: 10, left: 10, zIndex: 2 }}
                            size="small"
                            aria-label={t('nav.expand')}
                        >
                            <SidebarExpand fontSize="small" />
                        </IconButton>
                    </Tooltip>
                )}
                <Suspense fallback={<LinearProgress sx={{ position: 'absolute', top: 0, left: 0, right: 0 }} />}>
                    <Outlet />
                </Suspense>
            </Box>
        </Box>
    );
}
