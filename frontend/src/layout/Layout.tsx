import {
    AccountCircle as AccountIcon,
    Dashboard as DashboardIcon,
    Menu as MenuIcon,
    Settings as SystemIcon,
    ExpandLess,
    ExpandMore,
    BarChart as BarChartIcon,
    Code as CodeIcon,
    Psychology as PromptIcon,
    Bolt as SkillIcon,
    Send as UserPromptIcon,
    Lan as RemoteIcon,
    ChatBubble,
    NewReleases,
    ErrorOutline,
    LaptopMac,
    AutoAwesome,
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
    Popover,
    Divider,
} from '@mui/material';
import type { ReactNode } from 'react';
import React, { useState, useMemo } from 'react';
import { Link as RouterLink, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useVersion as useAppVersion } from '../contexts/VersionContext';
import { useHealth } from '../contexts/HealthContext';
import { useFeatureFlags } from '../contexts/FeatureFlagsContext';
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
    icon?: ReactNode;
    divider?: boolean;
}

interface MenuGroup {
    key: string;
    label?: string;
    path?: string;  // Optional path for group header click
    icon?: ReactNode;
    standalone?: boolean;
    items: MenuItem[];
}

const Layout = ({ children }: LayoutProps) => {
    const { t } = useTranslation();
    const location = useLocation();
    const { hasUpdate, currentVersion, showUpdateDialog } = useAppVersion();
    const { isHealthy, showDisconnectDialog } = useHealth();
    const { skillUser, skillIde, enableRemoteCoder } = useFeatureFlags();
    const [mobileOpen, setMobileOpen] = useState(false);
    const [homeMenuOpen, setHomeMenuOpen] = useState(true);
    const [promptMenuOpen, setPromptMenuOpen] = useState(true);
    const [systemMenuOpen, setSystemMenuOpen] = useState(false);
    const [remoteControlMenuOpen, setRemoteControlMenuOpen] = useState(true);
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

    const isActive = (path: string) => {
        return location.pathname === path;
    };

    const isGroupActive = (items: MenuItem[]) => {
        return items.some(item => isActive(item.path));
    };

    // Build prompt menu items based on feature flags
    const promptMenuItems = useMemo(() => {
        const items: MenuItem[] = [];
        if (skillUser) {
            items.push({
                path: '/prompt/user',
                label: 'User Request',
                icon: <UserPromptIcon sx={{ fontSize: 20 }} />,
            });
        }
        if (skillIde) {
            items.push({
                path: '/prompt/skill',
                label: 'Skills',
                icon: <SkillIcon sx={{ fontSize: 20 }} />,
            });
        }
        // // Command is always available if either skill feature is enabled
        // if (skillUser || skillIde) {
        //     items.push({
        //         path: '/prompt/command',
        //         label: 'Command',
        //         icon: <PromptIcon sx={{ fontSize: 20 }} />,
        //     });
        // }
        return items;
    }, [skillUser, skillIde]);

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
                    divider: true,
                    path: '/use-agent',
                    label: 'Claw | Agent',
                    icon: <AutoAwesome sx={{ fontSize: 20 }} />,
                },
                {
                    divider: true,
                    path: '/use-claude-code',
                    label: t('layout.nav.useClaudeCode', { defaultValue: 'Claude Code' }),
                    icon: <Claude size={20} />,
                },
                {
                    path: '/use-opencode',
                    label: t('layout.nav.useOpenCode', { defaultValue: 'OpenCode' }),
                    icon: <CodeIcon sx={{ fontSize: 20 }} />,
                },
                {
                    path: '/use-xcode',
                    label: t('layout.nav.useXcode', { defaultValue: 'Xcode' }),
                    icon: <LaptopMac sx={{ fontSize: 20 }} />,
                },
            ],
        },
        {
            key: 'credential',
            label: t('layout.nav.credential', { defaultValue: 'Credentials' }),
            icon: <LockIcon sx={{ fontSize: 20 }} />,
            standalone: true,
            items: [
                {
                    path: '/credentials',
                    label: t('layout.nav.credentials', { defaultValue: 'All Credentials' }),
                    icon: <LockIcon sx={{ fontSize: 20 }} />,
                },
            ],
        },
        ...(promptMenuItems.length > 0 ? [{
            key: 'prompt' as const,
            label: 'Prompt',
            icon: <PromptIcon sx={{ fontSize: 20 }} />,
            items: promptMenuItems,
        }] : []),
        ...(enableRemoteCoder ? [{
            key: 'remote-control' as const,
            label: 'Remote Control',
            icon: <RemoteIcon sx={{ fontSize: 20 }} />,
            items: [
                {
                    path: '/remote-control',
                    label: 'Overview',
                    icon: <RemoteIcon sx={{ fontSize: 20 }} />,
                },
                {
                    path: '/remote-control/bot',
                    label: 'IM Bot',
                    icon: <ChatBubble sx={{ fontSize: 20 }} />,
                },
                {
                    path: '/remote-control/agent',
                    label: 'Agent Assistant',
                    icon: <AutoAwesome sx={{ fontSize: 20 }} />,
                },
            ],
        }] : []),
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
        <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
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
                    flexShrink: 0,
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
                <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                    <Typography variant="h6" sx={{ fontWeight: 600, color: 'text.primary', lineHeight: 1.2 }}>
                        {t('layout.appTitle')}
                    </Typography>
                    <Typography
                        variant="caption"
                        sx={{
                            color: 'text.secondary',
                            fontSize: '0.7rem',
                        }}
                    >
                        {currentVersion}
                    </Typography>
                </Box>
            </Box>

            {/* Navigation Menu */}
            <List sx={{ flex: 1, py: 2, overflowY: 'auto', overflowX: 'hidden', '&::-webkit-scrollbar': { width: 6 }, '&::-webkit-scrollbar-track': { backgroundColor: 'transparent' }, '&::-webkit-scrollbar-thumb': { backgroundColor: 'grey.300', borderRadius: 1, '&:hover': { backgroundColor: 'grey.400' } } }}>
                {menuGroups.map((group) => {
                    const isDashboardGroup = group.key === 'dashboard';
                    const isHomeGroup = group.key === 'scenario';
                    const isCredentialGroup = group.key === 'credential';
                    const isPromptGroup = group.key === 'prompt';
                    const isSystemGroup = group.key === 'system';
                    const isRemoteCoderGroup = group.key === 'remote-control';
                    const isStandalone = group.standalone;

                    // For standalone groups (like Dashboard), no collapse state
                    let isOpen = true;
                    let setIsOpen: ((value: boolean) => void) | undefined = undefined;

                    if (!isStandalone) {
                        if (isHomeGroup) {
                            isOpen = homeMenuOpen;
                            setIsOpen = setHomeMenuOpen;
                        } else if (isPromptGroup) {
                            isOpen = promptMenuOpen;
                            setIsOpen = setPromptMenuOpen;
                        } else if (isSystemGroup) {
                            isOpen = systemMenuOpen;
                            setIsOpen = setSystemMenuOpen;
                        } else if (isRemoteCoderGroup) {
                            isOpen = remoteControlMenuOpen;
                            setIsOpen = setRemoteControlMenuOpen;
                        }
                    }

                    return (
                        <React.Fragment key={group.key}>
                            {/* Group Header */}
                            <ListItem key={`header-${group.key}`} disablePadding sx={{ mb: 0.5 }}>
                                <ListItemButton
                                    component={RouterLink}
                                    to={group.path || group.items[0].path}
                                    onClick={() => {
                                        handleDrawerToggle();
                                        // Expand the group when clicking header
                                        if (!isStandalone && !isOpen && setIsOpen) {
                                            setIsOpen(true);
                                        }
                                    }}
                                    sx={{
                                        mx: 1.5,
                                        borderRadius: 1.5,
                                        color: 'text.primary',
                                        bgcolor: 'action.hover',
                                        transition: 'all 150ms ease-in-out',
                                        '&:hover': {
                                            bgcolor: 'action.selected',
                                            '& .MuiListItemIcon-root': {
                                                color: 'primary.main',
                                            },
                                        },
                                        '&:focus-visible': {
                                            outline: 2,
                                            outlineColor: 'primary.main',
                                            outlineOffset: 2,
                                            borderRadius: 1,
                                        },
                                    }}
                                >
                                    <ListItemIcon
                                        sx={{
                                            color: 'text.secondary',
                                            minWidth: 40,
                                        }}
                                    >
                                        {group.icon}
                                    </ListItemIcon>
                                    <ListItemText
                                        primary={group.label || group.items[0].label}
                                        slotProps={{
                                            primary: {
                                                fontWeight: 600,
                                                fontSize: '0.875rem',
                                                letterSpacing: 0.15,
                                            },
                                        }}
                                    />
                                    {!isStandalone && (
                                        <Box
                                            onClick={(e) => {
                                                e.preventDefault();
                                                e.stopPropagation();
                                                setIsOpen && setIsOpen(!isOpen);
                                            }}
                                            sx={{ display: 'flex', alignItems: 'center' }}
                                            aria-expanded={isOpen}
                                            aria-controls={`${group.key}-menu`}
                                        >
                                            {isOpen ? <ExpandLess /> : <ExpandMore />}
                                        </Box>
                                    )}
                                </ListItemButton>
                            </ListItem>

                            {/* Group Items - only for non-standalone groups */}
                            {!isStandalone && (
                                <Collapse
                                    in={isOpen}
                                    timeout={250}
                                    easing={{
                                        enter: 'cubic-bezier(0.4, 0, 0.2, 1)',
                                        exit: 'cubic-bezier(0.4, 0, 0.2, 1)',
                                    }}
                                    unmountOnExit
                                >
                                    <List sx={{ pl: 0, py: 0 }}>
                                        {group.items.map((item) => (
                                            <>
                                            {item.divider && <Divider sx={{ mx: 4, my: 1 }} />}
                                            <ListItem disablePadding>
                                                <ListItemButton
                                                    component={RouterLink}
                                                    to={item.path}
                                                    onClick={handleDrawerToggle}
                                                    aria-current={isActive(item.path) ? 'page' : undefined}
                                                    sx={{
                                                        mx: 2,
                                                        mb: 0.5,
                                                        borderRadius: 1.5,
                                                        pl: 3.5,
                                                        pr: 2.5,
                                                        py: 1,
                                                        color: 'text.secondary',
                                                        transition: 'all 150ms ease-in-out',
                                                        position: 'relative',
                                                        ...(isActive(item.path) && {
                                                            backgroundColor: 'primary.main',
                                                            color: 'primary.contrastText',
                                                            fontWeight: 600,
                                                            '&::before': {
                                                                content: '""',
                                                                position: 'absolute',
                                                                left: 0,
                                                                top: '50%',
                                                                transform: 'translateY(-50%)',
                                                                width: 3,
                                                                height: '70%',
                                                                backgroundColor: 'primary.light',
                                                                borderRadius: '0 2px 2px 0',
                                                                boxShadow: '0 0 8px rgba(37, 99, 235, 0.5)',
                                                            },
                                                            '&:hover': {
                                                                backgroundColor: 'primary.dark',
                                                            },
                                                            '& .MuiListItemIcon-root': {
                                                                color: 'primary.contrastText',
                                                            },
                                                            '& .MuiListItemText-primary': {
                                                                color: 'primary.contrastText',
                                                                fontWeight: 600,
                                                            },
                                                        }),
                                                        '&:hover': {
                                                            backgroundColor: isActive(item.path) ? 'primary.dark' : 'action.hover',
                                                            transform: 'translateX(2px)',
                                                        },
                                                        '&:focus-visible': {
                                                            outline: 2,
                                                            outlineColor: 'primary.main',
                                                            outlineOffset: 2,
                                                            borderRadius: 1,
                                                        },
                                                        '& .MuiListItemIcon-root': {
                                                            color: isActive(item.path) ? 'primary.contrastText' : 'text.secondary',
                                                        },
                                                    }}
                                                >
                                                    <ListItemIcon sx={{ minWidth: 28 }}>
                                                        {item.icon}
                                                    </ListItemIcon>
                                                    <ListItemText
                                                        primary={item.label}
                                                        slotProps={{
                                                            primary: {
                                                                fontWeight: isActive(item.path) ? 600 : 400,
                                                                fontSize: '0.875rem',
                                                            },
                                                        }}
                                                    />
                                                </ListItemButton>
                                            </ListItem>
                                            </>
                                        ))}
                                    </List>
                                </Collapse>
                            )}
                        </React.Fragment>
                    );
                })}
            </List>

            {/* Bottom Section - Version Update, Health Status, Slogan and User */}
            <Box
                sx={{
                    p: 2,
                    borderTop: '1px solid',
                    borderColor: 'divider',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 1,
                    flexShrink: 0,
                }}
            >
                {/* Status Indicators */}
                <Box sx={{ display: 'flex', gap: 1 }}>
                    {/* Connection Lost Indicator */}
                    {!isHealthy && (
                        <Box
                            onClick={showDisconnectDialog}
                            sx={{
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                gap: 0.5,
                                flex: 1,
                                px: 1.5,
                                py: 0.75,
                                borderRadius: 1.5,
                                bgcolor: 'error.main',
                                color: 'error.contrastText',
                                cursor: 'pointer',
                                transition: 'all 150ms ease-in-out',
                                '&:hover': {
                                    bgcolor: 'error.dark',
                                    transform: 'scale(1.02)',
                                },
                                '&:active': {
                                    transform: 'scale(0.98)',
                                },
                            }}
                            role="button"
                            aria-label="View connection details"
                            tabIndex={0}
                            onKeyDown={(e) => {
                                if (e.key === 'Enter' || e.key === ' ') {
                                    e.preventDefault();
                                    showDisconnectDialog();
                                }
                            }}
                        >
                            <ErrorOutline sx={{ fontSize: 16 }} />
                            <Typography
                                variant="caption"
                                sx={{
                                    fontWeight: 600,
                                    fontSize: '0.7rem',
                                    textTransform: 'uppercase',
                                    letterSpacing: 0.5,
                                }}
                            >
                                Disconnected
                            </Typography>
                        </Box>
                    )}

                    {/* New Version Available Indicator - always show in dev mode */}
                    {(hasUpdate || import.meta.env.DEV) && (
                        <Box
                            onClick={showUpdateDialog}
                            sx={{
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                gap: 0.5,
                                flex: 1,
                                px: 1.5,
                                py: 0.75,
                                borderRadius: 1.5,
                                bgcolor: import.meta.env.DEV && !hasUpdate ? 'success.main' : 'info.main',
                                color: import.meta.env.DEV && !hasUpdate ? 'success.contrastText' : 'info.contrastText',
                                cursor: 'pointer',
                                transition: 'all 150ms ease-in-out',
                                '&:hover': {
                                    bgcolor: import.meta.env.DEV && !hasUpdate ? 'success.dark' : 'info.dark',
                                    transform: 'scale(1.02)',
                                },
                                '&:active': {
                                    transform: 'scale(0.98)',
                                },
                            }}
                            role="button"
                            aria-label="View update details"
                            tabIndex={0}
                            onKeyDown={(e) => {
                                if (e.key === 'Enter' || e.key === ' ') {
                                    e.preventDefault();
                                    showUpdateDialog();
                                }
                            }}
                        >
                            <NewReleases sx={{ fontSize: 16 }} />
                            <Typography
                                variant="caption"
                                sx={{
                                    fontWeight: 600,
                                    fontSize: '0.7rem',
                                    textTransform: 'uppercase',
                                    letterSpacing: 0.5,
                                }}
                            >
                                {import.meta.env.DEV && !hasUpdate ? 'Dev Mode' : 'New Version'}
                            </Typography>
                        </Box>
                    )}
                </Box>
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
                        aria-label="About Tingly-Box"
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
                color="primary"
                aria-label="Open navigation menu"
                edge="start"
                onClick={handleDrawerToggle}
                sx={{
                    position: 'fixed',
                    top: { xs: 12, sm: 16 },
                    left: { xs: 12, sm: 16 },
                    zIndex: 1300,
                    display: { sm: 'none' },
                    backgroundColor: 'background.paper',
                    boxShadow: 3,
                    width: 44,
                    height: 44,
                    '&:hover': {
                        backgroundColor: 'action.hover',
                        transform: 'scale(1.05)',
                    },
                    '&:active': {
                        transform: 'scale(0.95)',
                    },
                    transition: 'all 150ms ease-in-out',
                }}
            >
                <MenuIcon />
            </IconButton>

            {/* Sidebar Drawer */}
            <Box component="nav" sx={{ width: { sm: drawerWidth }, flexShrink: { sm: 0 } }} aria-label="Main navigation">
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
                Hi, I'm Tingly-Box, Your Smart AI Proxy
            </Popover>
        </Box>
    );
};

export default Layout;
