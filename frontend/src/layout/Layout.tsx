import {
    AccountCircle as AccountIcon,
    Dashboard as DashboardIcon,
    Settings as SystemIcon,
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
    Menu as MenuIcon,
} from '@mui/icons-material';
import LockIcon from '@mui/icons-material/Lock';
import {
    Box,
    Drawer,
    IconButton,
    List,
    ListItem,
    ListItemButton,
    ListItemIcon,
    ListItemText,
    Typography,
    Popover,
    Tooltip,
    Divider,
} from '@mui/material';
import type { ReactNode } from 'react';
import React, { useState, useMemo } from 'react';
import { Link as RouterLink, useLocation, useNavigate } from 'react-router-dom';
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

const activityBarWidth = 77;
const sidebarWidth = 200;

interface NavItem {
    path: string;
    label: string;
    icon?: ReactNode;
    divider?: boolean;
}

interface ActivityItem {
    key: string;
    icon: ReactNode;
    label: string;
    path?: string; // Direct navigation if no children
    children?: NavItem[];
}

const Layout = ({ children }: LayoutProps) => {
    const { t } = useTranslation();
    const location = useLocation();
    const navigate = useNavigate();
    const { hasUpdate, currentVersion, showUpdateDialog } = useAppVersion();
    const { isHealthy, showDisconnectDialog } = useHealth();
    const { skillUser, skillIde, enableRemoteCoder } = useFeatureFlags();
    const [mobileOpen, setMobileOpen] = useState(false);
    const [easterEggAnchorEl, setEasterEggAnchorEl] = useState<HTMLElement | null>(null);

    const handleEasterEgg = (event: React.MouseEvent<HTMLElement>) => {
        setEasterEggAnchorEl(event.currentTarget);
    };

    const handleEasterEggClose = () => {
        setEasterEggAnchorEl(null);
    };

    const isActive = (path: string) => {
        return location.pathname === path;
    };

    const isChildActive = (children?: NavItem[]) => {
        return children?.some(item => isActive(item.path)) ?? false;
    };

    // Build prompt menu items based on feature flags
    const promptMenuItems = useMemo(() => {
        const items: NavItem[] = [];
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
        return items;
    }, [skillUser, skillIde]);

    // Activity bar items
    const activityItems: ActivityItem[] = useMemo(() => {
        const items: ActivityItem[] = [
            {
                key: 'dashboard',
                icon: <BarChartIcon sx={{ fontSize: 22 }} />,
                label: 'Dashboard',
                path: '/dashboard',
            },
            {
                key: 'scenario',
                icon: <CodeIcon sx={{ fontSize: 22 }} />,
                label: t('layout.nav.home'),
                children: [
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
            ...(promptMenuItems.length > 0 ? [{
                key: 'prompt' as const,
                icon: <PromptIcon sx={{ fontSize: 22 }} />,
                label: 'Prompt',
                children: promptMenuItems,
            }] : []),
            ...(enableRemoteCoder ? [{
                key: 'remote-control' as const,
                icon: <RemoteIcon sx={{ fontSize: 22 }} />,
                label: 'Remote',
                children: [
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
                key: 'credential',
                icon: <LockIcon sx={{ fontSize: 22 }} />,
                label: t('layout.nav.credential', { defaultValue: 'Credentials' }),
                path: '/credentials',
            },
            {
                key: 'system',
                icon: <SystemIcon sx={{ fontSize: 22 }} />,
                label: 'System',
                path: '/system',
            },
        ];
        return items;
    }, [t, promptMenuItems, enableRemoteCoder]);

    // Find current active activity
    const activeActivity = useMemo(() => {
        for (const item of activityItems) {
            if (item.path && isActive(item.path)) return item.key;
            if (item.children && isChildActive(item.children)) return item.key;
        }
        return 'dashboard';
    }, [activityItems, location.pathname]);

    // Get sidebar items for active activity
    const sidebarItems = useMemo(() => {
        const activity = activityItems.find(item => item.key === activeActivity);
        return activity?.children || [];
    }, [activityItems, activeActivity]);

    // Get current activity label
    const activeActivityLabel = useMemo(() => {
        const activity = activityItems.find(item => item.key === activeActivity);
        return activity?.label || '';
    }, [activityItems, activeActivity]);

    // Activity bar content (first column - icon only)
    const activityBarContent = (
        <Box
            sx={{
                width: activityBarWidth,
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
                bgcolor: 'background.paper',
                borderRight: '1px solid',
                borderColor: 'divider',
            }}
        >
            {/* Logo */}
            <Box
                sx={{
                    height: 56,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                }}
            >
                <Tooltip title={`Tingly-Box v${currentVersion}`} placement="right" arrow>
                    <Box
                        component="a"
                        href="https://github.com/tingly-dev/tingly-box"
                        target="_blank"
                        rel="noopener noreferrer"
                        sx={{
                            width: 36,
                            height: 36,
                            borderRadius: 2,
                            background: 'linear-gradient(135deg, #1a1a1a 0%, #2d2d2d 100%)',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            color: 'white',
                            fontWeight: 'bold',
                            fontSize: '1.1rem',
                            textDecoration: 'none',
                            cursor: 'pointer',
                            transition: 'transform 0.2s',
                            '&:hover': {
                                transform: 'scale(1.08)',
                            },
                        }}
                    >
                        T
                    </Box>
                </Tooltip>
            </Box>

            {/* Activity Icons */}
            <Box sx={{ flex: 1, py: 0.5, overflowY: 'auto' }}>
                {activityItems.map((item) => {
                    const isActiveItem = activeActivity === item.key;

                    // Handle click: if has children, navigate to first child
                    const handleClick = () => {
                        setMobileOpen(false);
                        if (item.children && item.children.length > 0) {
                            navigate(item.children[0].path);
                        }
                    };

                    // Short label for display (max 8 chars)
                    const shortLabel = item.label.length > 12 ? item.label.slice(0, 7) + '…' : item.label;

                    return (
                        <ListItemButton
                            key={item.key}
                            component={item.path ? RouterLink : 'div'}
                            to={item.path}
                            onClick={item.children ? handleClick : () => setMobileOpen(false)}
                            sx={{
                                minHeight: 56,
                                px: 1,
                                py: 1,
                                flexDirection: 'column',
                                alignItems: 'center',
                                justifyContent: 'center',
                                gap: 0.25,
                                position: 'relative',
                                color: isActiveItem ? 'primary.main' : 'text.secondary',
                                transition: 'all 0.15s ease-in-out',
                                borderRadius: 0,
                                cursor: 'pointer',
                                '&:hover': {
                                    bgcolor: 'action.hover',
                                    color: 'primary.main',
                                },
                                ...(isActiveItem && {
                                    bgcolor: 'action.selected',
                                    '&::before': {
                                        content: '""',
                                        position: 'absolute',
                                        left: 0,
                                        top: '50%',
                                        transform: 'translateY(-50%)',
                                        width: 3,
                                        height: 36,
                                        bgcolor: 'primary.main',
                                        borderRadius: '0 2px 2px 0',
                                    },
                                }),
                            }}
                        >
                            <ListItemIcon
                                sx={{
                                    minWidth: 0,
                                    color: 'inherit',
                                    justifyContent: 'center',
                                }}
                            >
                                {item.icon}
                            </ListItemIcon>
                            <Typography
                                variant="caption"
                                sx={{
                                    fontSize: '0.65rem',
                                    fontWeight: isActiveItem ? 600 : 400,
                                    color: 'inherit',
                                    textAlign: 'center',
                                    lineHeight: 1.2,
                                    maxWidth: '100%',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    whiteSpace: 'nowrap',
                                }}
                            >
                                {shortLabel}
                            </Typography>
                        </ListItemButton>
                    );
                })}
            </Box>

            {/* Bottom Section: Status + User */}
            <Box sx={{ py: 1, borderTop: '1px solid', borderColor: 'divider' }}>
                {/* Status Indicators */}
                {!isHealthy && (
                    <Tooltip title="Disconnected" placement="right" arrow>
                        <ListItemButton
                            onClick={showDisconnectDialog}
                            sx={{
                                minHeight: 44,
                                px: 1.5,
                                justifyContent: 'center',
                                color: 'error.main',
                                borderRadius: 0,
                                '&:hover': {
                                    bgcolor: 'action.hover',
                                },
                            }}
                        >
                            <ListItemIcon sx={{ minWidth: 0, color: 'inherit', justifyContent: 'center' }}>
                                <ErrorOutline sx={{ fontSize: 22 }} />
                            </ListItemIcon>
                        </ListItemButton>
                    </Tooltip>
                )}

                {(hasUpdate || import.meta.env.DEV) && (
                    <Tooltip
                        title={import.meta.env.DEV && !hasUpdate ? 'Dev Mode' : 'New Version Available'}
                        placement="right"
                        arrow
                    >
                        <ListItemButton
                            onClick={showUpdateDialog}
                            sx={{
                                minHeight: 44,
                                px: 1.5,
                                justifyContent: 'center',
                                color: import.meta.env.DEV && !hasUpdate ? 'success.main' : 'info.main',
                                borderRadius: 0,
                                '&:hover': {
                                    bgcolor: 'action.hover',
                                },
                            }}
                        >
                            <ListItemIcon sx={{ minWidth: 0, color: 'inherit', justifyContent: 'center' }}>
                                <NewReleases sx={{ fontSize: 22 }} />
                            </ListItemIcon>
                        </ListItemButton>
                    </Tooltip>
                )}

                {/* User */}
                <Tooltip title="About" placement="right" arrow>
                    <ListItemButton
                        onClick={handleEasterEgg}
                        sx={{
                            minHeight: 44,
                            px: 1.5,
                            justifyContent: 'center',
                            color: 'text.secondary',
                            borderRadius: 0,
                            '&:hover': {
                                bgcolor: 'action.hover',
                                color: 'text.primary',
                            },
                        }}
                    >
                        <ListItemIcon sx={{ minWidth: 0, color: 'inherit', justifyContent: 'center' }}>
                            <AccountIcon sx={{ fontSize: 22 }} />
                        </ListItemIcon>
                    </ListItemButton>
                </Tooltip>
            </Box>
        </Box>
    );

    // Sidebar panel content (second column - shows sub-items)
    const sidebarContent = (
        <Box
            sx={{
                width: sidebarWidth,
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
                bgcolor: 'background.paper',
                borderRight: '1px solid',
                borderColor: 'divider',
                overflow: 'hidden',
            }}
        >
            {/* Sidebar Header */}
            <Box
                sx={{
                    height: 56,
                    px: 2,
                    display: 'flex',
                    alignItems: 'center',
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                }}
            >
                <Typography
                    variant="subtitle2"
                    sx={{
                        color: 'text.primary',
                        fontWeight: 600,
                        fontSize: '0.875rem',
                    }}
                >
                    {activeActivityLabel}
                </Typography>
            </Box>

            {/* Sidebar Items */}
            <List sx={{ flex: 1, py: 1.5, overflowY: 'auto', '&::-webkit-scrollbar': { width: 6 }, '&::-webkit-scrollbar-track': { backgroundColor: 'transparent' }, '&::-webkit-scrollbar-thumb': { backgroundColor: 'grey.300', borderRadius: 1, '&:hover': { backgroundColor: 'grey.400' } } }}>
                {sidebarItems.map((item) => (
                    <React.Fragment key={item.path}>
                        {item.divider && <Divider sx={{ mx: 2, my: 1 }} />}
                        <ListItem disablePadding>
                            <ListItemButton
                                component={RouterLink}
                                to={item.path}
                                onClick={() => setMobileOpen(false)}
                                sx={{
                                    mx: 1.5,
                                    borderRadius: 1.5,
                                    py: 1.25,
                                    px: 2,
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
                                        color: isActive(item.path) ? 'primary.contrastText' : 'text.primary',
                                        transform: 'translateX(2px)',
                                    },
                                }}
                            >
                                {item.icon && (
                                    <ListItemIcon
                                        sx={{
                                            minWidth: 32,
                                            color: isActive(item.path) ? 'primary.contrastText' : 'text.secondary',
                                            '& svg': { fontSize: 20 },
                                        }}
                                    >
                                        {item.icon}
                                    </ListItemIcon>
                                )}
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
                    </React.Fragment>
                ))}
            </List>

            {/* Bottom Slogan */}
            <Box
                sx={{
                    p: 2,
                    borderTop: '1px solid',
                    borderColor: 'divider',
                }}
            >
                <Typography
                    variant="caption"
                    sx={{
                        color: 'text.secondary',
                        fontSize: '0.7rem',
                        textAlign: 'center',
                        display: 'block',
                        fontStyle: 'italic',
                    }}
                >
                    {t('layout.slogan')}
                </Typography>
            </Box>
        </Box>
    );

    // Combined navigation for desktop (activity bar + conditional sidebar)
    const desktopNavigation = (
        <Box sx={{ display: 'flex', height: '100vh' }}>
            {activityBarContent}
            {sidebarItems.length > 0 && sidebarContent}
        </Box>
    );

    // Mobile drawer content
    const mobileDrawerContent = (
        <Box sx={{ display: 'flex', height: '100%' }}>
            {activityBarContent}
            {sidebarItems.length > 0 && sidebarContent}
        </Box>
    );

    return (
        <Box sx={{ display: 'flex', minHeight: '100vh', backgroundColor: '#f8f9fa' }}>
            {/* Desktop Layout */}
            <Box component="nav" sx={{ display: { xs: 'none', md: 'block' } }}>
                {desktopNavigation}
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
                        bgcolor: 'background.paper',
                    },
                }}
            >
                {mobileDrawerContent}
            </Drawer>

            {/* Mobile Toggle Button */}
            <IconButton
                color="primary"
                aria-label="Open navigation menu"
                onClick={() => setMobileOpen(!mobileOpen)}
                sx={{
                    display: { xs: 'flex', md: 'none' },
                    position: 'fixed',
                    top: 12,
                    left: 12,
                    zIndex: 1300,
                    bgcolor: 'background.paper',
                    boxShadow: 3,
                    width: 44,
                    height: 44,
                    '&:hover': {
                        bgcolor: 'action.hover',
                        transform: 'scale(1.05)',
                    },
                    transition: 'all 0.15s',
                }}
            >
                <MenuIcon />
            </IconButton>

            {/* Main Content */}
            <Box
                component="main"
                sx={{
                    flexGrow: 1,
                    height: '100vh',
                    display: 'flex',
                    flexDirection: 'column',
                    overflow: 'hidden',
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
