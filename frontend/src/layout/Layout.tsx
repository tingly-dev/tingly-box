import {
    AccountCircle as AccountIcon,
    Dashboard as DashboardIcon,
    Key as KeyIcon,
    Menu as MenuIcon
} from '@mui/icons-material';
import {
    Box,
    Drawer,
    IconButton,
    List,
    ListItem,
    ListItemButton,
    ListItemIcon,
    ListItemText,
    Typography
} from '@mui/material';
import type {ReactNode} from 'react';
import {useEffect, useState} from 'react';
import {Link as RouterLink, useLocation, useNavigate} from 'react-router-dom';
import {useTranslation} from 'react-i18next';
import {useAuth} from '../contexts/AuthContext';
import api from '../services/api';
import VerifiedIcon from '@mui/icons-material/Verified';

interface LayoutProps {
    children: ReactNode;
}

const drawerWidth = 260;

const Layout = ({ children }: LayoutProps) => {
    const { t } = useTranslation();
    const location = useLocation();
    const navigate = useNavigate();
    const { logout } = useAuth();
    const [mobileOpen, setMobileOpen] = useState(false);
    const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
    const [version, setVersion] = useState<string>('Loading...');

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

    const menuItems = [
        { path: '/', label: t('layout.nav.home'), icon: <DashboardIcon /> },
        { path: '/api-keys', label: t('layout.nav.apiKeys', { defaultValue: 'API Keys' }), icon: <KeyIcon /> },
        { path: '/oauth', label: t('layout.nav.oauth', { defaultValue: 'OAuth' }), icon: <VerifiedIcon /> },
        // { path: '/routing', label: 'Advance', icon: <ForkRight sx={{ transform: 'rotate(45deg)' }} /> },
        // { path: '/system', label: 'System', icon: <SettingsIcon /> },
        // { path: '/history', label: 'History', icon: <HistoryIcon /> },
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
                {menuItems.map((item) => (
                    <ListItem key={item.path} disablePadding sx={{ mb: 1 }}>
                        <ListItemButton
                            component={RouterLink}
                            to={item.path}
                            onClick={handleDrawerToggle}
                            className={isActive(item.path) ? 'nav-item-active' : ''}
                            sx={{
                                mx: 2,
                                borderRadius: 2,
                                color: isActive(item.path) ? 'inherit' : 'text.primary',
                                '&:hover': {
                                    backgroundColor: isActive(item.path) ? 'inherit' : 'action.hover',
                                },
                                '& .MuiListItemIcon-root': {
                                    color: isActive(item.path) ? 'inherit' : 'text.secondary',
                                },
                            }}
                        >
                            <ListItemIcon>{item.icon}</ListItemIcon>
                            <ListItemText
                                primary={item.label}
                                primaryTypographyProps={{
                                    fontWeight: isActive(item.path) ? 'inherit' : 400,
                                }}
                            />
                        </ListItemButton>
                    </ListItem>
                ))}
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
                    fontStyle: 'italic'
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
                    {/*<Menu*/}
                    {/*    anchorEl={anchorEl}*/}
                    {/*    open={Boolean(anchorEl)}*/}
                    {/*    onClose={handleMenuClose}*/}
                    {/*    anchorOrigin={{*/}
                    {/*        vertical: 'top',*/}
                    {/*        horizontal: 'right',*/}
                    {/*    }}*/}
                    {/*    transformOrigin={{*/}
                    {/*        vertical: 'bottom',*/}
                    {/*        horizontal: 'right',*/}
                    {/*    }}*/}
                    {/*>*/}
                    {/*    <MenuItem onClick={handleLogout}>*/}
                    {/*        <ListItemIcon>*/}
                    {/*            <LogoutIcon fontSize="small" />*/}
                    {/*        </ListItemIcon>*/}
                    {/*        <ListItemText>Logout</ListItemText>*/}
                    {/*    </MenuItem>*/}
                    {/*</Menu>*/}
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
            <Box
                component="nav"
                sx={{ width: { sm: drawerWidth }, flexShrink: { sm: 0 } }}
            >
                <Drawer
                    variant="temporary"
                    open={mobileOpen}
                    onClose={handleDrawerToggle}
                    ModalProps={{
                        keepMounted: true, // Better open performance on mobile.
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
                {/* Page Content */}
                <Box
                    sx={{
                        flex: 1,
                        p: 3,
                        overflowY: 'auto',
                        height: '100%',
                        // Ensure proper scrolling
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
