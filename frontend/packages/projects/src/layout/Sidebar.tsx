import { useState, type ReactNode } from 'react';
import { Badge, Box, Button, IconButton, List, ListItemButton, ListItemIcon, ListItemText, Menu, MenuItem, Tooltip, Typography, useTheme } from '@mui/material';
import { NavLink, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Add, Check, Folder, Forum, MoreHoriz, Settings, SidebarCollapse } from '../components/icons';
import { useAppData } from '../contexts/AppDataContext';
import { useThemeMode } from '../contexts/ThemeModeContext';
import { ChatActionsMenu } from '../components/ChatActionsMenu';
import type { Conversation } from '../client';
import { THEME_MODE_VALUES, ThemeModeIcon } from '../theme/modes';
import { fontSizes } from '@tingly/ui/theme/fonts';

export const SIDEBAR_WIDTH = 272;

function Section({ title, children }: { title: string; children: ReactNode }) {
    return (
        <Box sx={{ mt: 2 }}>
            <Typography variant="caption" sx={{ px: 2, color: 'text.secondary', fontWeight: 600 }}>{title}</Typography>
            <List dense disablePadding sx={{ mt: 0.5 }}>{children}</List>
        </Box>
    );
}

function ChatRow({ chat, selected }: { chat: Conversation; selected: boolean }) {
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    return (
        <>
            <ListItemButton
                selected={selected}
                component={NavLink}
                to={`/chat/${chat.id}`}
                sx={{ mx: 1, borderRadius: 1.5, py: 0.5, pr: 0.5, '& .row-menu': { opacity: anchor ? 1 : 0 }, '&:hover .row-menu': { opacity: 1 } }}
            >
                <ListItemText
                    primary={chat.title || t('chat.untitled')}
                    slotProps={{ primary: { noWrap: true, sx: { fontSize: fontSizes.lg, color: 'text.primary' } } }}
                />
                <IconButton
                    className="row-menu"
                    size="small"
                    aria-label={t('common.more')}
                    onClick={(e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        setAnchor(e.currentTarget);
                    }}
                >
                    <MoreHoriz fontSize="inherit" />
                </IconButton>
            </ListItemButton>
            <ChatActionsMenu
                chat={chat}
                anchorEl={anchor}
                onClose={() => setAnchor(null)}
                onDeleted={() => {
                    if (selected) navigate('/chats');
                }}
            />
        </>
    );
}

export function Sidebar({ onCollapse, onNavigate }: { onCollapse?: () => void; onNavigate?: () => void }) {
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const location = useLocation();
    const theme = useTheme();
    const { recents, starredChats, projects } = useAppData();
    const { mode, setMode } = useThemeMode();
    const [themeAnchor, setThemeAnchor] = useState<HTMLElement | null>(null);
    const starredIds = new Set(starredChats.map((c) => c.id));

    const nav = [
        { to: '/', label: t('nav.projects'), icon: <Folder fontSize="small" /> },
        { to: '/chats', label: t('nav.chats'), icon: <Forum fontSize="small" /> },
    ];
    // Starred first, then the most recently active; waiting counts make the
    // projects that need the user stand out.
    const sideProjects = projects.slice(0, 12);

    return (
        <Box
            onClick={(e) => {
                if ((e.target as HTMLElement).closest('a')) onNavigate?.();
            }}
            sx={{ width: SIDEBAR_WIDTH, height: '100%', display: 'flex', flexDirection: 'column', bgcolor: 'background.paper', borderRight: 1, borderColor: 'divider' }}
        >
            <Box sx={{ display: 'flex', alignItems: 'center', px: 2, pt: 1.5, pb: 1 }}>
                <Box component={NavLink} to="/" sx={{ display: 'flex', alignItems: 'center', gap: 1, textDecoration: 'none', color: 'text.primary', flex: 1 }}>
                    <Box component="img" src="/icon.svg" alt="" sx={{ width: 22, height: 22 }} />
                    <Typography sx={{ fontWeight: 650, fontSize: '0.9375rem' }}>Tingly Project</Typography>
                </Box>
                {onCollapse && (
                    <Tooltip title={t('nav.collapse')}>
                        <IconButton size="small" onClick={onCollapse} aria-label={t('nav.collapse')}>
                            <SidebarCollapse fontSize="small" />
                        </IconButton>
                    </Tooltip>
                )}
            </Box>

            <Box sx={{ px: 1.5 }}>
                <Button
                    fullWidth
                    startIcon={<Add />}
                    onClick={() => {
                        navigate('/?new=1');
                        onNavigate?.();
                    }}
                    sx={{ justifyContent: 'flex-start', color: 'primary.main', fontWeight: 600, px: 1.25 }}
                >
                    {t('projects.new')}
                </Button>
            </Box>
            <List dense disablePadding sx={{ mt: 0.5 }}>
                {nav.map((n) => (
                    <ListItemButton key={n.to} component={NavLink} to={n.to} selected={location.pathname === n.to} sx={{ mx: 1, borderRadius: 1.5 }}>
                        <ListItemIcon sx={{ minWidth: 32 }}>{n.icon}</ListItemIcon>
                        <ListItemText primary={n.label} slotProps={{ primary: { sx: { fontSize: '0.875rem', color: 'text.primary' } } }} />
                    </ListItemButton>
                ))}
            </List>

            <Box sx={{ flex: 1, overflowY: 'auto', pb: 2 }}>
                {sideProjects.length > 0 && (
                    <Section title={t('nav.projects')}>
                        {sideProjects.map((p) => (
                            <ListItemButton key={p.id} component={NavLink} to={`/project/${p.id}`} selected={location.pathname === `/project/${p.id}`} sx={{ mx: 1, borderRadius: 1.5, py: 0.5 }}>
                                <ListItemIcon sx={{ minWidth: 28 }}>
                                    <Badge color="warning" variant="dot" invisible={p.threads.waiting === 0}><Folder sx={{ fontSize: 16 }} /></Badge>
                                </ListItemIcon>
                                <ListItemText primary={p.name} slotProps={{ primary: { noWrap: true, sx: { fontSize: fontSizes.lg, color: 'text.primary' } } }} />
                                {p.threads.waiting > 0 && <Typography variant="caption" color="warning.main" sx={{ fontWeight: 600 }}>{p.threads.waiting}</Typography>}
                            </ListItemButton>
                        ))}
                    </Section>
                )}
                {(starredChats.length > 0 || recents.length > 0) && (
                    <Section title={t('nav.recentChats')}>
                        {starredChats.map((c) => <ChatRow key={c.id} chat={c} selected={location.pathname === `/chat/${c.id}`} />)}
                        {recents.filter((c) => !starredIds.has(c.id)).slice(0, 8).map((c) => (
                            <ChatRow key={c.id} chat={c} selected={location.pathname === `/chat/${c.id}`} />
                        ))}
                    </Section>
                )}
            </Box>

            <Box sx={{ display: 'flex', alignItems: 'center', px: 1, py: 1, borderTop: 1, borderColor: 'divider', gap: 0.5 }}>
                <ListItemButton component={NavLink} to="/settings" selected={location.pathname === '/settings'} sx={{ borderRadius: 1.5, flex: 1 }}>
                    <ListItemIcon sx={{ minWidth: 32 }}><Settings fontSize="small" /></ListItemIcon>
                    <ListItemText primary={t('nav.settings')} slotProps={{ primary: { sx: { fontSize: '0.875rem', color: 'text.primary' } } }} />
                </ListItemButton>
                <Tooltip title={`${t('settings.theme')}: ${t(`theme.${mode}`)}`}>
                    <IconButton size="small" onClick={(e) => setThemeAnchor(e.currentTarget)} aria-label={t('settings.theme')} sx={{ color: theme.palette.text.secondary }}>
                        <ThemeModeIcon mode={mode} size={20} />
                    </IconButton>
                </Tooltip>
                <Menu anchorEl={themeAnchor} open={!!themeAnchor} onClose={() => setThemeAnchor(null)} anchorOrigin={{ vertical: 'top', horizontal: 'right' }} transformOrigin={{ vertical: 'bottom', horizontal: 'right' }}>
                    {THEME_MODE_VALUES.map((m) => (
                        <MenuItem key={m} selected={m === mode} onClick={() => { setMode(m); setThemeAnchor(null); }}>
                            <ListItemIcon><ThemeModeIcon mode={m} /></ListItemIcon>
                            <ListItemText>{t(`theme.${m}`)}</ListItemText>
                            {m === mode && <Check fontSize="small" sx={{ ml: 2, color: 'primary.main' }} />}
                        </MenuItem>
                    ))}
                </Menu>
            </Box>
        </Box>
    );
}
