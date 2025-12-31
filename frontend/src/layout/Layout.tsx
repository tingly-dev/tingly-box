import {
    AccountCircle as AccountIcon,
    Dashboard as DashboardIcon,
    Key as KeyIcon,
    Menu as MenuIcon,
    Settings as SystemIcon,
    ExpandLess,
    ExpandMore,
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
} from '@mui/material';
import type { ReactNode } from 'react';
import React from 'react';
import { useEffect, useState } from 'react';
import { Link as RouterLink, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useAuth } from '../contexts/AuthContext';
import api from '../services/api';
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
    const navigate = useNavigate();
    const { logout } = useAuth();
    const [mobileOpen, setMobileOpen] = useState(false);
    const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
    const [version, setVersion] = useState<string>('Loading...');
    const [homeMenuOpen, setHomeMenuOpen] = useState(true);
    const [credentialMenuOpen, setCredentialMenuOpen] = useState(true);

    useEffect(() => {
        const fetchVersion = async () => {
            const v = await api.getVersion();
            setVersion(v);
        };
        fetchVersion();
    }, []);

    const handleDrawerToggle = () => {
        setMobileOpen(!mobileOpen);
    };

    const handleMenuOpen = (event: React.MouseEvent<HTMLElement>) => {
        setAnchorEl(event.currentTarget);
    };

    const handleMenuClose = () => {
        setAnchorEl(null);
    };

    const handleLogout = () => {
        logout();
        navigate('/login');
        handleMenuClose();
    };

    const isActive = (path: string) => {
        return location.pathname === path;
    };

    const isGroupActive = (items: MenuItem[]) => {
        return items.some(item => isActive(item.path));
    };

    const menuGroups: MenuGroup[] = [
        {
            key: 'home',
            label: t('layout.nav.home'),
            icon: <DashboardIcon sx={{ fontSize: 20 }} />,
            items: [
                {
                    path: '/use-openai',
                    label: t('layout.nav.useOpenAI', { defaultValue: 'OpenAI' }),
                    icon: <OpenAI size={18} />,
                },
                {
                    path: '/use-anthropic',
                    label: t('layout.nav.useAnthropic', { defaultValue: 'Anthropic' }),
                    icon: <Anthropic size={18} />,
                },
                {
                    path: '/use-claude-code',
                    label: t('layout.nav.useClaudeCode', { defaultValue: 'Claude Code' }),
                    icon: <Claude size={18} />,
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
                    icon: <KeyIcon sx={{ fontSize: 18 }} />,
                },
                {
                    path: '/oauth',
                    label: t('layout.nav.oauth', { defaultValue: 'OAuth' }),
                    icon: <VerifiedIcon sx={{ fontSize: 18 }} />,
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
                    icon: <SystemIcon sx={{ fontSize: 18 }} />,
                },
            ],
        },
    ];

    const drawerContent = (
        <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
            {/* Logo Section */}
            <Box
                sx={{
                    p: 3,
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                    display: 'flex',
                    alignItems: 'center',
                    gap: 2,
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
                            <ListItem disablePadding sx={{ mb: isStandalone ? 0 : 1 }}>
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

            {/* Version Info */}
            <Typography
                variant="body2"
                sx={{
                    color: 'text.secondary',
                    fontSize: '0.75rem',
                    textAlign: 'center',
                    py: 1,
                    borderTop: '1px solid',
                    borderColor: 'divider',
                    mt: 1,
                    fontStyle: 'italic',
                }}
            >
                {t('layout.version', { version })}
            </Typography>

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
                        onClick={handleMenuOpen}
                        sx={{ color: 'text.secondary' }}
                    >
                        <AccountIcon />
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
                    minHeight: '100vh',
                    display: 'flex',
                    flexDirection: 'column',
                }}
            >
                <Box
                    sx={{
                        flex: 1,
                        p: 3,
                        overflowY: 'auto',
                        height: '100%',
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
        </Box>
    );
};

export default Layout;
