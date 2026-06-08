import { Info as IconInfoCircle } from '@/components/icons';
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
    Stack,
    Switch,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import React, { useCallback, useRef, useState } from 'react';
import {Trans, useTranslation} from 'react-i18next';
import { Link as RouterLink, useLocation } from 'react-router-dom';
import { api } from '@/services/api';
import { useProfileContext } from '@/contexts/ProfileContext';
import { useVersion } from '@/contexts/VersionContext';
import { footerHeight, headerHeight, sidebarWidth } from './constants';
import type { NavItem } from './types';
import { VersionDisplay } from '@/components/VersionDisplay';
import { UpdatePanelDialog } from '@/components/UpdatePanelDialog';

interface SidebarProps {
    sidebarItems: NavItem[];
    activeActivityLabel: string;
    onClose: () => void;
    headerAction?: React.ReactNode;
}

export const Sidebar: React.FC<SidebarProps> = ({ sidebarItems, activeActivityLabel, onClose, headerAction }) => {
    const { t } = useTranslation();
    const location = useLocation();
    const { refresh } = useProfileContext();
    const { currentVersion } = useVersion();

    const [addProfileAnchorEl, setAddProfileAnchorEl] = useState<HTMLElement | null>(null);
    const [newProfileName, setNewProfileName] = useState('');
    const [newProfileUnified, setNewProfileUnified] = useState(true);  // Default to unified
    const [isCreating, setIsCreating] = useState(false);
    const [updatePanelOpen, setUpdatePanelOpen] = useState(false);
    const addProfileInputRef = useRef<HTMLInputElement>(null);

    const isActive = (path: string) => location.pathname === path;

    const handleAddProfileClick = useCallback((e: React.MouseEvent<HTMLElement>) => {
        setAddProfileAnchorEl(e.currentTarget);
        setNewProfileName('');
        setNewProfileUnified(true);  // Reset to unified when opening
        setTimeout(() => addProfileInputRef.current?.focus(), 100);
    }, []);

    const handleAddProfileClose = useCallback(() => {
        setAddProfileAnchorEl(null);
        setNewProfileName('');
        setNewProfileUnified(true);  // Reset to unified when closing
    }, []);

    const handleCreateProfile = useCallback(async () => {
        if (!newProfileName.trim()) return;
        try {
            setIsCreating(true);
            const result = await api.createProfile('claude_code', newProfileName.trim(), newProfileUnified);
            if (result.success) {
                handleAddProfileClose();
                refresh();
            }
        } catch {
            // silent fail
        } finally {
            setIsCreating(false);
        }
    }, [newProfileName, newProfileUnified, refresh, handleAddProfileClose]);

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
                    justifyContent: 'space-between',
                    gap: 1,
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                }}
            >
                <Typography variant="body2" sx={{ color: 'text.primary', fontWeight: 600 }}>
                    {activeActivityLabel}
                </Typography>
                {headerAction}
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
                                        '&:hover': { backgroundColor: 'primary.main' },
                                        '& .MuiListItemIcon-root': { color: 'primary.contrastText' },
                                        '& .MuiListItemText-primary': { color: 'primary.contrastText', fontWeight: 600 },
                                    }),
                                    '&:hover': {
                                        backgroundColor: active ? 'primary.main' : 'action.hover',
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
                                        primary: { fontWeight: active ? 600 : 400, variant: 'body2' as const, sx: { lineHeight: 1.3 } },
                                        secondary: { variant: 'caption' as const, sx: { fontSize: '0.6875rem', lineHeight: 1.2 } },
                                    }}
                                    sx={{
                                        '& .MuiListItemText-primary': {
                                            fontSize: '0.875rem',
                                        },
                                        '& .MuiListItemText-secondary': {
                                            color: active ? 'rgba(255,255,255,0.7)' : 'text.secondary',
                                        },
                                    }}
                                />
                                {item.tooltip && (
                                    <IconInfoCircle
                                        sx={{
                                            fontSize: 13,
                                            flexShrink: 0,
                                            ml: '4px',
                                            opacity: active ? 0.6 : 0.35,
                                        }}
                                    />
                                )}
                            </ListItemButton>
                        </ListItem>
                    );

                    return (
                        <React.Fragment key={item.path}>
                            {isAddProfile ? (
                                <Tooltip title={t('layout.sidebar.createProfileTooltip')} arrow placement="right">
                                    {button}
                                </Tooltip>
                            ) : item.tooltip ? (
                                <Tooltip
                                    title={item.tooltip}
                                    arrow
                                    placement="right"
                                    enterDelay={600}
                                    enterNextDelay={600}
                                    slotProps={{ tooltip: { sx: { maxWidth: 320 } } }}
                                >
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
                <VersionDisplay onClick={() => setUpdatePanelOpen(true)} />
            </Box>

            {/* Add Profile Popover */}
            <Popover
                open={Boolean(addProfileAnchorEl)}
                anchorEl={addProfileAnchorEl}
                onClose={handleAddProfileClose}
                anchorOrigin={{ vertical: 'top', horizontal: 'right' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                slotProps={{ paper: { sx: { p: 2, width: 280, mt: -0.5 } } }}
            >
                <Typography variant="subtitle2" sx={{ mb: 1.5, fontWeight: 600 }}>{t('layout.sidebar.newProfile')}</Typography>
                <TextField
                    inputRef={addProfileInputRef}
                    fullWidth
                    size="small"
                    placeholder={t('layout.sidebar.profileName')}
                    value={newProfileName}
                    onChange={(e) => setNewProfileName(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleCreateProfile()}
                    disabled={isCreating}
                />
                <Box sx={{ mt: 2, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <Box>
                        <Typography variant="body2" sx={{ fontWeight: 500 }}>{t('layout.sidebar.mode')}</Typography>
                        <Typography variant="caption" color="text.secondary">
                            {newProfileUnified ? t('layout.sidebar.modeUnified') : t('layout.sidebar.modeSeparate')}
                        </Typography>
                    </Box>
                    <Stack direction="row" spacing={1} alignItems="center">
                        <Typography variant="body2" color="text.secondary">{t('layout.sidebar.separate')}</Typography>
                        <Switch
                            size="small"
                            checked={newProfileUnified}
                            onChange={(e) => setNewProfileUnified(e.target.checked)}
                            disabled={isCreating}
                        />
                        <Typography variant="body2" color="text.secondary">{t('layout.sidebar.unified')}</Typography>
                    </Stack>
                </Box>
                <Box sx={{ mt: 1.5, display: 'flex', justifyContent: 'flex-end', gap: 1 }}>
                    <Button size="small" onClick={handleAddProfileClose} disabled={isCreating}>{t('common.cancel')}</Button>
                    <Button size="small" variant="contained" onClick={handleCreateProfile} disabled={!newProfileName.trim() || isCreating}>
                        {t('common.add')}
                    </Button>
                </Box>
            </Popover>

            {/* Footer bottom row: slogan */}
            <Box
                sx={{
                   height: footerHeight, py: 1.5, px: 2, borderTop: '1px solid', borderColor: 'divider'
                }}
            >
                <Tooltip title={t('layout.sidebar.sloganTooltip')} placement="top" arrow>
                    <Typography
                        variant="caption"
                        sx={{
                            color: 'text.secondary',
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

            {/* Update Panel Dialog */}
            <UpdatePanelDialog
                open={updatePanelOpen}
                onClose={() => setUpdatePanelOpen(false)}
            />
        </Box>
    );
};
