import {
    AccountCircle as AccountIcon,
    Dashboard as DashboardIcon,
    Key as KeyIcon,
    Menu as MenuIcon,
    Settings as SystemIcon,
    ExpandLess,
    ExpandMore,
    BarChart as BarChartIcon,
    CloudUpload,
    Refresh,
    CheckCircle,
    Error as ErrorIcon,
    Error as VersionIcon,
} from '@mui/icons-material';
import LockIcon from '@mui/icons-material/Lock';
import {
    Box,
    Collapse,
    Drawer,
    IconButton,
    List,
    ListItem,
    ListItemButton,
    ListItemIcon,
    ListItemText,
    Typography,
    CircularProgress,
    Popover,
} from '@mui/material';
import type { ReactNode } from 'react';
import React, { useState } from 'react';
import { Link as RouterLink, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useHealth } from '../contexts/HealthContext';
import { useVersion as useAppVersion } from '../contexts/VersionContext';
import VerifiedIcon from '@mui/icons-material/Verified';
import OpenAI from '@lobehub/icons/es/OpenAI';
import Anthropic from '@lobehub/icons/es/Anthropic';
import Claude from '@lobehub/icons/es/Claude';

interface LayoutProps {
    children: ReactNode;
}

const drawerWidth = 260;

interface MenuItem {
    path: string;
    label: string;
    icon: ReactNode;
}

interface MenuGroup {
    key: string;
    label?: string;
    icon?: ReactNode;
    standalone?: boolean;
    items: MenuItem[];
}

const Layout = ({ children }: LayoutProps) => {
    const { t } = useTranslation();
    const location = useLocation();
    const { isHealthy, checking, checkHealth } = useHealth();
    const { hasUpdate, checking: checkingVersion, checkForUpdates, currentVersion, latestVersion } = useAppVersion();
    const [mobileOpen, setMobileOpen] = useState(false);
    const [homeMenuOpen, setHomeMenuOpen] = useState(true);
    const [credentialMenuOpen, setCredentialMenuOpen] = useState(true);
    const [easterEggAnchorEl, setEasterEggAnchorEl] = useState<HTMLElement | null>(null);

    const handleDrawerToggle = () => {
        setMobileOpen(!mobileOpen);
    };

    const handleEasterEgg = (event: React.MouseEvent<HTMLElement>) => {
        setEasterEggAnchorEl(event.currentTarget);
    };

    const handleEasterEggClose = () => {
        setEasterEggAnchorEl(null);
    };

    const handleCheckUpdates = () => {
        checkForUpdates(true);
    };

    const isActive = (path: string) => {
        return location.pathname === path;
    };

    const isGroupActive = (items: MenuItem[]) => {
        return items.some(item => isActive(item.path));
    };

    const menuGroups: MenuGroup[] = [
        {
            key: 'dashboard',
            label: 'Dashboard',
            icon: <BarChartIcon sx={{ fontSize: 20 }} />,
            standalone: true,
            items: [
                {
                    path: '/dashboard',
                    label: 'Usage Dashboard',
                    icon: <BarChartIcon sx={{ fontSize: 20 }} />,
                },
            ],
        },
        {
            key: 'scenario',
            label: t('layout.nav.home'),
            icon: <DashboardIcon sx={{ fontSize: 20 }} />,
            items: [
                {
                    path: '/use-openai',
                    label: t('layout.nav.useOpenAI', { defaultValue: 'OpenAI' }),
                    icon: <OpenAI size={20} />,
                },
                {
                    path: '/use-anthropic',
                    label: t('layout.nav.useAnthropic', { defaultValue: 'Anthropic' }),
                    icon: <Anthropic size={20} />,
                },
                {
                    path: '/use-claude-code',
                    label: t('layout.nav.useClaudeCode', { defaultValue: 'Claude Code' }),
                    icon: <Claude size={20} />,
                },
            ],
        },
        {
            key: 'credential',
            label: t('layout.nav.credential'),
            icon: <LockIcon sx={{ fontSize: 20 }} />,
            items: [
                {
                    path: '/api-keys',
                    label: t('layout.nav.apiKeys', { defaultValue: 'API Keys' }),
                    icon: <KeyIcon sx={{ fontSize: 20 }} />,
                },
                {
                    path: '/oauth',
                    label: t('layout.nav.oauth', { defaultValue: 'OAuth' }),
                    icon: <VerifiedIcon sx={{ fontSize: 20 }} />,
                },
            ],
        },
        {
            key: 'system',
            label: 'System',
            icon: <SystemIcon sx={{ fontSize: 20 }} />,
            standalone: true,
            items: [
                {
                    path: '/system',
                    label: 'System',
                    icon: <SystemIcon sx={{ fontSize: 20 }} />,
                },
            ],
        },
    ];

    const drawerContent = (
        <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
            {/* Logo Section */}
            <Box
                component="a"
                href="https://github.com/tingly-dev/tingly-box"
                target="_blank"
                rel="noopener noreferrer"
                sx={{
                    p: 3,
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                    display: 'flex',
                    alignItems: 'center',
                    gap: 2,
                    textDecoration: 'none',
                    cursor: 'pointer',
                    '&:hover': {
                        opacity: 0.8,
                    },
                }}
            >
                <Box
                    sx={{
                        width: 40,
                        height: 40,
                        borderRadius: 2,
                        background: 'linear-gradient(135deg, #1a1a1a 0%, #2d2d2d 100%)',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: 'white',
                        fontWeight: 'bold',
                        fontSize: '1.2rem',
                    }}
                >
                    T
                </Box>
                <Typography variant="h6" sx={{ fontWeight: 600, color: 'text.primary' }}>
                    {t('layout.appTitle')}
                </Typography>
            </Box>

            {/* Navigation Menu */}
            <List sx={{ flex: 1, py: 2 }}>
                {menuGroups.map((group) => {
                    const isHomeGroup = group.key === 'home';
                    const isCredentialGroup = group.key === 'credential';
                    const isStandalone = group.standalone;

                    // For standalone groups (like System), no collapse state
                    const isOpen = isStandalone ? true : (isHomeGroup ? homeMenuOpen : credentialMenuOpen);
                    const setIsOpen = isStandalone ? undefined : (isHomeGroup ? setHomeMenuOpen : setCredentialMenuOpen);

                    return (
                        <React.Fragment key={group.key}>
                            {/* Group Header */}
                            <ListItem key={`header-${group.key}`} disablePadding sx={{ mb: isStandalone ? 0 : 1 }}>
                                <ListItemButton
                                    component={isStandalone ? RouterLink : undefined}
                                    to={isStandalone ? group.items[0].path : undefined}
                                    onClick={isStandalone ? handleDrawerToggle : () => setIsOpen && setIsOpen(!isOpen)}
                                    sx={{
                                        mx: 2,
                                        borderRadius: 2,
                                        color: isGroupActive(group.items) ? 'primary.main' : 'text.primary',
                                        bgcolor: isGroupActive(group.items) ? 'primary.50' : 'transparent',
                                    }}
                                >
                                    <ListItemIcon
                                        sx={{ color: isGroupActive(group.items) ? 'primary.main' : 'text.secondary' }}
                                    >
                                        {group.icon}
                                    </ListItemIcon>
                                    <ListItemText
                                        primary={group.label || group.items[0].label}
                                        primaryTypographyProps={{
                                            fontWeight: isGroupActive(group.items) ? 600 : 400,
                                        }}
                                    />
                                    {!isStandalone && (isOpen ? <ExpandLess /> : <ExpandMore />)}
                                </ListItemButton>
                            </ListItem>

                            {/* Group Items - only for non-standalone groups */}
                            {!isStandalone && (
                                <Collapse in={isOpen} timeout="auto" unmountOnExit>
                                    <List sx={{ pl: 0, py: 0 }}>
                                        {group.items.map((item) => (
                                            <ListItem key={item.path} disablePadding sx={{ mb: 0.5 }}>
                                                <ListItemButton
                                                    component={RouterLink}
                                                    to={item.path}
                                                    onClick={handleDrawerToggle}
                                                    className={isActive(item.path) ? 'nav-item-active' : ''}
                                                    sx={{
                                                        mx: 2,
                                                        borderRadius: 2,
                                                        pl: 5,
                                                        pr: 3,
                                                        color: isActive(item.path) ? 'primary.main' : 'text.primary',
                                                        bgcolor: isActive(item.path) ? 'primary.50' : 'transparent',
                                                        '&:hover': {
                                                            backgroundColor: isActive(item.path) ? 'primary.50' : 'action.hover',
                                                        },
                                                        '& .MuiListItemIcon-root': {
                                                            color: isActive(item.path) ? 'primary.main' : 'text.secondary',
                                                        },
                                                    }}
                                                >
                                                    <ListItemIcon sx={{ minWidth: 32 }}>
                                                        {item.icon}
                                                    </ListItemIcon>
                                                    <ListItemText
                                                        primary={item.label}
                                                        primaryTypographyProps={{
                                                            fontWeight: isActive(item.path) ? 600 : 400,
                                                            fontSize: '0.875rem',
                                                        }}
                                                    />
                                                </ListItemButton>
                                            </ListItem>
                                        ))}
                                    </List>
                                </Collapse>
                            )}
                        </React.Fragment>
                    );
                })}
            </List>

            {/* Status Section - Health & Version */}
            <Box
                sx={{
                    p: 2,
                    borderTop: '1px solid',
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                }}
            >
                {/* Connection Status */}
                <Box sx={{ mb: 2 }}>
                    <Box
                        sx={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'space-between',
                            mb: 1,
                        }}
                    >
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                            {checking ? (
                                <CircularProgress size={16} thickness={2} />
                            ) : isHealthy ? (
                                <CheckCircle color="success" sx={{ fontSize: 18 }} />
                            ) : (
                                <ErrorIcon color="error" sx={{ fontSize: 18 }} />
                            )}
                            <Typography variant="body2" color="text.secondary">
                                Server: <span/>
                                {checking ? t('health.checking') : isHealthy ? t('health.connected') : t('health.disconnected')}
                            </Typography>
                        </Box>
                        <IconButton size="small" onClick={checkHealth} disabled={checking}>
                            <Refresh sx={{ fontSize: 16 }} />
                        </IconButton>
                    </Box>
                </Box>

                {/* Version Status */}
                <Box>
                    <Box
                        sx={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'space-between',
                        }}
                    >
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                            {hasUpdate? (
                                <CloudUpload color="info" sx={{ fontSize: 18 }} />
                            ): <CheckCircle color="success" sx={{ fontSize: 18 }} />}
                            <Typography variant="body2" color="text.secondary">
                                Version: <span/>
                                {hasUpdate
                                    ? t('update.versionAvailable', { latest: latestVersion, current: currentVersion })
                                    : currentVersion
                                }
                            </Typography>
                        </Box>
                        <IconButton size="small" onClick={handleCheckUpdates} disabled={checkingVersion}>
                            {checkingVersion ? (
                                <CircularProgress size={16} thickness={2} />
                            ) : (
                                <Refresh sx={{ fontSize: 16 }} />
                            )}
                        </IconButton>
                    </Box>
                </Box>
            </Box>

            {/* Bottom Section - Slogan and User */}
            <Box
                sx={{
                    p: 2,
                    borderTop: '1px solid',
                    borderColor: 'divider',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 1,
                }}
            >
                <Typography
                    variant="body2"
                    sx={{
                        color: 'text.secondary',
                        fontSize: '0.75rem',
                        textAlign: 'center',
                        fontStyle: 'italic',
                    }}
                >
                    {t('layout.slogan')}
                </Typography>
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <IconButton
                        color="inherit"
                        onClick={handleEasterEgg}
                        sx={{ color: 'text.secondary' }}
                    >
                        <AccountIcon sx={{ fontSize: 24 }} />
                    </IconButton>
                </Box>
            </Box>
        </Box>
    );

    return (
        <Box sx={{ display: 'flex', minHeight: '100vh', backgroundColor: '#f8f9fa' }}>
            {/* Mobile Menu Button */}
            <IconButton
                color="inherit"
                aria-label="open drawer"
                edge="start"
                onClick={handleDrawerToggle}
                sx={{
                    position: 'fixed',
                    top: 16,
                    left: 16,
                    zIndex: 1300,
                    display: { sm: 'none' },
                    backgroundColor: 'background.paper',
                    boxShadow: 1,
                }}
            >
                <MenuIcon />
            </IconButton>

            {/* Sidebar Drawer */}
            <Box component="nav" sx={{ width: { sm: drawerWidth }, flexShrink: { sm: 0 } }}>
                <Drawer
                    variant="temporary"
                    open={mobileOpen}
                    onClose={handleDrawerToggle}
                    ModalProps={{
                        keepMounted: true,
                    }}
                    sx={{
                        display: { xs: 'block', sm: 'none' },
                        '& .MuiDrawer-paper': {
                            boxSizing: 'border-box',
                            width: drawerWidth,
                            backgroundColor: 'background.paper',
                            borderRight: '1px solid',
                            borderColor: 'divider',
                        },
                    }}
                >
                    {drawerContent}
                </Drawer>
                <Drawer
                    variant="permanent"
                    sx={{
                        display: { xs: 'none', sm: 'block' },
                        '& .MuiDrawer-paper': {
                            boxSizing: 'border-box',
                            width: drawerWidth,
                            backgroundColor: 'background.paper',
                            borderRight: '1px solid',
                            borderColor: 'divider',
                        },
                    }}
                    open
                >
                    {drawerContent}
                </Drawer>
            </Box>

            {/* Main Content */}
            <Box
                component="main"
                sx={{
                    flexGrow: 1,
                    width: { sm: `calc(100% - ${drawerWidth}px)` },
                    height: '100vh',
                    display: 'flex',
                    flexDirection: 'column',
                }}
            >
                <Box
                    sx={{
                        flex: 1,
                        p: 3,
                        overflowY: 'auto',
                        scrollBehavior: 'smooth',
                        '&::-webkit-scrollbar': {
                            width: 8,
                        },
                        '&::-webkit-scrollbar-track': {
                            backgroundColor: 'grey.100',
                            borderRadius: 1,
                        },
                        '&::-webkit-scrollbar-thumb': {
                            backgroundColor: 'grey.300',
                            borderRadius: 1,
                            '&:hover': {
                                backgroundColor: 'grey.400',
                            },
                        },
                    }}
                >
                    {children}
                </Box>
            </Box>

            {/* Easter Egg Popover */}
            <Popover
                open={Boolean(easterEggAnchorEl)}
                anchorEl={easterEggAnchorEl}
                onClose={handleEasterEggClose}
                anchorOrigin={{
                    vertical: 'top',
                    horizontal: 'center',
                }}
                transformOrigin={{
                    vertical: 'bottom',
                    horizontal: 'center',
                }}
                sx={{
                    '& .MuiPopover-paper': {
                        bgcolor: 'primary.main',
                        color: 'white',
                        borderRadius: 2,
                        px: 2,
                        py: 1,
                        fontSize: '0.875rem',
                    },
                }}
            >
                Hi, I'm Tingly-Box, your Smart AI Proxy
            </Popover>
        </Box>
    );
};

export default Layout;
