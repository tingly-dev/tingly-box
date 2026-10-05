import {Add, FolderOpen, Search, Stream, Close} from '@/components/icons';
import type {SessionInfo} from '@/services/deskApi';
import {Box, Button, CircularProgress, IconButton, InputBase, List, ListItemButton, Tooltip, Typography, Tabs, Tab} from '@mui/material';
import {useMemo, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {folderName, groupSessionsByFolder, isBusyStatus, sessionTitle} from './deskUtils';
import {getStatusColor} from '@/theme/status';
import { fontSizes } from '@/theme/fonts';

interface DeskSidebarProps {
    sessions: SessionInfo[];
    selectedId: string | null;
    onSelect: (id: string) => void;
    // folder is set when starting from a folder group's "+".
    onNew: (folder?: string) => void;
    // Sessions whose turn ended while the user was elsewhere.
    unseen: Set<string>;
    projects?: string[];
    selectedProject?: string;
    onAddProject?: () => void;
    onRemoveProject?: (path: string) => void;
}

// StatusMark says only what needs attention, most urgent first: a turn
// waiting on the user, one still running, one that failed, one that ended
// while the user was elsewhere. A seen, finished session needs no mark.
const StatusMark = ({session, unseen}: {session: SessionInfo; unseen: boolean}) => {
    const {t} = useTranslation();
    if (session.awaiting_input) {
        return (
            <Box component="span" sx={{px: 0.75, borderRadius: 1, fontSize: fontSizes.micro, fontWeight: 600, lineHeight: 1.6, bgcolor: (theme) => getStatusColor(theme, 'warning'), color: (theme) => theme.palette.getContrastText(getStatusColor(theme, 'warning'))}}>
                {t('desk.waitingMark', {defaultValue: 'waiting'})}
            </Box>
        );
    }
    if (isBusyStatus(session.status)) return <CircularProgress size={10} thickness={6}/>;
    // The reply is done but work it started goes on: a quieter mark than a
    // running turn, since nothing waits on the user.
    const bg = session.background_tasks?.length ?? 0;
    if (bg > 0) {
        return (
            <Tooltip title={t('desk.backgroundRunning', {defaultValue: '{{count}} background task(s) running', count: bg})}>
                <Box component="span" sx={{display: 'inline-flex', alignItems: 'center', gap: 0.25, color: 'text.secondary', fontSize: fontSizes.xs}}>
                    <Stream sx={{fontSize: 13}}/>{bg}
                </Box>
            </Tooltip>
        );
    }
    if (session.status === 'failed') return <Box sx={{width: 7, height: 7, borderRadius: '50%', bgcolor: (theme) => getStatusColor(theme, 'error')}}/>;
    if (unseen) return <Box sx={{width: 7, height: 7, borderRadius: '50%', bgcolor: 'primary.main'}}/>;
    return null;
};

// Rows use primary text (the theme's body text defaults to secondary grey,
// which read as disabled here); selection keeps the theme's own highlight.
const rowSx = {
    borderRadius: 1.5,
    gap: 1,
    color: 'text.primary',
    '&.Mui-selected': {fontWeight: 500},
} as const;

const EMPTY_PROJECTS: string[] = [];

const DeskSidebar = ({sessions, selectedId, onSelect, onNew, unseen, projects = EMPTY_PROJECTS, selectedProject, onAddProject, onRemoveProject}: DeskSidebarProps) => {
    const {t} = useTranslation();
    const [query, setQuery] = useState('');
    const selectedClosed = sessions.find((session) => session.id === selectedId)?.status === 'closed';
    const filterKey = `${selectedId}:${selectedClosed}`;
    const [filter, setFilter] = useState({key: filterKey, archived: selectedClosed});
    const archived = filter.key === filterKey ? filter.archived : selectedClosed;

    const groups = useMemo(() => {
        const q = query.trim().toLowerCase();
        const visible = sessions.filter((s) => (s.status === 'closed') === archived
            && (!q || sessionTitle(s).toLowerCase().includes(q) || s.project.toLowerCase().includes(q)));
        const groups = groupSessionsByFolder(visible);
        if (archived) return groups;
        const byPath = new Map(groups.map((g) => [g.path, g]));
        for (const path of projects) {
            if (!byPath.has(path) && (!q || path.toLowerCase().includes(q))) byPath.set(path, {path, sessions: []});
        }
        return [...projects.filter((path) => byPath.has(path)).map((path) => byPath.get(path)!), ...groups.filter((g) => !projects.includes(g.path))];
    }, [sessions, query, archived, projects]);

    return (
        <Box sx={{display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0}}>
            <Box sx={{px: 1, pt: 1, pb: 0.5}}>
                <ListItemButton onClick={() => onNew()} selected={selectedId === null} sx={{...rowSx, py: 0.75}}>
                    <Add sx={{fontSize: 18}}/>
                    <Typography variant="body2" sx={{color: 'inherit'}}>{t('desk.newSession', {defaultValue: 'New session'})}</Typography>
                </ListItemButton>
                {onAddProject && <Button fullWidth size="small" startIcon={<FolderOpen/>} onClick={onAddProject} sx={{justifyContent: 'flex-start', px: 1.5, mt: 0.5}}>
                    {t('desk.addProject', {defaultValue: 'Add project'})}
                </Button>}
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1, px: 1.5, py: 0.5, mt: 0.5, borderRadius: 1.5, bgcolor: 'action.hover'}}>
                    <Search sx={{fontSize: 16, color: 'text.secondary'}}/>
                    <InputBase
                        fullWidth
                        inputProps={{'aria-label': t('desk.search', {defaultValue: 'Search'})}}
                        placeholder={t('desk.search', {defaultValue: 'Search'})}
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        sx={{fontSize: '0.875rem'}}
                    />
                </Box>
                <Tabs value={archived ? 1 : 0} onChange={(_event, value: number) => setFilter({key: filterKey, archived: value === 1})} variant="fullWidth" aria-label={t('desk.sessionFilter', {defaultValue: 'Session filter'})} sx={{minHeight: 36, '& .MuiTab-root': {minHeight: 36, py: 0.75}}}>
                    <Tab label={t('desk.activeSessions', {defaultValue: 'Active'})}/>
                    <Tab label={t('desk.archivedSessions', {defaultValue: 'Archived'})}/>
                </Tabs>
            </Box>

            <Box sx={{flex: 1, minHeight: 0, overflowY: 'auto', px: 1, pb: 1}}>
                {groups.length === 0 && (
                    <Typography variant="body2" color="text.secondary" sx={{px: 1.5, py: 2}}>
                        {query
                            ? t('desk.noMatches', {defaultValue: 'No matching sessions'})
                            : t('desk.noSessions', {defaultValue: 'No sessions yet'})}
                    </Typography>
                )}
                {groups.map((g) => (
                    <Box key={g.path} sx={{mt: 1.5, borderRadius: 1.5, bgcolor: selectedId === null && selectedProject === g.path ? 'action.selected' : undefined}}>
                        <Box sx={{display: 'flex', alignItems: 'center', px: 1.5, mb: 0.25, '&:hover .desk-folder-add': {opacity: 1}}}>
                            <Tooltip title={g.path} placement="right">
                                <Typography variant="body2" noWrap sx={{flex: 1, fontWeight: 600, fontSize: fontSizes.sm, color: 'text.secondary'}}>
                                    {folderName(g.path)}
                                </Typography>
                            </Tooltip>
                            {g.sessions.length === 0 && onRemoveProject && <Tooltip title={t('desk.removeProjectShortcut', {defaultValue: 'Remove project shortcut'})}>
                                <IconButton size="small" aria-label={t('desk.removeProjectShortcut', {defaultValue: 'Remove project shortcut'})} onClick={() => onRemoveProject(g.path)} sx={{p: 0.25}}><Close sx={{fontSize: 16}}/></IconButton>
                            </Tooltip>}
                            <Tooltip title={t('desk.newInFolder', {defaultValue: 'New session in this folder'})}>
                                <IconButton
                                    aria-label={t('desk.newInFolder', {defaultValue: 'New session in this folder'})}
                                    className="desk-folder-add"
                                    size="small"
                                    onClick={() => onNew(g.path)}
                                    sx={{p: 0.25, opacity: 0.6}}
                                >
                                    <Add sx={{fontSize: 16}}/>
                                </IconButton>
                            </Tooltip>
                        </Box>
                        <List dense disablePadding>
                            {g.sessions.length === 0 && <ListItemButton onClick={() => onNew(g.path)} sx={{...rowSx, py: 0.75}}>
                                <Add sx={{fontSize: 16}}/><Typography variant="body2" sx={{fontSize: fontSizes.md}}>{t('desk.firstProjectTask', {defaultValue: 'Create first task'})}</Typography>
                            </ListItemButton>}
                            {g.sessions.map((s) => (
                                <ListItemButton
                                    key={s.id}
                                    title={sessionTitle(s)}
                                    aria-current={s.id === selectedId ? 'page' : undefined}
                                    selected={s.id === selectedId}
                                    onClick={() => onSelect(s.id)}
                                    sx={{...rowSx, py: 0.5, opacity: 1}}
                                >
                                    <Typography variant="body2" noWrap sx={{flex: 1, color: 'inherit', fontSize: fontSizes.md, fontWeight: unseen.has(s.id) ? 600 : undefined}}>
                                        {sessionTitle(s)}
                                    </Typography>
                                    <StatusMark session={s} unseen={unseen.has(s.id)}/>
                                </ListItemButton>
                            ))}
                        </List>
                    </Box>
                ))}
            </Box>
        </Box>
    );
};

export default DeskSidebar;
