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
import { Link as RouterLink, useLocation, useNavigate } from 'react-router-dom';
import { api } from '@/services/api';
import { useProfileContext } from '@/contexts/ProfileContext';
import { useTeamContext } from '@/contexts/TeamContext';
import { useNotify } from '@/hooks/useNotify';
import { useVersion } from '@/contexts/VersionContext';
import { footerHeight, sidebarWidth } from './constants';
import {
    NAV_ROW_SX,
    navRowTextSlotProps,
    sidebarContainerSx,
    sidebarHeaderSx,
    sidebarListScrollSx,
} from './styles';
import type { NavItem } from './types';
import { VersionDisplay } from '@/components/VersionDisplay';
import { UpdatePanelDialog } from '@/components/UpdatePanelDialog';

// Shared sizing for the sidebar's nav-style rows now lives in ./styles
// (NAV_ROW_SX / navRowTextSlotProps), so every row is the same height whether
// or not it carries a subtitle.

interface SidebarProps {
    sidebarItems: NavItem[];
    activeActivityLabel: string;
    onClose: () => void;
    headerAction?: React.ReactNode;
}

export const Sidebar: React.FC<SidebarProps> = ({ sidebarItems, activeActivityLabel, onClose, headerAction }) => {
    const { t } = useTranslation();
    const location = useLocation();
    const navigate = useNavigate();
    const notify = useNotify();
    const { refresh } = useProfileContext();
    const { refresh: refreshTeams } = useTeamContext();
    const { currentVersion } = useVersion();

    const [addProfileAnchorEl, setAddProfileAnchorEl] = useState<HTMLElement | null>(null);
    const [newProfileName, setNewProfileName] = useState('');
    const [newProfileUnified, setNewProfileUnified] = useState(true);  // Default to unified
    const [isCreating, setIsCreating] = useState(false);
    const [updatePanelOpen, setUpdatePanelOpen] = useState(false);
    const addProfileInputRef = useRef<HTMLInputElement>(null);
    const [addTeamAnchorEl, setAddTeamAnchorEl] = useState<HTMLElement | null>(null);
    const [newTeamName, setNewTeamName] = useState('');
    const addTeamInputRef = useRef<HTMLInputElement>(null);

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

    const handleAddTeamClick = useCallback((e: React.MouseEvent<HTMLElement>) => {
        setAddTeamAnchorEl(e.currentTarget);
        setNewTeamName('');
        setTimeout(() => addTeamInputRef.current?.focus(), 100);
    }, []);

    const handleAddTeamClose = useCallback(() => {
        setAddTeamAnchorEl(null);
        setNewTeamName('');
    }, []);

    const handleCreateTeam = useCallback(async () => {
        if (!newTeamName.trim()) return;
        setIsCreating(true);
        const result = await api.createTeam({name: newTeamName.trim()});
        setIsCreating(false);
        if (!result.success) {
            notify.error(result.error?.message || t('teams.saveFailed'));
            return;
        }
        handleAddTeamClose();
        await refreshTeams();
        navigate(`/agent/team/${result.data.slug}`);
        onClose();
    }, [newTeamName, notify, t, handleAddTeamClose, refreshTeams, navigate, onClose]);

    return (
        <Box
            sx={{ width: sidebarWidth, ...sidebarContainerSx }}
        >
            {/* Header */}
            <Box sx={sidebarHeaderSx}>
                <Typography variant="body2" sx={{ color: 'text.primary', fontWeight: 600 }}>
                    {activeActivityLabel}
                </Typography>
                {headerAction}
            </Box>
            {/* Nav Items */}
            <List sx={sidebarListScrollSx}>
                {sidebarItems.map((item, index) => {
                    if (item.type === 'divider') {
                        return <Divider key={`divider-${index}`} sx={{ mx: 2, my: 1 }} />;
                    }

                    const isAddProfile = item.path === '#add-profile';
                    const isAddTeam = item.path === '#add-team';
                    const isAddAction = isAddProfile || isAddTeam;
                    const active = !isAddAction && (item.match ? item.match(location.pathname) : isActive(item.path));

                    const button = (
                        <ListItem disablePadding>
                            <ListItemButton
                                {...(isAddAction
                                    ? { onClick: isAddProfile ? handleAddProfileClick : handleAddTeamClick }
                                    : { component: RouterLink, to: item.path, onClick: onClose }
                                )}
                                sx={{
                                    ...NAV_ROW_SX,
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
                                        '& .MuiListItemText-primary': { color: 'primary.contrastText' },
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
                                    slotProps={navRowTextSlotProps(active)}
                                    sx={{ minWidth: 0 }}
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
                            ) : isAddTeam ? (
                                <Tooltip title={t('layout.sidebar.createTeamTooltip')} arrow placement="right">
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
                        <Typography variant="caption" sx={{
                            color: "text.secondary"
                        }}>
                            {newProfileUnified ? t('layout.sidebar.modeUnified') : t('layout.sidebar.modeSeparate')}
                        </Typography>
                    </Box>
                    <Stack direction="row" spacing={1} sx={{
                        alignItems: "center"
                    }}>
                        <Typography variant="body2" sx={{
                            color: "text.secondary"
                        }}>{t('layout.sidebar.separate')}</Typography>
                        <Switch
                            size="small"
                            checked={newProfileUnified}
                            onChange={(e) => setNewProfileUnified(e.target.checked)}
                            disabled={isCreating}
                        />
                        <Typography variant="body2" sx={{
                            color: "text.secondary"
                        }}>{t('layout.sidebar.unified')}</Typography>
                    </Stack>
                </Box>
                <Box sx={{ mt: 1.5, display: 'flex', justifyContent: 'flex-end', gap: 1 }}>
                    <Button size="small" onClick={handleAddProfileClose} disabled={isCreating}>{t('common.cancel')}</Button>
                    <Button size="small" variant="contained" onClick={handleCreateProfile} disabled={!newProfileName.trim() || isCreating}>
                        {t('common.add')}
                    </Button>
                </Box>
            </Popover>
            {/* Add Team Popover — mirrors profile creation at the layout level. */}
            <Popover
                open={Boolean(addTeamAnchorEl)}
                anchorEl={addTeamAnchorEl}
                onClose={handleAddTeamClose}
                anchorOrigin={{vertical: 'top', horizontal: 'right'}}
                transformOrigin={{vertical: 'top', horizontal: 'left'}}
                slotProps={{paper: {sx: {p: 2, width: 280, mt: -0.5}}}}
            >
                <Typography variant="subtitle2" sx={{mb: 1.5, fontWeight: 600}}>{t('layout.sidebar.newTeam')}</Typography>
                <Stack spacing={1.5}>
                    <TextField
                        inputRef={addTeamInputRef}
                        fullWidth
                        size="small"
                        label={t('teams.name')}
                        value={newTeamName}
                        onChange={(e) => setNewTeamName(e.target.value)}
                        onKeyDown={(e) => e.key === 'Enter' && void handleCreateTeam()}
                        disabled={isCreating}
                    />
                </Stack>
                <Box sx={{mt: 1.5, display: 'flex', justifyContent: 'flex-end', gap: 1}}>
                    <Button size="small" onClick={handleAddTeamClose} disabled={isCreating}>{t('common.cancel')}</Button>
                    <Button size="small" variant="contained" onClick={handleCreateTeam}
                            disabled={!newTeamName.trim() || isCreating}>
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
