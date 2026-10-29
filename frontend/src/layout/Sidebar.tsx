import { Info as IconInfoCircle, Pin as IconPin, PinFilled as IconPinFilled, Visibility as IconVisibility, VisibilityOff as IconVisibilityOff } from '@/components/icons';
import {
    Box,
    Button,
    Divider,
    IconButton,
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
import { teamPath } from '@/utils/team';
import { useNotify } from '@/hooks/useNotify';
import { useVersion } from '@/contexts/VersionContext';
import { footerHeight, sidebarWidth } from './constants';
import {
    NAV_ROW_SX,
    navRowSelectedSx,
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
    /** A custom body in place of the rows (ActivityItem.sidebar). */
    body?: (onNavigate: () => void) => React.ReactNode;
    activeActivityLabel: string;
    onClose: () => void;
    headerAction?: React.ReactNode;
    /** Edit mode: hidden rows are listed (dimmed) and every hideable row gets a visibility toggle. */
    editing?: boolean;
    onToggleHidden?: (id: string) => void;
    /** Hover a pinnable row to pin it to the top of the list (outside edit mode). */
    onTogglePinned?: (id: string) => void;
}

export const Sidebar: React.FC<SidebarProps> = ({ sidebarItems, body, activeActivityLabel, onClose, headerAction, editing = false, onToggleHidden, onTogglePinned }) => {
    const { t } = useTranslation();
    const location = useLocation();
    const navigate = useNavigate();
    const notify = useNotify();
    const { refresh } = useProfileContext();
    const { refresh: refreshTeams } = useTeamContext();
    const { currentVersion } = useVersion();

    const [addProfileAnchorEl, setAddProfileAnchorEl] = useState<HTMLElement | null>(null);
    const [newProfileName, setNewProfileName] = useState('');
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

    const handleAddTeamClick = useCallback((e: React.MouseEvent<HTMLElement>) => {
        setAddTeamAnchorEl(e.currentTarget);
        setNewTeamName('');
        setTimeout(() => addTeamInputRef.current?.focus(), 100);
    }, []);

    const handleAddTeamClose = useCallback(() => {
        setAddTeamAnchorEl(null);
        setNewTeamName('');
    }, []);

    const [isCreatingTeam, setIsCreatingTeam] = useState(false);

    const handleCreateTeam = useCallback(async () => {
        if (!newTeamName.trim()) return;
        setIsCreatingTeam(true);
        const result = await api.createTeam({name: newTeamName.trim()});
        setIsCreatingTeam(false);
        if (!result.success) {
            notify.error(result.error?.message || t('teams.saveFailed'));
            return;
        }
        handleAddTeamClose();
        await refreshTeams();
        navigate(teamPath(result.data));
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
            {body ? <Box sx={sidebarListScrollSx}>{body(onClose)}</Box> : (
            <List sx={sidebarListScrollSx}>
                {sidebarItems.map((item, index) => {
                    if (item.type === 'divider') {
                        return <Divider key={`divider-${index}`} sx={{ mx: 2, my: 1 }} />;
                    }

                    const isAddProfile = item.path === '#add-profile';
                    const isAddTeam = item.path === '#add-team';
                    const isAddAction = isAddProfile || isAddTeam;
                    const active = !isAddAction && (item.match ? item.match(location.pathname) : isActive(item.path));

                    const toggleable = editing && item.hideId && onToggleHidden;
                    const pinnable = !editing && !!item.pinId && !!onTogglePinned;
                    const button = (
                        <ListItem
                            disablePadding
                            sx={{
                                opacity: editing && item.hidden ? 0.5 : 1,
                                // The pin shows only on hover / keyboard focus; pinned rows already sit in their own group up top.
                                '& .pin-btn': { opacity: 0, transition: 'opacity 0.12s' },
                                '&:hover .pin-btn, & .pin-btn:focus-visible': { opacity: 1 },
                                // Room for the pin only while it is showing, so names keep their width.
                                ...(pinnable && {
                                    '&& > .MuiListItemButton-root': { pr: 1.5 },
                                    '&&:hover > .MuiListItemButton-root, &&:focus-within > .MuiListItemButton-root': { pr: 5 },
                                }),
                            }}
                            secondaryAction={pinnable ? (
                                <Tooltip title={item.pinned ? t('layout.sidebar.unpin') : t('layout.sidebar.pinToTop')} placement="right" arrow>
                                    <IconButton
                                        className="pin-btn"
                                        size="small"
                                        edge="end"
                                        onClick={() => onTogglePinned!(item.pinId!)}
                                        aria-label={item.pinned ? t('layout.sidebar.unpin') : t('layout.sidebar.pinToTop')}
                                        aria-pressed={!!item.pinned}
                                        sx={{ color: 'text.secondary', mr: 0.5 }}
                                    >
                                        {item.pinned ? <IconPinFilled sx={{ fontSize: 16 }} /> : <IconPin sx={{ fontSize: 16 }} />}
                                    </IconButton>
                                </Tooltip>
                            ) : toggleable ? (
                                <Tooltip title={item.hidden ? t('scenarioOverview.showInSidebar') : t('scenarioOverview.hideFromSidebar')} placement="right" arrow>
                                    <IconButton
                                        size="small"
                                        edge="end"
                                        onClick={() => onToggleHidden(item.hideId!)}
                                        aria-label={item.hidden ? t('scenarioOverview.showInSidebar') : t('scenarioOverview.hideFromSidebar')}
                                        sx={{ color: 'text.secondary', mr: 0.5 }}
                                    >
                                        {item.hidden ? <IconVisibilityOff sx={{ fontSize: 18 }} /> : <IconVisibility sx={{ fontSize: 18 }} />}
                                    </IconButton>
                                </Tooltip>
                            ) : undefined}
                        >
                            <ListItemButton
                                aria-current={active ? 'page' : undefined}
                                {...(isAddAction
                                    ? { onClick: isAddProfile ? handleAddProfileClick : handleAddTeamClick }
                                    : { component: RouterLink, to: item.path, onClick: onClose }
                                )}
                                sx={{
                                    ...NAV_ROW_SX,
                                    color: 'text.secondary',
                                    position: 'relative',
                                    '&:hover': { backgroundColor: 'action.hover', color: 'text.primary' },
                                    ...(active && navRowSelectedSx),
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
                                {item.count && (
                                    <Tooltip title={item.count.title} placement="right">
                                        <Typography
                                            component="span"
                                            variant="caption"
                                            sx={{
                                                flexShrink: 0,
                                                ml: 1,
                                                fontVariantNumeric: 'tabular-nums',
                                                color: 'text.secondary',
                                            }}
                                        >
                                            {item.count.value}
                                        </Typography>
                                    </Tooltip>
                                )}
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
                            ) : item.hint && !editing ? (
                                <Tooltip
                                    title={item.hint}
                                    arrow
                                    placement="right"
                                    enterDelay={800}
                                    enterNextDelay={800}
                                    slotProps={{ tooltip: { sx: { maxWidth: 280 } } }}
                                >
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
            )}
            {/* Footer: the running version and its update badge — app state,
                one row. A dev build's long version ellipsizes, the badge stays.
                The GitHub star link lives under the slogan below instead:
                side by side it was pushed out, and stacked here it read as a
                second status line. */}
            <Box
                sx={{
                    py: 1.5, px: 2,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    flexShrink: 0,
                    height: footerHeight,
                    minWidth: 0,
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
                        disabled={isCreatingTeam}
                    />
                </Stack>
                <Box sx={{mt: 1.5, display: 'flex', justifyContent: 'flex-end', gap: 1}}>
                    <Button size="small" onClick={handleAddTeamClose} disabled={isCreatingTeam}>{t('common.cancel')}</Button>
                    <Button size="small" variant="contained" onClick={handleCreateTeam}
                            disabled={!newTeamName.trim() || isCreatingTeam}>
                        {t('common.add')}
                    </Button>
                </Box>
            </Popover>
            {/* Footer bottom row: the slogan. Still footerHeight tall, so its
                bottom lines up with the activity bar's footer. The GitHub star
                lives in the activity bar, under Help. */}
            <Box
                sx={{
                    height: footerHeight, py: 1, px: 2, borderTop: '1px solid', borderColor: 'divider',
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                }}
            >
                <Typography
                    variant="caption"
                    component="div"
                    sx={{ color: 'text.secondary', textAlign: 'center', fontStyle: 'italic' }}
                >
                    <Tooltip title={t('layout.sidebar.sloganTooltip')} placement="top" arrow>
                        <Box component="span" sx={{ cursor: 'default' }}>{t('layout.slogan')}</Box>
                    </Tooltip>
                </Typography>
            </Box>
            {/* Update Panel Dialog */}
            <UpdatePanelDialog
                open={updatePanelOpen}
                onClose={() => setUpdatePanelOpen(false)}
            />
        </Box>
    );
};
