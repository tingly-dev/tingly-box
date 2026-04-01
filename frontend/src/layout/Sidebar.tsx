import {
    Add as AddIcon,
} from '@mui/icons-material';
import {
    Box,
    Button,
    Divider,
    List,
    ListItem,
    ListItemButton,
    ListItemIcon,
    ListItemText,
    Popover,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import React, { useCallback, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink, useLocation } from 'react-router-dom';
import { api } from '@/services/api';
import { useProfileContext } from '@/contexts/ProfileContext';
import { footerHeight, headerHeight, sidebarWidth } from './constants';
import type { NavItem } from './types';

interface SidebarProps {
    sidebarItems: NavItem[];
    activeActivityLabel: string;
    onClose: () => void;
}

export const Sidebar: React.FC<SidebarProps> = ({ sidebarItems, activeActivityLabel, onClose }) => {
    const { t } = useTranslation();
    const location = useLocation();
    const { refresh } = useProfileContext();

    const [addProfileAnchorEl, setAddProfileAnchorEl] = useState<HTMLElement | null>(null);
    const [newProfileName, setNewProfileName] = useState('');
    const [isCreating, setIsCreating] = useState(false);
    const addProfileInputRef = useRef<HTMLInputElement>(null);

    const isActive = (path: string) => location.pathname === path;

    const handleAddProfileClick = useCallback((e: React.MouseEvent<HTMLElement>) => {
        setAddProfileAnchorEl(e.currentTarget);
        setNewProfileName('');
        setTimeout(() => addProfileInputRef.current?.focus(), 100);
    }, []);

    const handleAddProfileClose = useCallback(() => {
        setAddProfileAnchorEl(null);
        setNewProfileName('');
    }, []);

    const handleCreateProfile = useCallback(async () => {
        if (!newProfileName.trim()) return;
        try {
            setIsCreating(true);
            const result = await api.createProfile('claude_code', newProfileName.trim());
            if (result.success) {
                handleAddProfileClose();
                refresh();
            }
        } catch {
            // silent fail
        } finally {
            setIsCreating(false);
        }
    }, [newProfileName, refresh, handleAddProfileClose]);

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
                                    ? { onClick: handleAddProfileClick }
                                    : { component: RouterLink, to: item.path, onClick: onClose }
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
                                <Tooltip title="Create a new Claude Code profile with custom settings" arrow placement="right">
                                    {button}
                                </Tooltip>
                            ) : button}
                        </React.Fragment>
                    );
                })}
            </List>

            {/* Add Profile Popover */}
            <Popover
                open={Boolean(addProfileAnchorEl)}
                anchorEl={addProfileAnchorEl}
                onClose={handleAddProfileClose}
                anchorOrigin={{ vertical: 'top', horizontal: 'right' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                slotProps={{ paper: { sx: { p: 2, width: 220, mt: -0.5 } } }}
            >
                <Typography variant="subtitle2" sx={{ mb: 1.5, fontWeight: 600 }}>New Profile</Typography>
                <TextField
                    inputRef={addProfileInputRef}
                    fullWidth
                    size="small"
                    placeholder="Profile name"
                    value={newProfileName}
                    onChange={(e) => setNewProfileName(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleCreateProfile()}
                    disabled={isCreating}
                />
                <Box sx={{ mt: 1.5, display: 'flex', justifyContent: 'flex-end', gap: 1 }}>
                    <Button size="small" onClick={handleAddProfileClose} disabled={isCreating}>Cancel</Button>
                    <Button size="small" variant="contained" onClick={handleCreateProfile} disabled={!newProfileName.trim() || isCreating}>
                        Create
                    </Button>
                </Box>
            </Popover>

            {/* Footer Slogan */}
            <Box sx={{ height: footerHeight, py: 1.5, px: 2, borderTop: '1px solid', borderColor: 'divider' }}>
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
                        {t('layout.slogan')}
                    </Typography>
                </Tooltip>
            </Box>
        </Box>
    );
};
