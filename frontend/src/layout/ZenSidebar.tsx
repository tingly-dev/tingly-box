import { Box, Divider, List, ListItem, ListItemButton, ListItemIcon, ListItemText, Tooltip, Typography } from '@mui/material';
import React from 'react';
import {Trans, useTranslation} from 'react-i18next';
import { Link as RouterLink, useLocation } from 'react-router-dom';
import { useVersion } from '@/contexts/VersionContext';
import { sidebarWidth, headerHeight, footerHeight } from './constants';
import type { NavItem } from './types';

interface ZenSidebarProps {
    sidebarItems: NavItem[];
    activeActivityLabel: string;
}

/**
 * Zen Sidebar Component
 *
 * Simplified sidebar for zen mode showing:
 * - Agent page
 * - Profiles
 * - Add Profile button
 */
export const ZenSidebar: React.FC<ZenSidebarProps> = ({ sidebarItems, activeActivityLabel }) => {
    const { t } = useTranslation();
    const location = useLocation();
    const { currentVersion } = useVersion();
    const displayVersion = (currentVersion || 'Unknown').split('+')[0];

    const isActive = (path: string) => location.pathname === path;

    return (
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
            {/* Header */}
            <Box
                sx={{
                    height: headerHeight,
                    px: 2,
                    display: 'flex',
                    alignItems: 'center',
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                }}
            >
                <Typography variant="subtitle2" sx={{ color: 'text.primary', fontWeight: 600, fontSize: '0.875rem' }}>
                    {activeActivityLabel}
                </Typography>
            </Box>

            {/* Nav Items */}
            <List
                sx={{
                    flex: 1,
                    py: 1,
                    overflowY: 'auto',
                    '&::-webkit-scrollbar': { width: 6 },
                    '&::-webkit-scrollbar-track': { backgroundColor: 'transparent' },
                    '&::-webkit-scrollbar-thumb': {
                        backgroundColor: 'grey.300',
                        borderRadius: 1,
                        '&:hover': { backgroundColor: 'grey.400' },
                    },
                }}
            >
                {sidebarItems.map((item, index) => {
                    if (item.type === 'divider') {
                        return <Divider key={`divider-${index}`} sx={{ mx: 2, my: 1 }} />;
                    }

                    const isAddProfile = item.path === '#add-profile';
                    const active = !isAddProfile && isActive(item.path);

                    const button = (
                        <ListItem disablePadding>
                            <ListItemButton
                                {...(isAddProfile
                                    ? { onClick: () => {/* TODO: Handle add profile */ } }
                                    : { component: RouterLink, to: item.path }
                                )}
                                sx={{
                                    mx: 1.5,
                                    borderRadius: 1.25,
                                    py: 1.25,
                                    px: 2,
                                    color: 'text.secondary',
                                    position: 'relative',
                                    ...(active && {
                                        backgroundColor: 'primary.main',
                                        color: 'primary.contrastText',
                                        '& img': { filter: 'none !important' },
                                        '& .MuiListItemIcon-root > div': {
                                            bgcolor: 'white',
                                            borderRadius: 0.5,
                                            p: 0.25,
                                        },
                                        '&::before': {
                                            content: '""',
                                            position: 'absolute',
                                            left: 0,
                                            top: '50%',
                                            transform: 'translateY(-50%)',
                                            width: 3,
                                            height: 28,
                                            backgroundColor: 'primary.light',
                                            borderRadius: '0 2px 2px 0',
                                            boxShadow: '0 0 8px rgba(37, 99, 235, 0.5)',
                                        },
                                        '&:hover': { backgroundColor: 'primary.dark' },
                                        '& .MuiListItemIcon-root': { color: 'primary.contrastText' },
                                        '& .MuiListItemText-primary': { color: 'primary.contrastText', fontWeight: 600 },
                                    }),
                                    '&:hover': {
                                        backgroundColor: active ? 'primary.dark' : 'action.hover',
                                        color: active ? 'primary.contrastText' : 'text.primary',
                                    },
                                }}
                            >
                                {item.icon && (
                                    <ListItemIcon sx={{ minWidth: 32, color: 'inherit', '& svg': { fontSize: 20 } }}>
                                        {item.icon}
                                    </ListItemIcon>
                                )}
                                <ListItemText
                                    primary={item.label}
                                    secondary={item.subtitle}
                                    slotProps={{
                                        primary: { fontWeight: active ? 600 : 400, fontSize: '0.875rem', lineHeight: 1.3 },
                                        secondary: { fontSize: '0.6875rem', lineHeight: 1.2 },
                                    }}
                                    sx={{
                                        '& .MuiListItemText-secondary': {
                                            color: active ? 'rgba(255,255,255,0.7)' : 'text.secondary',
                                        },
                                    }}
                                />
                            </ListItemButton>
                        </ListItem>
                    );

                    return (
                        <React.Fragment key={item.path}>
                            {isAddProfile ? (
                                <Tooltip title="Create a new profile with custom settings" arrow placement="right">
                                    {button}
                                </Tooltip>
                            ) : button}
                        </React.Fragment>
                    );
                })}
            </List>

            {/* Footer top row: version */}
            <Box
                sx={{
                    py: 1.5, px: 2,
                    borderColor: 'divider',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    flexShrink: 0,
                    height: footerHeight,
                }}
            >
                <Typography
                    sx={{
                        color: 'text.secondary',
                        fontSize: '0.7rem',
                        textAlign: 'center',
                        display: 'block',
                        fontStyle: 'italic',
                        cursor: 'default',
                        maxWidth: '100%',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                    }}
                >
                    <Trans i18nKey="layout.version" values={{ version: displayVersion }} />
                </Typography>
            </Box>

            {/* Footer bottom row: slogan */}
            <Box
                sx={{
                   height: footerHeight, py: 1.5, px: 2, borderTop: '1px solid', borderColor: 'divider'
                }}
            >
                <Tooltip title="For all Solo Builders, Dev Teams and Agents." placement="top" arrow>
                    <Typography
                        variant="caption"
                        sx={{
                            color: 'text.secondary',
                            fontSize: '0.7rem',
                            textAlign: 'center',
                            display: 'block',
                            fontStyle: 'italic',
                            cursor: 'default',
                        }}
                    >
                        Zen Mode · Focus
                    </Typography>
                </Tooltip>
            </Box>
        </Box>
    );
};

export default ZenSidebar;
